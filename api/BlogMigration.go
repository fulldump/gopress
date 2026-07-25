package api

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopress/inceptiondb"
)

const maxBlogMigrationRows = int(^uint(0) >> 1)

type BlogMigrationReport struct {
	ArticlesScanned    int      `json:"articlesScanned"`
	AuthorsFound       int      `json:"authorsFound"`
	BlogsCreated       int      `json:"blogsCreated"`
	BlogsRepaired      int      `json:"blogsRepaired"`
	MembershipsCreated int      `json:"membershipsCreated"`
	ArticlesPatched    int      `json:"articlesPatched"`
	ArticlesUnchanged  int      `json:"articlesUnchanged"`
	Errors             []string `json:"errors"`
}

type FileBlogMigrationReport struct {
	FilesScanned   int `json:"filesScanned"`
	BlogsCreated   int `json:"blogsCreated"`
	FilesPatched   int `json:"filesPatched"`
	FilesUnchanged int `json:"filesUnchanged"`
}

type migrationArticle struct {
	Id         string    `json:"id"`
	AuthorId   string    `json:"author_id"`
	AuthorNick string    `json:"author_nick"`
	CreatedOn  time.Time `json:"createdOn"`
	BlogId     string    `json:"blog_id"`
	BlogSlug   string    `json:"blog_slug"`
	Revision   uint64    `json:"revision"`
}

type migrationAuthor struct {
	UserId   string
	Articles []*migrationArticle
}

