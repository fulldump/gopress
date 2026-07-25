package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/inceptiondb"
)

type CreateBlogRequest struct {
	Slug        string       `json:"slug"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Language    BlogLanguage `json:"language"`
	AccentColor string       `json:"accent_color"`
	HeaderImage string       `json:"header_image"`
}

type PatchBlogRequest struct {
	Name        *string       `json:"name,omitempty"`
	Description *string       `json:"description,omitempty"`
	Language    *BlogLanguage `json:"language,omitempty"`
	AccentColor *string       `json:"accent_color,omitempty"`
	HeaderImage *string       `json:"header_image,omitempty"`
}

func ListBlogs(ctx context.Context) ([]*BlogWithRole, error) {
	auth, err := requireBlogAuth(ctx)
	if err != nil {
		return nil, err
	}
	db := GetInceptionClient(ctx)
	byId := map[string]*BlogWithRole{}
	if err := db.FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogs, Filter: JSON{"owner_id": auth.User.ID}}, func(blog *Blog) {
		byId[blog.Id] = &BlogWithRole{Blog: blog, Role: BlogRoleOwner}
	}); err != nil {
		return nil, blogStorageError("list owned blogs", err)
	}
	memberships := []*BlogMember{}
	if err := db.FindAll(blogMembersCollection, inceptiondb.FindQuery{Limit: maxBlogMembers, Filter: JSON{"user_id": auth.User.ID}}, func(member *BlogMember) {
		memberships = append(memberships, member)
	}); err != nil {
		return nil, blogStorageError("list memberships", err)
	}
	for _, member := range memberships {
		if member.Role != BlogRoleEditor {
			continue
		}
		if _, owner := byId[member.BlogId]; owner {
			continue
		}
		blog := &Blog{}
		if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": member.BlogId}}, blog); err == nil && blog.OwnerId != auth.User.ID {
			byId[blog.Id] = &BlogWithRole{Blog: blog, Role: BlogRoleEditor}
		}
	}
	result := make([]*BlogWithRole, 0, len(byId))
	for _, blog := range byId {
		result = append(result, blog)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Id < result[j].Id })
	return result, nil
}

func CreateBlog(ctx context.Context, w http.ResponseWriter, r *http.Request) (*BlogWithRole, error) {
	auth, err := requireBlogAuth(ctx)
	if err != nil {
		return nil, err
	}
	input := &CreateBlogRequest{}
	if err := decodeBlogJSON(w, r, input); err != nil {
		return nil, err
	}
	input.Slug = strings.TrimSpace(input.Slug)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.HeaderImage = strings.TrimSpace(input.HeaderImage)
	if input.Language == "" {
		input.Language = BlogLanguageES
	}
	if input.AccentColor == "" {
		input.AccentColor = "#2563EB"
	}
	if err := ValidateBlogSlug(input.Slug); err != nil {
		return nil, invalidBlog(err.Error())
	}
	if err := validateBlogFields(input.Name, input.Description, input.Language, input.AccentColor, input.HeaderImage); err != nil {
		return nil, invalidBlog(err.Error())
	}
	if input.HeaderImage != "" {
		return nil, invalidBlog("upload the header image after creating the blog")
	}
	now := time.Now().UTC()
	blog := &Blog{Id: "blog_" + uuid.NewString(), Slug: input.Slug, Name: input.Name,
		Description: input.Description, Language: input.Language, AccentColor: input.AccentColor,
		HeaderImage: input.HeaderImage, OwnerId: auth.User.ID, OwnerNick: auth.User.Nick,
		LegacyNicks: []string{}, CreatedOn: now, UpdatedOn: now}
	blogSlugMu.Lock()
	err = insertBlogWithOwner(GetInceptionClient(ctx), blog)
	blogSlugMu.Unlock()
	if err != nil {
		if errors.Is(err, inceptiondb.ErrorAlreadyExist) {
			return nil, blogSlugConflict()
		}
		return nil, err
	}
	w.WriteHeader(http.StatusCreated)
	return &BlogWithRole{Blog: blog, Role: BlogRoleOwner}, nil
}

func GetBlog(ctx context.Context) (*BlogWithRole, error) {
	blog, role, err := RequireBlogAccess(ctx, box.GetUrlParameter(ctx, "blogId"))
	if err != nil {
		return nil, err
	}
	return &BlogWithRole{Blog: blog, Role: role}, nil
}

func PatchBlog(ctx context.Context, w http.ResponseWriter, r *http.Request) (*BlogWithRole, error) {
	blog, err := RequireBlogOwner(ctx, box.GetUrlParameter(ctx, "blogId"))
	if err != nil {
		return nil, err
	}
	input := &PatchBlogRequest{}
	if err := decodeBlogJSON(w, r, input); err != nil {
		return nil, err
	}
	updated := *blog
	if input.Name != nil {
		updated.Name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		updated.Description = strings.TrimSpace(*input.Description)
	}
	if input.Language != nil {
		updated.Language = *input.Language
	}
	if input.AccentColor != nil {
		updated.AccentColor = *input.AccentColor
	}
	if input.HeaderImage != nil {
		updated.HeaderImage = strings.TrimSpace(*input.HeaderImage)
	}
	if err := validateBlogFields(updated.Name, updated.Description, updated.Language, updated.AccentColor, updated.HeaderImage); err != nil {
		return nil, invalidBlog(err.Error())
	}
	if input.HeaderImage != nil {
		if err := validateBlogHeaderFile(ctx, blog.Id, updated.HeaderImage); err != nil {
			return nil, err
		}
	}
	updated.UpdatedOn = time.Now().UTC()
	patch := JSON{"updatedOn": updated.UpdatedOn}
	if input.Name != nil {
		patch["name"] = updated.Name
	}
	if input.Description != nil {
		patch["description"] = updated.Description
	}
	if input.Language != nil {
		patch["language"] = updated.Language
	}
	if input.AccentColor != nil {
		patch["accent_color"] = updated.AccentColor
	}
	if input.HeaderImage != nil {
		patch["header_image"] = updated.HeaderImage
	}
	patched := &Blog{}
	matched, err := GetInceptionClient(ctx).PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": blog.Id, "owner_id": blog.OwnerId, "slug": blog.Slug,
	}, Patch: patch}, patched)
	if err != nil {
		return nil, blogStorageError("patch blog", err)
	}
	if !matched {
		return nil, blogForbidden()
	}
	return &BlogWithRole{Blog: patched, Role: BlogRoleOwner}, nil
}

func ListBlogMembers(ctx context.Context) ([]*BlogMember, error) {
	blogId := box.GetUrlParameter(ctx, "blogId")
	if _, err := RequireBlogOwner(ctx, blogId); err != nil {
		return nil, err
	}
	members := []*BlogMember{}
	if err := GetInceptionClient(ctx).FindAll(blogMembersCollection, inceptiondb.FindQuery{Limit: maxBlogMembers, Filter: JSON{"blog_id": blogId}}, func(member *BlogMember) {
		members = append(members, member)
	}); err != nil {
		return nil, blogStorageError("list members", err)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Id < members[j].Id })
	return members, nil
}

func RemoveBlogMember(ctx context.Context) (*BlogMember, error) {
	blogId := box.GetUrlParameter(ctx, "blogId")
	blog, err := RequireBlogOwner(ctx, blogId)
	if err != nil {
		return nil, err
	}
	userId := box.GetUrlParameter(ctx, "userId")
	if userId == blog.OwnerId {
		return nil, HttpError{Status: http.StatusForbidden, Title: "Blog Owner Cannot Be Removed", Description: "No se puede eliminar al propietario del blog"}
	}
	removed, err := GetInceptionClient(ctx).Remove(blogMembersCollection, inceptiondb.FindQuery{Limit: 1, Filter: JSON{
		"id": deterministicBlogMemberId(blogId, userId), "blog_id": blogId, "user_id": userId, "role": BlogRoleEditor,
	}})
	if err != nil {
		return nil, blogStorageError("remove member", err)
	}
	defer removed.Close()
	member := &BlogMember{}
	if err := json.NewDecoder(removed).Decode(member); err != nil {
		return nil, HttpError{Status: http.StatusNotFound, Title: "Blog Member Not Found", Description: "El miembro no existe"}
	}
	return member, nil
}

func decodeBlogJSON(w http.ResponseWriter, r *http.Request, output any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBlogRequestSize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return invalidBlog("request body must be a valid JSON object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return invalidBlog("request body must contain one JSON object")
	}
	return nil
}

func blogSlugConflict() HttpError {
	return HttpError{Status: http.StatusConflict, Title: "Blog Slug Already Exists", Description: "Ya existe un blog con ese slug"}
}
