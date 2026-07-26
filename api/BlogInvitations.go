package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/inceptiondb"
)

type BlogInvitationStatus string

const (
	BlogInvitationPending  BlogInvitationStatus = "pending"
	BlogInvitationAccepted BlogInvitationStatus = "accepted"
	BlogInvitationRevoked  BlogInvitationStatus = "revoked"
)

type BlogInvitation struct {
	Id             string               `json:"id"`
	BlogId         string               `json:"blog_id"`
	TokenHash      string               `json:"token_hash"`
	Status         BlogInvitationStatus `json:"status"`
	CreatedById    string               `json:"created_by_id"`
	CreatedByNick  string               `json:"created_by_nick"`
	AcceptedById   string               `json:"accepted_by_id,omitempty"`
	AcceptedByNick string               `json:"accepted_by_nick,omitempty"`
	CreatedOn      time.Time            `json:"createdOn"`
	ExpiresOn      time.Time            `json:"expiresOn"`
	AcceptedOn     time.Time            `json:"acceptedOn,omitempty"`
	RevokedOn      time.Time            `json:"revokedOn,omitempty"`
}

type PublicBlogInvitation struct {
	Id             string               `json:"id"`
	BlogId         string               `json:"blog_id"`
	Status         BlogInvitationStatus `json:"status"`
	CreatedById    string               `json:"created_by_id"`
	CreatedByNick  string               `json:"created_by_nick"`
	AcceptedById   string               `json:"accepted_by_id,omitempty"`
	AcceptedByNick string               `json:"accepted_by_nick,omitempty"`
	CreatedOn      time.Time            `json:"createdOn"`
	ExpiresOn      time.Time            `json:"expiresOn"`
	AcceptedOn     time.Time            `json:"acceptedOn,omitempty"`
	RevokedOn      time.Time            `json:"revokedOn,omitempty"`
}

type CreateBlogInvitationResponse struct {
	*PublicBlogInvitation
	InviteUrl string `json:"inviteUrl"`
}

func ListBlogInvitations(ctx context.Context, r *http.Request) (ListPage[*PublicBlogInvitation], error) {
	offset, err := listOffset(r)
	if err != nil {
		return ListPage[*PublicBlogInvitation]{}, err
	}
	blogId := box.GetUrlParameter(ctx, "blogId")
	if _, err := RequireBlogOwner(ctx, blogId); err != nil {
		return ListPage[*PublicBlogInvitation]{}, err
	}
	result := []*PublicBlogInvitation{}
	if err := GetInceptionClient(ctx).FindAll(blogInvitationsCollection, inceptiondb.FindQuery{
		Index: "blog-invitations-created-v2", Skip: offset, Limit: listPageSize + 1, Reverse: true, Filter: JSON{
			"blog_id": blogId, "status": BlogInvitationPending,
		}}, func(invitation *BlogInvitation) {
		result = append(result, publicBlogInvitation(invitation))
	}); err != nil {
		return ListPage[*PublicBlogInvitation]{}, blogStorageError("list invitations", err)
	}
	return pageFromLookahead(result, offset), nil
}

func CreateBlogInvitation(ctx context.Context, w http.ResponseWriter) (*CreateBlogInvitationResponse, error) {
	blogId := box.GetUrlParameter(ctx, "blogId")
	blog, err := RequireBlogOwner(ctx, blogId)
	if err != nil {
		return nil, err
	}
	if blog.Banned {
		return nil, blogBanned()
	}
	auth, _ := requireBlogAuth(ctx)
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, blogStorageError("generate invitation token", err)
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	now := time.Now().UTC()
	invitation := &BlogInvitation{Id: "blog_invitation_" + uuid.NewString(), BlogId: blogId,
		TokenHash: hashBlogInvitationToken(token), Status: BlogInvitationPending,
		CreatedById: auth.User.ID, CreatedByNick: auth.User.Nick, CreatedOn: now, ExpiresOn: now.Add(blogInvitationPeriod)}
	if err := GetInceptionClient(ctx).Insert(blogInvitationsCollection, invitation); err != nil {
		return nil, blogStorageError("create invitation", err)
	}
	w.WriteHeader(http.StatusCreated)
	return &CreateBlogInvitationResponse{PublicBlogInvitation: publicBlogInvitation(invitation), InviteUrl: "/admin/#/invite/" + token}, nil
}

func RevokeBlogInvitation(ctx context.Context) (*PublicBlogInvitation, error) {
	blogId := box.GetUrlParameter(ctx, "blogId")
	if _, err := RequireBlogOwner(ctx, blogId); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	patched := &BlogInvitation{}
	matched, err := GetInceptionClient(ctx).PatchOne(blogInvitationsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": box.GetUrlParameter(ctx, "invitationId"), "blog_id": blogId, "status": BlogInvitationPending,
	}, Patch: JSON{"status": BlogInvitationRevoked, "revokedOn": now}}, patched)
	if err != nil {
		return nil, blogStorageError("revoke invitation", err)
	}
	if !matched {
		return nil, invitationUnavailable()
	}
	return publicBlogInvitation(patched), nil
}