func MigrateBlogs(db *inceptiondb.Client) (*BlogMigrationReport, error) {
	report := &BlogMigrationReport{Errors: []string{}}
	if err := InitializeBlogCollections(db); err != nil {
		return report, err
	}
	blogSlugMu.Lock()
	defer blogSlugMu.Unlock()

	articles := []*migrationArticle{}
	if err := db.FindAll("articles", inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(article *migrationArticle) {
		articles = append(articles, article)
	}); err != nil {
		return report, fmt.Errorf("scan articles: %w", err)
	}
	report.ArticlesScanned = len(articles)

	blogs := []*Blog{}
	if err := db.FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(blog *Blog) {
		blogs = append(blogs, blog)
	}); err != nil {
		return report, fmt.Errorf("scan blogs: %w", err)
	}
	blogsById := map[string]*Blog{}
	occupiedSlugs := map[string]string{}
	for _, blog := range blogs {
		if blog.Id == "" || blog.OwnerId == "" {
			return migrationFailure(report, fmt.Errorf("blog %q has an empty id or owner_id", blog.Id))
		}
		if previous, exists := blogsById[blog.Id]; exists && previous != blog {
			return migrationFailure(report, fmt.Errorf("duplicate blog id %q", blog.Id))
		}
		blogsById[blog.Id] = blog
		if blog.Slug != "" {
			if other, exists := occupiedSlugs[blog.Slug]; exists && other != blog.Id {
				return migrationFailure(report, fmt.Errorf("duplicate blog slug %q", blog.Slug))
			}
			occupiedSlugs[blog.Slug] = blog.Id
		}
	}

	authors := map[string]*migrationAuthor{}
	seenArticleIds := map[string]bool{}
	for _, article := range articles {
		article.AuthorId = strings.TrimSpace(article.AuthorId)
		if article.Id == "" || article.AuthorId == "" || article.CreatedOn.IsZero() {
			return migrationFailure(report, fmt.Errorf("article %q has an empty id, author_id, or createdOn", article.Id))
		}
		if seenArticleIds[article.Id] {
			return migrationFailure(report, fmt.Errorf("duplicate article id %q", article.Id))
		}
		seenArticleIds[article.Id] = true
		author := authors[article.AuthorId]
		if author == nil {
			author = &migrationAuthor{UserId: article.AuthorId}
			authors[article.AuthorId] = author
		}
		author.Articles = append(author.Articles, article)
	}
	report.AuthorsFound = len(authors)

	userIds := make([]string, 0, len(authors))
	for userId := range authors {
		userIds = append(userIds, userId)
	}
	sort.Strings(userIds)
	for _, userId := range userIds {
		author := authors[userId]
		sort.Slice(author.Articles, func(i, j int) bool {
			if author.Articles[i].CreatedOn.Equal(author.Articles[j].CreatedOn) {
				return author.Articles[i].Id < author.Articles[j].Id
			}
			return author.Articles[i].CreatedOn.After(author.Articles[j].CreatedOn)
		})
		personalId := deterministicPersonalBlogId(userId)
		needsPersonal := false
		for _, article := range author.Articles {
			if article.BlogId == "" || article.BlogId == personalId {
				needsPersonal = true
				break
			}
		}
		if needsPersonal {
			blog, created, repaired, memberCreated, err := ensureMigratedPersonalBlog(db, author, blogsById, occupiedSlugs)
			if err != nil {
				return migrationFailure(report, fmt.Errorf("author %q: %w", userId, err))
			}
			if created {
				report.BlogsCreated++
			}
			if repaired {
				report.BlogsRepaired++
			}
			if memberCreated {
				report.MembershipsCreated++
			}
			blogsById[blog.Id] = blog
			occupiedSlugs[blog.Slug] = blog.Id
		}
	}
	blogIds := make([]string, 0, len(blogsById))
	for blogId := range blogsById {
		blogIds = append(blogIds, blogId)
	}
	sort.Strings(blogIds)
	for _, blogId := range blogIds {
		blog := blogsById[blogId]
		if err := validatePersistedBlog(blog); err != nil {
			return migrationFailure(report, fmt.Errorf("blog %q is invalid: %w", blog.Id, err))
		}
		if blog.Personal && blog.Id != deterministicPersonalBlogId(blog.OwnerId) {
			return migrationFailure(report, fmt.Errorf("personal blog %q has a non-deterministic id", blog.Id))
		}
		member := &BlogMember{}
		memberMissing := db.FindOne(blogMembersCollection, inceptiondb.FindQuery{Filter: JSON{
			"id": deterministicBlogMemberId(blog.Id, blog.OwnerId),
		}}, member) != nil
		if err := ensureOwnerMembership(db, blog); err != nil {
			return migrationFailure(report, fmt.Errorf("ensure owner for blog %q: %w", blog.Id, err))
		}
		if memberMissing {
			report.MembershipsCreated++
		}
	}

	for _, article := range articles {
		target := blogsById[article.BlogId]
		if article.BlogId == "" || article.BlogId == deterministicPersonalBlogId(article.AuthorId) {
			target = blogsById[deterministicPersonalBlogId(article.AuthorId)]
		}
		if target == nil {
			return migrationFailure(report, fmt.Errorf("article %q references missing blog %q", article.Id, article.BlogId))
		}
		if err := validatePersistedBlog(target); err != nil {
			return migrationFailure(report, fmt.Errorf("article %q references invalid blog: %w", article.Id, err))
		}
		patch := JSON{}
		if article.BlogId != target.Id {
			patch["blog_id"] = target.Id
		}
		if article.BlogSlug != target.Slug {
			patch["blog_slug"] = target.Slug
		}
		if article.Revision < 1 {
			patch["revision"] = uint64(1)
		}
		if len(patch) == 0 {
			report.ArticlesUnchanged++
			continue
		}
		var patched migrationArticle
		matched, err := db.PatchOne("articles", inceptiondb.PatchQuery{Filter: JSON{
			"id": article.Id, "author_id": article.AuthorId, "createdOn": article.CreatedOn,
		}, Patch: patch}, &patched)
		if err != nil || !matched {
			if err == nil {
				err = errors.New("patch returned no article")
			}
			return migrationFailure(report, fmt.Errorf("patch article %q: %w", article.Id, err))
		}
		report.ArticlesPatched++
	}
	return report, nil
}

