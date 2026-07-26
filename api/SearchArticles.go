package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"gopress/inceptiondb"
)

const maxPublicSearchCandidates = 10000

type PublicArticleSearchResult struct {
	Id                 string         `json:"id"`
	Title              string         `json:"title"`
	Url                string         `json:"url"`
	PublicUrl          string         `json:"publicUrl"`
	BlogName           string         `json:"blogName"`
	BlogSlug           string         `json:"blogSlug"`
	AuthorNick         string         `json:"authorNick"`
	Tags               []string       `json:"tags"`
	PublishOn          time.Time      `json:"publishOn"`
	AvailableLanguages []LanguageLink `json:"availableLanguages"`
}

func SearchPublicArticles(ctx context.Context, r *http.Request) any {
	offset, err := listOffset(r)
	if err != nil {
		return err
	}
	query := normalizePublicSearch(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 {
		items := []PublicArticleSearchResult{}
		return JSON{"items": items, "hasMore": false, "nextOffset": 0, "articles": items}
	}
	db := GetInceptionClient(ctx)
	blogs := map[string]*Blog{}
	missingBlogs := map[string]bool{}
	results := make([]PublicArticleSearchResult, 0, listPageSize+1)
	preferred := preferredContentLanguage(ctx)
	logicalIndex := 0
	rawOffset := 0
	for len(results) <= listPageSize && rawOffset < maxPublicSearchCandidates {
		batch := []*Article{}
		batchLimit := min(listPageSize+1, maxPublicSearchCandidates-rawOffset)
		err = db.FindAll("articles", inceptiondb.FindQuery{
			Index: "articles-public-publish-v2", Skip: rawOffset, Limit: batchLimit, Reverse: true,
			Filter: JSON{"published": true, "$ne": JSON{"banned": true}},
		}, func(article *Article) { batch = append(batch, article) })
		if err != nil {
			return blogStorageError("search articles", err)
		}
		if len(batch) == 0 {
			break
		}
		rawOffset += len(batch)
		for _, article := range batch {
			blog := blogs[article.BlogId]
			if blog == nil && !missingBlogs[article.BlogId] {
				candidate := &Blog{}
				if findErr := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{
					"id": article.BlogId, "$ne": JSON{"banned": true},
				}}, candidate); findErr != nil {
					missingBlogs[article.BlogId] = true
				} else {
					blog = candidate
					blogs[article.BlogId] = candidate
				}
			}
			if blog == nil || !articleMatchesSearch(article, blog, query) {
				continue
			}
			if logicalIndex < offset {
				logicalIndex++
				continue
			}
			localized := preparePublicArticle(article, blog, preferred)
			results = append(results, PublicArticleSearchResult{
				Id: article.Id, Title: localized.Title, Url: article.Url, PublicUrl: localized.PublicUrl,
				BlogName: blog.Name, BlogSlug: blog.Slug, AuthorNick: article.AuthorNick,
				Tags: article.Tags, PublishOn: article.PublishOn, AvailableLanguages: localized.AvailableLanguages,
			})
			logicalIndex++
			if len(results) > listPageSize {
				break
			}
		}
		if len(batch) < batchLimit {
			break
		}
	}
	page := pageFromLookahead(results, offset)
	return JSON{"items": page.Items, "hasMore": page.HasMore, "nextOffset": page.NextOffset, "articles": page.Items}
}

func articleMatchesSearch(article *Article, blog *Blog, query string) bool {
	values := []string{article.Title, article.AuthorNick, blog.Name, blog.Slug, strings.Join(article.Tags, " ")}
	for _, translation := range article.Translations {
		if translation.SourceContentRevision == article.ContentRevision && blogOffersLanguage(blog, translation.Language) {
			values = append(values, translation.Title)
		}
	}
	for _, value := range values {
		if strings.Contains(normalizePublicSearch(value), query) {
			return true
		}
	}
	return false
}

func normalizePublicSearch(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	).Replace(value)
}
