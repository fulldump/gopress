package api

import (
	"context"
	"net/http"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func ListArticles(ctx context.Context, r *http.Request) any {
	offset, err := listOffset(r)
	if err != nil {
		return err
	}

	auth := glueauth.GetAuth(ctx)
	blogId := box.GetUrlParameter(ctx, "blogId")
	var blog *Blog
	err = nil
	if blogId == "" {
		blog, err = EnsurePersonalBlog(ctx, auth)
	} else {
		blog, _, err = RequireBlogAccess(ctx, blogId)
	}
	if err != nil {
		return err
	}
	filter := JSON{"blog_id": blog.Id}
	if value := r.URL.Query().Get("published"); value != "" {
		switch value {
		case "true":
			filter["published"] = true
		case "false":
			filter["published"] = false
		default:
			return HttpError{Status: http.StatusBadRequest, Title: "Invalid Publication Filter", Description: "published must be true or false"}
		}
	}
	query := inceptiondb.FindQuery{
		Index:   "articles-admin-updated-v2",
		Skip:    offset,
		Limit:   listPageSize + 1,
		Reverse: true,
		Filter:  filter,
	}

	result := []*ArticleShort{}
	err = GetInceptionClient(ctx).FindAll("articles", query, func(article *Article) {
		links := availableArticleLanguages(article, blog, article.SourceLanguage)
		languages := make([]BlogLanguage, 0, len(links))
		for _, link := range links {
			languages = append(languages, link.Language)
		}
		result = append(result, &ArticleShort{
			Id:        article.Id,
			Title:     article.Title,
			Url:       article.Url,
			Published: article.Published,
			Stats:     article.Stats,
			Tags:      article.Tags,
			CreatedOn: article.CreatedOn,
			UpdatedOn: article.UpdatedOn,
			BlogId:    article.BlogId,
			BlogSlug:  article.BlogSlug,
			Revision:  article.Revision,
			EditLock:  publicArticleLock(article.EditLock),
			Languages: languages,
		})
	})
	if err != nil {
		return blogStorageError("list articles", err)
	}
	return pageFromLookahead(result, offset)
}
