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
	for _, collection := range []string{blogsCollection, blogInvitationsCollection} {
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
		{blogsCollection, inceptiondb.IndexOptions{Name: "blogs-id-v1", Type: "btree", Fields: []string{"id"}, Unique: true}},
		{blogsCollection, inceptiondb.IndexOptions{Name: "blogs-name-v1", Type: "btree", Fields: []string{"name", "id"}, Unique: true}},
		{blogInvitationsCollection, inceptiondb.IndexOptions{Name: "blog-invitations-token-hash-v1", Type: "map", Field: "token_hash"}},
		{blogInvitationsCollection, inceptiondb.IndexOptions{Name: "blog-invitations-created-v2", Type: "btree", Fields: []string{"createdOn", "id"}, Unique: true}},
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
	if blog.Banned {
		return nil, "", blogNotFound()
	}
	member := findBlogMember(blog.Members, auth.User.ID)
	if member == nil || member.Role != BlogRoleEditor {
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
	blog.Members = []BlogMember{canonicalOwnerMember(blog)}
	blog.MembersRevision = 1
	if err := db.Insert(blogsCollection, blog); err != nil {
		if errors.Is(err, inceptiondb.ErrorAlreadyExist) {
			return err
		}
		return blogStorageError("insert blog", err)
	}
	return nil
}

func ensureEditorMembership(db *inceptiondb.Client, blog *Blog, userId, userNick string, createdOn time.Time) (bool, bool, error) {
	if userId == blog.OwnerId {
		return false, false, ensureOwnerMembership(db, blog)
	}
	created := false
	alreadyEditor := false
	updated, err := mutateBlogMembers(db, blog.Id, func(current *Blog) (bool, error) {
		created = false
		alreadyEditor = false
		member := findBlogMember(current.Members, userId)
		if member != nil && member.Role == BlogRoleEditor {
			alreadyEditor = true
			id := deterministicBlogMemberId(current.Id, userId)
			if member.Id == id && member.BlogId == current.Id {
				return false, nil
			}
			member.Id = id
			member.BlogId = current.Id
			return true, nil
		}
		current.Members = removeBlogMemberByUser(current.Members, userId)
		current.Members = append(current.Members, BlogMember{Id: deterministicBlogMemberId(current.Id, userId), BlogId: current.Id,
			UserId: userId, UserNick: userNick, Role: BlogRoleEditor, CreatedOn: createdOn})
		created = member == nil
		return true, nil
	})
	if updated != nil {
		*blog = *updated
	}
	return created, alreadyEditor, err
}

func ensureOwnerMembership(db *inceptiondb.Client, blog *Blog) error {
	updated, err := mutateBlogMembers(db, blog.Id, func(current *Blog) (bool, error) {
		canonical, err := canonicalizeBlogMembers(current, current.Members, true)
		if err != nil {
			return false, err
		}
		if blogMembersEqual(current.Members, canonical) {
			return false, nil
		}
		current.Members = canonical
		return true, nil
	})
	if updated != nil {
		*blog = *updated
	}
	return err
}

const maxBlogMemberCASAttempts = 32

var errBlogMemberCASConflict = errors.New("blog membership changed too many times")

func mutateBlogMembers(db *inceptiondb.Client, blogId string, mutation func(*Blog) (bool, error)) (*Blog, error) {
	for attempt := 0; attempt < maxBlogMemberCASAttempts; attempt++ {
		current := &Blog{}
		if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blogId}}, current); err != nil {
			return nil, err
		}
		changed, err := mutation(current)
		if err != nil {
			return nil, err
		}
		if !changed {
			return current, nil
		}
		if len(current.Members) > maxBlogMembers {
			return nil, errors.New("blog has too many members")
		}
		sort.Slice(current.Members, func(i, j int) bool { return current.Members[i].Id < current.Members[j].Id })
		filter := JSON{"id": current.Id, "members_revision": current.MembersRevision}
		if current.MembersRevision == 0 {
			filter = JSON{"id": current.Id, "$or": []JSON{{"members_revision": 0}, {"members_revision": nil}}}
		}
		patched := &Blog{}
		matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: filter, Patch: JSON{
			"members": current.Members, "members_revision": current.MembersRevision + 1,
		}}, patched)
		if err != nil {
			return nil, err
		}
		if matched {
			return patched, nil
		}
	}
	return nil, errBlogMemberCASConflict
}

func canonicalOwnerMember(blog *Blog) BlogMember {
	return BlogMember{Id: deterministicBlogMemberId(blog.Id, blog.OwnerId), BlogId: blog.Id,
		UserId: blog.OwnerId, UserNick: blog.OwnerNick, Role: BlogRoleOwner, CreatedOn: blog.CreatedOn}
}

func findBlogMember(members []BlogMember, userId string) *BlogMember {
	for i := range members {
		if members[i].UserId == userId {
			return &members[i]
		}
	}
	return nil
}

func removeBlogMemberByUser(members []BlogMember, userId string) []BlogMember {
	result := make([]BlogMember, 0, len(members))
	for _, member := range members {
		if member.UserId != userId {
			result = append(result, member)
		}
	}
	return result
}

func canonicalizeBlogMembers(blog *Blog, members []BlogMember, rejectDuplicates bool) ([]BlogMember, error) {
	result := []BlogMember{canonicalOwnerMember(blog)}
	seen := map[string]bool{blog.OwnerId: true}
	for _, member := range members {
		if member.UserId == "" || member.UserId == blog.OwnerId || member.Role != BlogRoleEditor {
			continue
		}
		if seen[member.UserId] {
			if rejectDuplicates {
				return nil, fmt.Errorf("duplicate membership for blog %q and user %q", blog.Id, member.UserId)
			}
			continue
		}
		seen[member.UserId] = true
		member.Id = deterministicBlogMemberId(blog.Id, member.UserId)
		member.BlogId = blog.Id
		member.Role = BlogRoleEditor
		result = append(result, member)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Id < result[j].Id })
	return result, nil
}

func blogMembersEqual(left, right []BlogMember) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
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
