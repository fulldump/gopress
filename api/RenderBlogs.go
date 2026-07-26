package api

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
	"gopress/templates"
)

func RenderBlogs(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	offset, err := listOffset(r)
	if err != nil {
		return err
	}
	db := GetInceptionClient(ctx)
	blogs := make([]*Blog, 0, listPageSize+1)
	preferred := preferredContentLanguage(ctx)
	logicalIndex := 0
	rawOffset := 0
	for len(blogs) <= listPageSize && rawOffset < maxBlogs {
		batch := []*Blog{}
		batchLimit := min(listPageSize+1, maxBlogs-rawOffset)
		if err := db.FindAll(blogsCollection, inceptiondb.FindQuery{
			Index: "blogs-name-v1", Skip: rawOffset, Limit: batchLimit, Filter: JSON{"$ne": JSON{"banned": true}},
		}, func(blog *Blog) { batch = append(batch, blog) }); err != nil {
			return blogStorageError("render blogs", err)
		}
		if len(batch) == 0 {
			break
		}
		rawOffset += len(batch)
		for _, blog := range batch {
			article := &Article{}
			if err := db.FindOne("articles", inceptiondb.FindQuery{Filter: JSON{
				"blog_id": blog.Id, "published": true, "$ne": JSON{"banned": true},
			}}, article); err != nil {
				continue
			}
			if logicalIndex < offset {
				logicalIndex++
				continue
			}
			blogs = append(blogs, preparePublicBlog(blog, preferred))
			logicalIndex++
			if len(blogs) > listPageSize {
				break
			}
		}
		if len(batch) < batchLimit {
			break
		}
	}
	page := pageFromLookahead(blogs, offset)
	if err := templates.Execute(ctx, "blogs", w, JSON{
		"blogs": page.Items, "nextURL": nextPageURL(r, page.NextOffset, page.HasMore),
	}); err != nil {
		return fmt.Errorf("error rendering blogs: %w", err)
	}
	return nil
}

func RedirectBlogsSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/blogs", http.StatusPermanentRedirect)
}

func RenderBlog(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	blog, err := getPublicBlogBySlug(ctx, box.GetUrlParameter(ctx, "blogSlug"))
	if err != nil {
		return err
	}
	return renderBlogFeed(ctx, w, r, blog, "")
}

func RenderBlogTag(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	blog, err := getPublicBlogBySlug(ctx, box.GetUrlParameter(ctx, "blogSlug"))
	if err != nil {
		return err
	}
	w.Header().Set("X-Robots-Tag", "noindex")
	return renderBlogFeed(ctx, w, r, blog, box.GetUrlParameter(ctx, "tag"))
}

func renderBlogFeed(ctx context.Context, w http.ResponseWriter, r *http.Request, blog *Blog, tag string) error {
	offset, err := listOffset(r)
	if err != nil {
		return err
	}
	filter := JSON{"blog_id": blog.Id, "published": true, "$ne": JSON{"banned": true}}
	if tag != "" {
		filter["tags"] = tag
	}
	page, err := publicArticlePage(ctx, filter, offset)
	if err != nil {
		return blogStorageError("render blog articles", err)
	}
	if tag != "" && offset == 0 && len(page.Items) == 0 {
		return HttpError{Status: http.StatusNotFound, Title: "Tag Not Found", Description: "La etiqueta no existe en este blog"}
	}
	if err := templates.Execute(ctx, "blog", w, JSON{
		"blog": preparePublicBlog(blog, preferredContentLanguage(ctx)), "articles": page.Items, "tag": tag,
		"nextURL": nextPageURL(r, page.NextOffset, page.HasMore),
	}); err != nil {
		return fmt.Errorf("error rendering blog: %w", err)
	}
	return nil
}

func getPublicBlogBySlug(ctx context.Context, slug string) (*Blog, error) {
	blog := &Blog{}
	if err := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{
		"slug": slug, "$ne": JSON{"banned": true},
	}}, blog); err != nil {
		return nil, blogNotFound()
	}
	return blog, nil
}

func publicBlogIds(ctx context.Context) map[string]bool {
	result := map[string]bool{}
	_ = GetInceptionClient(ctx).FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogs, Filter: JSON{
		"$ne": JSON{"banned": true},
	}}, func(blog *Blog) {
		result[blog.Id] = true
	})
	return result
}

func isPublicBlog(ctx context.Context, blogId string) bool {
	blog := &Blog{}
	return GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{
		"id": blogId, "$ne": JSON{"banned": true},
	}}, blog) == nil
}

func ensureArticleSummary(article *Article) {
	if article.ContentSummary == "" {
		article.ContentSummary = template.HTML(HtmlSummary(string(article.ContentHTML), 50))
	}
}