func MigrateFileBlogIds(db *inceptiondb.Client) (*FileBlogMigrationReport, error) {
	report := &FileBlogMigrationReport{}
	blogSlugMu.Lock()
	defer blogSlugMu.Unlock()

	files := []*File{}
	if err := db.FindAll("files", inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(file *File) {
		files = append(files, file)
	}); err != nil {
		return report, fmt.Errorf("scan files: %w", err)
	}
	report.FilesScanned = len(files)
	blogsById := map[string]*Blog{}
	occupiedSlugs := map[string]string{}
	if err := db.FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(blog *Blog) {
		blogsById[blog.Id] = blog
		occupiedSlugs[blog.Slug] = blog.Id
	}); err != nil {
		return report, fmt.Errorf("scan blogs for files: %w", err)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Id < files[j].Id })
	for _, file := range files {
		if file.Id == "" || strings.TrimSpace(file.AuthorId) == "" {
			return report, fmt.Errorf("file %q has an empty id or author_id", file.Id)
		}
		if file.BlogId != "" {
			if blogsById[file.BlogId] == nil {
				return report, fmt.Errorf("file %q references unknown blog %q", file.Id, file.BlogId)
			}
			report.FilesUnchanged++
			continue
		}
		author := &migrationAuthor{UserId: file.AuthorId, Articles: []*migrationArticle{{
			Id: file.Id, AuthorId: file.AuthorId, AuthorNick: file.AuthorNick, CreatedOn: file.CreatedOn,
		}}}
		if author.Articles[0].CreatedOn.IsZero() {
			author.Articles[0].CreatedOn = time.Now().UTC()
		}
		blog, created, _, _, err := ensureMigratedPersonalBlog(db, author, blogsById, occupiedSlugs)
		if err != nil {
			return report, fmt.Errorf("file %q: ensure personal blog: %w", file.Id, err)
		}
		if created {
			report.BlogsCreated++
			blogsById[blog.Id] = blog
			occupiedSlugs[blog.Slug] = blog.Id
		}
		patched := &File{}
		matched, err := db.PatchOne("files", inceptiondb.PatchQuery{Filter: JSON{
			"id": file.Id, "author_id": file.AuthorId,
		}, Patch: JSON{"blog_id": blog.Id}}, patched)
		if err != nil {
			return report, fmt.Errorf("patch file %q: %w", file.Id, err)
		}
		if !matched {
			return report, fmt.Errorf("patch file %q returned no row", file.Id)
		}
		report.FilesPatched++
	}
	return report, nil
}

func MigrateCommentArticleVersions(db *inceptiondb.Client) (int, error) {
	articles := map[string]time.Time{}
	if err := db.FindAll("articles", inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(article *migrationArticle) {
		articles[article.Id] = article.CreatedOn
	}); err != nil {
		return 0, fmt.Errorf("scan articles for comment migration: %w", err)
	}
	comments := []*Comment{}
	if err := db.FindAll(commentsCollection, inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(comment *Comment) {
		if comment.ArticleCreatedOn.IsZero() {
			comments = append(comments, comment)
		}
	}); err != nil {
		return 0, fmt.Errorf("scan comments for migration: %w", err)
	}
	patchedCount := 0
	for _, comment := range comments {
		createdOn, exists := articles[comment.ArticleId]
		if !exists || createdOn.IsZero() {
			continue
		}
		patched := &Comment{}
		matched, err := db.PatchOne(commentsCollection, inceptiondb.PatchQuery{Filter: JSON{
			"id": comment.Id, "articleId": comment.ArticleId,
		}, Patch: JSON{"articleCreatedOn": createdOn}}, patched)
		if err != nil {
			return patchedCount, fmt.Errorf("patch comment %q: %w", comment.Id, err)
		}
		if matched {
			patchedCount++
		}
	}
	return patchedCount, nil
}