func AcceptBlogInvitation(ctx context.Context, w http.ResponseWriter, r *http.Request) (*BlogWithRole, error) {
	auth, err := requireBlogAuth(ctx)
	if err != nil {
		return nil, err
	}
	input := struct {
		Token string `json:"token"`
	}{}
	if err := decodeBlogJSON(w, r, &input); err != nil {
		return nil, err
	}
	token := strings.TrimSpace(input.Token)
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return nil, invitationNotFound()
	}
	db := GetInceptionClient(ctx)
	hash := hashBlogInvitationToken(token)
	invitation := &BlogInvitation{}
	if err := db.FindOne(blogInvitationsCollection, inceptiondb.FindQuery{Index: "blog-invitations-token-hash-v1", Value: hash, Filter: JSON{"token_hash": hash}}, invitation); err != nil {
		return nil, invitationNotFound()
	}
	now := time.Now().UTC()
	if invitation.Status != BlogInvitationPending {
		return nil, invitationUnavailable()
	}
	if !invitation.ExpiresOn.After(now) {
		return nil, HttpError{Status: http.StatusGone, Title: "Invitation Expired", Description: "La invitación ha caducado"}
	}
	blog := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": invitation.BlogId}}, blog); err != nil {
		return nil, blogNotFound()
	}
	if blog.Banned {
		return nil, invitationUnavailable()
	}
	if blog.OwnerId == auth.User.ID {
		return nil, HttpError{Status: http.StatusConflict, Title: "Invitation Not Needed", Code: "invitation_not_needed", Description: "Ya eres propietario de este blog"}
	}
	existingMember := findBlogMember(blog.Members, auth.User.ID)
	if existingMember != nil && existingMember.Role == BlogRoleEditor {
		return nil, HttpError{Status: http.StatusConflict, Title: "Invitation Not Needed", Code: "invitation_not_needed", Description: "Ya eres editor de este blog"}
	}
	membershipCreated, alreadyEditor, membershipErr := ensureEditorMembership(db, blog, auth.User.ID, auth.User.Nick, now)
	if membershipErr != nil {
		return nil, blogStorageError("create invitation membership", membershipErr)
	}
	if alreadyEditor {
		return nil, HttpError{Status: http.StatusConflict, Title: "Invitation Not Needed", Code: "invitation_not_needed", Description: "Ya eres editor de este blog"}
	}
	claimed := &BlogInvitation{}
	matched, err := db.PatchOne(blogInvitationsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": invitation.Id, "token_hash": hash, "status": BlogInvitationPending, "expiresOn": JSON{"$gt": now},
	}, Patch: JSON{"status": BlogInvitationAccepted, "accepted_by_id": auth.User.ID,
		"accepted_by_nick": auth.User.Nick, "acceptedOn": now}}, claimed)
	if err != nil {
		if membershipCreated {
			removeInvitationMembership(db, blog.Id, auth.User.ID, now)
		}
		return nil, blogStorageError("accept invitation", err)
	}
	if !matched {
		if membershipCreated {
			removeInvitationMembership(db, blog.Id, auth.User.ID, now)
		}
		return nil, invitationUnavailable()
	}
	return &BlogWithRole{Blog: blog, Role: BlogRoleEditor}, nil
}

func removeInvitationMembership(db *inceptiondb.Client, blogId, userId string, createdOn time.Time) {
	_, _ = mutateBlogMembers(db, blogId, func(current *Blog) (bool, error) {
		member := findBlogMember(current.Members, userId)
		if member == nil || member.Id != deterministicBlogMemberId(blogId, userId) || member.Role != BlogRoleEditor || !member.CreatedOn.Equal(createdOn) {
			return false, nil
		}
		current.Members = removeBlogMemberByUser(current.Members, userId)
		return true, nil
	})
}

func hashBlogInvitationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func publicBlogInvitation(invitation *BlogInvitation) *PublicBlogInvitation {
	return &PublicBlogInvitation{Id: invitation.Id, BlogId: invitation.BlogId, Status: invitation.Status,
		CreatedById: invitation.CreatedById, CreatedByNick: invitation.CreatedByNick,
		AcceptedById: invitation.AcceptedById, AcceptedByNick: invitation.AcceptedByNick,
		CreatedOn: invitation.CreatedOn, ExpiresOn: invitation.ExpiresOn,
		AcceptedOn: invitation.AcceptedOn, RevokedOn: invitation.RevokedOn}
}

func invitationNotFound() HttpError {
	return HttpError{Status: http.StatusNotFound, Title: "Invitation Not Found", Description: "La invitación no existe"}
}

func invitationUnavailable() HttpError {
	return HttpError{Status: http.StatusGone, Title: "Invitation Unavailable", Description: "La invitación ya no está disponible"}
}
