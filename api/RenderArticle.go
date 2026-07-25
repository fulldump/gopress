package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
	"gopress/templates"
)

func RenderArticle(w http.ResponseWriter, ctx context.Context, r *http.Request) error {
	article := &Article{}
	err := GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{Filter: JSON{
		"author_nick": box.GetUrlParameter(ctx, "userNick"),
		"url":         box.GetUrlParameter(ctx, "articleUrl"),
		"published":   true,
		"$ne":         JSON{"banned": true},
	}}, article)
	if err != nil {
		return articleNotFound()
	}
	blog := &Blog{}
	if err := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{
		"id": article.BlogId, "$ne": JSON{"banned": true},
	}}, blog); err != nil {
		return blogNotFound()
	}
	http.Redirect(w, r, articlePublicPath(blog.Slug, article.Url), http.StatusPermanentRedirect)
	return nil
}

func RenderBlogArticle(w http.ResponseWriter, ctx context.Context) error {
	blog, err := getPublicBlogBySlug(ctx, box.GetUrlParameter(ctx, "blogSlug"))
	if err != nil {
		return err
	}
	filter := JSON{
		"blog_id":   blog.Id,
		"url":       box.GetUrlParameter(ctx, "articleUrl"),
		"published": true,
		"$ne":       JSON{"banned": true},
	}
	db := GetInceptionClient(ctx)
	article := &Article{}
	if err := db.FindOne("articles", inceptiondb.FindQuery{Filter: filter}, article); err != nil {
		return articleNotFound()
	}

	const maxWords = 15
	words := strings.SplitN(article.Title, " ", maxWords)
	trail := ""
	if len(words) >= maxWords {
		words = words[:maxWords-1]
		trail = "..."
	}
	title := "@" + article.AuthorNick + ": " + strings.Join(words, " ") + trail
	selfURL := "https://gopress.org" + articlePublicPath(blog.Slug, article.Url)
	descriptionRunes := []rune(removeSpaces(html2text(string(article.ContentHTML))))
	if len(descriptionRunes) > 150 {
		descriptionRunes = append(descriptionRunes[:150], []rune("...")...)
	}

	if err := templates.Execute(ctx, "article", w, JSON{
		"article": article, "blog": blog, "og_title": title, "og_url": selfURL,
		"og_image": article.AuthorPicture, "og_description": string(descriptionRunes),
	}); err != nil {
		return fmt.Errorf("error rendering template: %w", err)
	}

	response, patchErr := db.Patch("articles", inceptiondb.PatchQuery{Filter: filter, Patch: JSON{
		"stats": JSON{"views": article.Stats.Views + 1},
	}})
	if patchErr == nil && response != nil {
		io.Copy(io.Discard, response)
		response.Close()
	}
	return nil
}

func articlePublicPath(blogSlug, articleURL string) string {
	return "/blog/" + url.PathEscape(blogSlug) + "/article/" + url.PathEscape(articleURL)
}
