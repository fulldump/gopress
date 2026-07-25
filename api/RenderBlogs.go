package api

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
	"gopress/templates"
)

func RenderBlogs(ctx context.Context, w http.ResponseWriter) error {
	blogs := []*Blog{}
	if err := GetInceptionClient(ctx).FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogs}, func(blog *Blog) {
		blogs = append(blogs, blog)
	}); err != nil {
		return blogStorageError("render blogs", err)
	}
	sort.SliceStable(blogs, func(i, j int) bool {
		return strings.ToLower(blogs[i].Name) < strings.ToLower(blogs[j].Name)
	})
	if err := templates.Execute(ctx, "blogs", w, JSON{"blogs": blogs}); err != nil {
		return fmt.Errorf("error rendering blogs: %w", err)
	}
	return nil
}

func RedirectBlogsSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/blogs", http.StatusPermanentRedirect)
}

func RenderBlog(ctx context.Context, w http.ResponseWriter) error {
	blog, err := getPublicBlogBySlug(ctx, box.GetUrlParameter(ctx, "blogSlug"))
	if err != nil {
		return err
	}
	return renderBlogFeed(ctx, w, blog, "")
}

func RenderBlogTag(ctx context.Context, w http.ResponseWriter) error {
	blog, err := getPublicBlogBySlug(ctx, box.GetUrlParameter(ctx, "blogSlug"))
	if err != nil {
		return err
	}
	w.Header().Set("X-Robots-Tag", "noindex")
	return renderBlogFeed(ctx, w, blog, box.GetUrlParameter(ctx, "tag"))
}

func renderBlogFeed(ctx context.Context, w http.ResponseWriter, blog *Blog, tag string) error {
	filter := JSON{"blog_id": blog.Id, "published": true, "$ne": JSON{"banned": true}}
	if tag != "" {
		filter["tags"] = tag
	}
	articles := []*Article{}
	if err := GetInceptionClient(ctx).FindAll("articles", inceptiondb.FindQuery{Limit: 1000, Filter: filter}, func(article *Article) {
		ensureArticleSummary(article)
		articles = append(articles, article)
	}); err != nil {
		return blogStorageError("render blog articles", err)
	}
	if tag != "" && len(articles) == 0 {
		return HttpError{Status: http.StatusNotFound, Title: "Tag Not Found", Description: "La etiqueta no existe en este blog"}
	}
	sort.SliceStable(articles, func(i, j int) bool { return articles[i].PublishOn.After(articles[j].PublishOn) })
	if err := templates.Execute(ctx, "blog", w, JSON{"blog": blog, "articles": articles, "tag": tag}); err != nil {
		return fmt.Errorf("error rendering blog: %w", err)
	}
	return nil
}

func getPublicBlogBySlug(ctx context.Context, slug string) (*Blog, error) {
	blog := &Blog{}
	if err := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"slug": slug}}, blog); err != nil {
		return nil, blogNotFound()
	}
	return blog, nil
}

func ensureArticleSummary(article *Article) {
	if article.ContentSummary == "" {
		article.ContentSummary = template.HTML(HtmlSummary(string(article.ContentHTML), 50))
	}
}
