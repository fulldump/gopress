package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

var blogSlugMu sync.Mutex

func InitializeBlogCollections(db *inceptiondb.Client) error {
	for _, collection := range []string{blogsCollection, blogMembersCollection, blogInvitationsCollection} {
		if err := db.CreateCollection(collection); err != nil && !errors.Is(err, inceptiondb.ErrorAlreadyExist) {
			return fmt.Errorf("create collection %s: %w", collection, err)
		}
	}
	return nil
}

func EnsureBlogIndexes(db *inceptiondb.Client) error {
	indexes := []struct {
		collection string
		options    inceptiondb.IndexOptions
	}{
		{blogsCollection, inceptiondb.IndexOptions{Name: "blogs-slug-v1", Type: "btree", Fields: []string{"slug"}, Unique: true}},
		{blogMembersCollection, inceptiondb.IndexOptions{Name: "blog-members-blog-user-v1", Type: "btree", Fields: []string{"blog_id", "user_id"}, Unique: true}},
		{blogInvitationsCollection, inceptiondb.IndexOptions{Name: "blog-invitations-token-hash-v1", Type: "map", Field: "token_hash"}},
	}
	for _, index := range indexes {
		if err := db.EnsureIndex(index.collection, &index.options); err != nil {
			return fmt.Errorf("ensure index %s on %s: %w", index.options.Name, index.collection, err)
		}
	}
	return nil
}

func RequireBlogAccess(ctx context.Context, blogId string) (*Blog, BlogRole, error) {
	auth, err := requireBlogAuth(ctx)
	if err != nil {
		return nil, "", err
	}
	blog := &Blog{}
	if err := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blogId}}, blog); err != nil {
		return nil, "", blogNotFound()
	}
	if blog.OwnerId == auth.User.ID {
		return blog, BlogRoleOwner, nil
	}
	member := &BlogMember{}
	if err := GetInceptionClient(ctx).FindOne(blogMembersCollection, inceptiondb.FindQuery{Filter: JSON{
		"id": deterministicBlogMemberId(blog.Id, auth.User.ID), "blog_id": blog.Id, "user_id": auth.User.ID,
	}}, member); err != nil || member.Role != BlogRoleEditor {
		return nil, "", blogForbidden()
	}
	return blog, BlogRoleEditor, nil
}

func RequireBlogOwner(ctx context.Context, blogId string) (*Blog, error) {
	blog, role, err := RequireBlogAccess(ctx, blogId)
	if err != nil {
		return nil, err
	}
	if role != BlogRoleOwner {
		return nil, blogForbidden()
	}
	return blog, nil
}

func EnsurePersonalBlog(ctx context.Context, auth *glueauth.GlueAuthentication) (*Blog, error) {
	if auth == nil || strings.TrimSpace(auth.User.ID) == "" {
		return nil, blogForbidden()
	}
	db := GetInceptionClient(ctx)
	blogSlugMu.Lock()
	defer blogSlugMu.Unlock()

	blogId := deterministicPersonalBlogId(auth.User.ID)
	blog := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blogId}}, blog); err == nil {
		if blog.OwnerId != auth.User.ID || !blog.Personal {
			return nil, blogStorageError("personal blog identity is incoherent", errors.New("owner or personal flag does not match"))
		}
		if err := ensureOwnerMembership(db, blog); err != nil {
			return nil, blogStorageError("ensure personal blog owner", err)
		}
		return blog, nil
	}

	slug, err := availablePersonalSlug(db, auth.User.Nick, auth.User.ID)
	if err != nil {
		return nil, blogStorageError("choose personal blog slug", err)
	}
	now := time.Now().UTC()
	name := strings.TrimSpace(auth.User.Nick)
	if name == "" {
		name = "Personal blog"
	}
	blog = &Blog{
		Id: blogId, Slug: slug, Name: name, Language: BlogLanguageES, AccentColor: "#2563EB",
		OwnerId: auth.User.ID, OwnerNick: auth.User.Nick, Personal: true,
		LegacyNicks: compactStrings([]string{auth.User.Nick}), CreatedOn: now, UpdatedOn: now,
	}
	if err := insertBlogWithOwner(db, blog); err != nil {
		return nil, err
	}
	return blog, nil
}

func insertBlogWithOwner(db *inceptiondb.Client, blog *Blog) error {
	if err := db.Insert(blogsCollection, blog); err != nil {
		if errors.Is(err, inceptiondb.ErrorAlreadyExist) {
			return err
		}
		return blogStorageError("insert blog", err)
	}
	if err := ensureOwnerMembership(db, blog); err != nil {
		removed, removeErr := db.Remove(blogsCollection, inceptiondb.FindQuery{Limit: 1, Filter: JSON{"id": blog.Id}})
		if removed != nil {
			io.Copy(io.Discard, removed)
			removed.Close()
		}
		if removeErr != nil {
			return blogStorageError("insert owner membership and compensate", errors.Join(err, removeErr))
		}
		return blogStorageError("insert owner membership", err)
	}
	return nil
}

