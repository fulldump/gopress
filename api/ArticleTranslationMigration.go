package api

import (
	"time"

	"gopress/inceptiondb"
)

func MigrateArticleLocalization(db *inceptiondb.Client) (int, error) {
	blogs := map[string]*Blog{}
	if err := db.FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(blog *Blog) {
		blogs[blog.Id] = blog
	}); err != nil {
		return 0, err
	}
	articles := []*Article{}
	if err := db.FindAll("articles", inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(article *Article) {
		articles = append(articles, article)
	}); err != nil {
		return 0, err
	}
	patchedCount := 0
	for _, article := range articles {
		patch := JSON{}
		if article.SourceLanguage == "" {
			if blog := blogs[article.BlogId]; blog != nil {
				patch["source_language"] = blog.Language
			}
		}
		if article.ContentRevision == 0 {
			patch["content_revision"] = uint64(1)
		}
		if article.UpdatedOn.IsZero() {
			patch["updatedOn"] = time.Now().UTC()
		}
		if len(patch) == 0 {
			continue
		}
		matched, err := db.PatchOne("articles", inceptiondb.PatchQuery{Filter: articleIdentityFilter(article), Patch: patch}, &Article{})
		if err != nil {
			return patchedCount, err
		}
		if matched {
			patchedCount++
		}
	}
	return patchedCount, nil
}