func ensureMigratedPersonalBlog(db *inceptiondb.Client, author *migrationAuthor, blogsById map[string]*Blog, occupied map[string]string) (*Blog, bool, bool, bool, error) {
	blogId := deterministicPersonalBlogId(author.UserId)
	blog := blogsById[blogId]
	primaryNick := ""
	nicks := []string{}
	var earliest time.Time
	for _, article := range author.Articles {
		nick := strings.TrimSpace(article.AuthorNick)
		if primaryNick == "" && nick != "" {
			primaryNick = nick
		}
		nicks = append(nicks, nick)
		if earliest.IsZero() || article.CreatedOn.Before(earliest) {
			earliest = article.CreatedOn.UTC()
		}
	}
	if blog == nil {
		slug, err := deterministicMigrationSlug(primaryNick, author.UserId, occupied)
		if err != nil {
			return nil, false, false, false, err
		}
		name := primaryNick
		if name == "" {
			name = "Personal blog"
		}
		blog = &Blog{Id: blogId, Slug: slug, Name: name, Language: BlogLanguageES, AccentColor: "#2563EB",
			OwnerId: author.UserId, OwnerNick: primaryNick, Personal: true, LegacyNicks: compactStrings(nicks),
			CreatedOn: earliest, UpdatedOn: earliest}
		if err := insertBlogWithOwner(db, blog); err != nil {
			return nil, false, false, false, err
		}
		return blog, true, false, true, nil
	}
	if blog.OwnerId != author.UserId || !blog.Personal {
		return nil, false, false, false, errors.New("deterministic personal blog has a different owner or is not personal")
	}
	updated := *blog
	if updated.Slug == "" {
		slug, err := deterministicMigrationSlug(primaryNick, author.UserId, occupied)
		if err != nil {
			return nil, false, false, false, err
		}
		updated.Slug = slug
	}
	if err := ValidateBlogSlug(updated.Slug); err != nil {
		return nil, false, false, false, fmt.Errorf("invalid personal blog slug: %w", err)
	}
	if other := occupied[updated.Slug]; other != "" && other != blog.Id {
		return nil, false, false, false, fmt.Errorf("personal blog slug %q is occupied", updated.Slug)
	}
	if updated.Name == "" {
		updated.Name = primaryNick
		if updated.Name == "" {
			updated.Name = "Personal blog"
		}
	}
	if updated.Language == "" {
		updated.Language = BlogLanguageES
	}
	if updated.AccentColor == "" {
		updated.AccentColor = "#2563EB"
	}
	if updated.OwnerNick == "" {
		updated.OwnerNick = primaryNick
	}
	updated.LegacyNicks = compactStrings(append(updated.LegacyNicks, nicks...))
	if updated.CreatedOn.IsZero() {
		updated.CreatedOn = earliest
	}
	if updated.UpdatedOn.IsZero() {
		updated.UpdatedOn = updated.CreatedOn
	}
	if err := validatePersistedBlog(&updated); err != nil {
		return nil, false, false, false, err
	}
	repaired := updated.Slug != blog.Slug || updated.Name != blog.Name || updated.Language != blog.Language || updated.AccentColor != blog.AccentColor ||
		updated.OwnerNick != blog.OwnerNick || updated.CreatedOn != blog.CreatedOn || updated.UpdatedOn != blog.UpdatedOn ||
		strings.Join(updated.LegacyNicks, "\x00") != strings.Join(blog.LegacyNicks, "\x00")
	if repaired {
		patched := &Blog{}
		matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{
			"id": blog.Id, "owner_id": author.UserId, "personal": true,
		}, Patch: &updated}, patched)
		if err != nil || !matched {
			if err == nil {
				err = errors.New("patch returned no blog")
			}
			return nil, false, false, false, err
		}
		blog = patched
	}
	member := &BlogMember{}
	memberMissing := db.FindOne(blogMembersCollection, inceptiondb.FindQuery{Filter: JSON{"id": deterministicBlogMemberId(blog.Id, blog.OwnerId)}}, member) != nil
	if err := ensureOwnerMembership(db, blog); err != nil {
		return nil, false, false, false, err
	}
	return blog, false, repaired, memberMissing, nil
}

func deterministicMigrationSlug(nick, userId string, occupied map[string]string) (string, error) {
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
		if owner := occupied[candidate]; owner == "" || owner == blogId {
			return candidate, nil
		}
	}
	return "", errors.New("could not derive an available deterministic slug")
}

func validatePersistedBlog(blog *Blog) error {
	if blog.Id == "" || blog.OwnerId == "" || blog.CreatedOn.IsZero() || blog.UpdatedOn.IsZero() {
		return errors.New("id, owner_id, createdOn, and updatedOn are required")
	}
	if err := ValidateBlogSlug(blog.Slug); err != nil {
		return err
	}
	return validateBlogFields(blog.Name, blog.Description, blog.Language, blog.AccentColor, blog.HeaderImage)
}

func migrationFailure(report *BlogMigrationReport, err error) (*BlogMigrationReport, error) {
	report.Errors = append(report.Errors, err.Error())
	return report, err
}