func ensureEditorMembership(db *inceptiondb.Client, blog *Blog, userId, userNick string, createdOn time.Time) error {
	if userId == blog.OwnerId {
		return ensureOwnerMembership(db, blog)
	}
	id := deterministicBlogMemberId(blog.Id, userId)
	existing := &BlogMember{}
	if err := db.FindOne(blogMembersCollection, inceptiondb.FindQuery{Filter: JSON{
		"id": id, "blog_id": blog.Id, "user_id": userId,
	}}, existing); err == nil {
		if existing.Role == BlogRoleEditor {
			return nil
		}
		patched := &BlogMember{}
		matched, err := db.PatchOne(blogMembersCollection, inceptiondb.PatchQuery{Filter: JSON{
			"id": id, "blog_id": blog.Id, "user_id": userId,
		}, Patch: JSON{"role": BlogRoleEditor, "user_nick": userNick}}, patched)
		if err != nil {
			return err
		}
		if !matched {
			return errors.New("editor membership disappeared during repair")
		}
		return nil
	}
	member := &BlogMember{Id: id, BlogId: blog.Id, UserId: userId, UserNick: userNick, Role: BlogRoleEditor, CreatedOn: createdOn}
	return db.Insert(blogMembersCollection, member)
}

func ensureOwnerMembership(db *inceptiondb.Client, blog *Blog) error {
	member := &BlogMember{Id: deterministicBlogMemberId(blog.Id, blog.OwnerId), BlogId: blog.Id,
		UserId: blog.OwnerId, UserNick: blog.OwnerNick, Role: BlogRoleOwner, CreatedOn: blog.CreatedOn}
	existing := &BlogMember{}
	if err := db.FindOne(blogMembersCollection, inceptiondb.FindQuery{Filter: JSON{"id": member.Id}}, existing); err == nil {
		if existing.BlogId == member.BlogId && existing.UserId == member.UserId && existing.Role == member.Role {
			return nil
		}
		patched := &BlogMember{}
		matched, err := db.PatchOne(blogMembersCollection, inceptiondb.PatchQuery{
			Filter: JSON{"id": member.Id}, Patch: member}, patched)
		if err != nil {
			return err
		}
		if !matched {
			return errors.New("owner membership disappeared during repair")
		}
		return nil
	}
	if err := db.Insert(blogMembersCollection, member); err != nil && !errors.Is(err, inceptiondb.ErrorAlreadyExist) {
		return err
	}
	return nil
}

func deterministicPersonalBlogId(userId string) string {
	return "blog_personal_" + stableHex(userId, 32)
}

func deterministicBlogMemberId(blogId, userId string) string {
	return "blog_member_" + stableHex(blogId+"\x00"+userId, 64)
}

func stableHex(value string, length int) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:length]
}

func availablePersonalSlug(db *inceptiondb.Client, nick, userId string) (string, error) {
	base := normalizeBlogSlug(nick)
	candidates := []string{base}
	for _, length := range []int{8, 12, 16, 32} {
		candidates = append(candidates, slugWithSuffix(base, stableHex(userId, length)))
	}
	blogId := deterministicPersonalBlogId(userId)
	for _, candidate := range candidates {
		if IsReservedBlogSlug(candidate) {
			continue
		}
		existing := &Blog{}
		err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"slug": candidate}}, existing)
		if errors.Is(err, io.EOF) || err == nil && existing.Id == blogId {
			return candidate, nil
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
	}
	return "", errors.New("could not derive an available deterministic slug")
}

func normalizeBlogSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	lastHyphen := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			result.WriteRune(r)
			lastHyphen = false
		} else if result.Len() > 0 && !lastHyphen {
			result.WriteByte('-')
			lastHyphen = true
		}
	}
	base := truncateSlug(strings.Trim(result.String(), "-"), 63)
	if len(base) < 2 {
		return "blog"
	}
	return base
}

func truncateSlug(slug string, limit int) string {
	if len(slug) > limit {
		slug = slug[:limit]
	}
	return strings.TrimRight(slug, "-")
}

func slugWithSuffix(base, suffix string) string {
	return truncateSlug(base, 63-len(suffix)-1) + "-" + suffix
}

func compactStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func requireBlogAuth(ctx context.Context) (*glueauth.GlueAuthentication, error) {
	auth := glueauth.GetAuth(ctx)
	if auth == nil || strings.TrimSpace(auth.User.ID) == "" {
		return nil, blogForbidden()
	}
	return auth, nil
}

func blogNotFound() HttpError {
	return HttpError{Status: http.StatusNotFound, Title: "Blog Not Found", Description: "El blog no existe"}
}

func blogForbidden() HttpError {
	return HttpError{Status: http.StatusForbidden, Title: "Blog Access Denied", Description: "No tienes acceso a este blog"}
}

func invalidBlog(description string) HttpError {
	return HttpError{Status: http.StatusBadRequest, Title: "Invalid Blog", Description: description}
}

func blogStorageError(operation string, err error) HttpError {
	log.Printf("blog storage error [%s]: %v", operation, err)
	return HttpError{Status: http.StatusInternalServerError, Title: "Blog Storage Error", Description: "No se pudo completar la operación", Details: operation}
}
