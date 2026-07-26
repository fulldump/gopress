package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"gopress/inceptiondb"
)

const (
	maxArticleSearchResults = 20
	maxPublicDiscoveryRows  = 10000
)

type PublicArticleSearchResult struct {
	Title      string    `json:"title"`
	Url        string    `json:"url"`
	PublicUrl  string    `json:"publicUrl"`
	BlogName   string    `json:"blogName"`
	BlogSlug   string    `json:"blogSlug"`
	AuthorNick string    `json:"authorNick"`
	Tags       []string  `json:"tags"`
	PublishOn  time.Time `json:"publishOn"`
}

func SearchPublicArticles(ctx context.Context, r *http.Request) any {
	query := normalizePublicSearch(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 {
		return JSON{"articles": []PublicArticleSearchResult{}}
	}
	db := GetInceptionClient(ctx)
	blogs := map[string]*Blog{}
	if err := db.FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogs, Filter: JSON{
		"$ne": JSON{"banned": true},
	}}, func(blog *Blog) {
		blogs[blog.Id] = blog
	}); err != nil {
		return blogStorageError("search article blogs", err)
	}

	results := []PublicArticleSearchResult{}
	err := db.FindAll("articles", inceptiondb.FindQuery{Limit: maxPublicDiscoveryRows, Filter: JSON{
		"published": true, "$ne": JSON{"banned": true},
	}}, func(article *Article) {
		blog := blogs[article.BlogId]
		if blog == nil || !articleMatchesSearch(article, blog, query) {
			return
		}
		results = append(results, PublicArticleSearchResult{
			Title: article.Title, Url: article.Url, PublicUrl: articlePublicPath(blog.Slug, article.Url),
			BlogName: blog.Name, BlogSlug: blog.Slug, AuthorNick: article.AuthorNick,
			Tags: article.Tags, PublishOn: article.PublishOn,
		})
		if len(results) > maxArticleSearchResults {
			sort.SliceStable(results, func(i, j int) bool { return results[i].PublishOn.After(results[j].PublishOn) })
			results = results[:maxArticleSearchResults]
		}
	})
	if err != nil {
		return blogStorageError("search articles", err)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].PublishOn.After(results[j].PublishOn) })
	return JSON{"articles": results}
}

func articleMatchesSearch(article *Article, blog *Blog, query string) bool {
	values := []string{article.Title, article.AuthorNick, blog.Name, blog.Slug, strings.Join(article.Tags, " ")}
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
