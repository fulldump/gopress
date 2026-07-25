package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func FindAccessibleArticle(ctx context.Context) (*Article, *Blog, BlogRole, error) {
	auth := glueauth.GetAuth(ctx)
	if auth == nil || strings.TrimSpace(auth.User.ID) == "" {
		return nil, nil, "", blogForbidden()
	}
	db := GetInceptionClient(ctx)
	blogId := box.GetUrlParameter(ctx, "blogId")
	var blog *Blog
	var role BlogRole
	var err error
	if blogId == "" {
		blog, err = EnsurePersonalBlog(ctx, auth)
		role = BlogRoleOwner
	} else {
		blog, role, err = RequireBlogAccess(ctx, blogId)
	}
	if err != nil {
		return nil, nil, "", err
	}
	article := &Article{}
	if err := db.FindOne("articles", inceptiondb.FindQuery{Filter: JSON{
		"id": box.GetUrlParameter(ctx, "articleId"), "blog_id": blog.Id,
	}}, article); err != nil {
		return nil, nil, "", articleNotFound()
	}
	setArticlePublicURL(article)
	return article, blog, role, nil
}

func revalidateArticleAccess(ctx context.Context, article *Article) error {
	blogId := box.GetUrlParameter(ctx, "blogId")
	if blogId == "" {
		auth := glueauth.GetAuth(ctx)
		return requirePersonalArticleIdentity(article, auth)
	}
	if article.BlogId != blogId {
		return articleNotFound()
	}
	_, _, err := RequireBlogAccess(ctx, blogId)
	return err
}

func requirePersonalArticleIdentity(article *Article, auth *glueauth.GlueAuthentication) error {
	if auth == nil || article.BlogId != deterministicPersonalBlogId(auth.User.ID) {
		return articleNotFound()
	}
	return nil
}

func articleNotFound() HttpError {
	return HttpError{Status: http.StatusNotFound, Title: "Article Not Found", Description: "El artículo que intentas buscar no existe"}
}

func setArticlePublicURL(article *Article) {
	if article == nil || article.BlogSlug == "" || article.Url == "" {
		return
	}
	article.PublicUrl = articlePublicPath(article.BlogSlug, article.Url)
}
