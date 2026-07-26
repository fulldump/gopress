package api

import (
	"context"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func ListArticles(ctx context.Context) any {

	auth := glueauth.GetAuth(ctx)
	blogId := box.GetUrlParameter(ctx, "blogId")
	var blog *Blog
	var err error
	if blogId == "" {
		blog, err = EnsurePersonalBlog(ctx, auth)
	} else {
		blog, _, err = RequireBlogAccess(ctx, blogId)
	}
	if err != nil {
		return err
	}
	query := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"blog_id": blog.Id,
		},
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

	return result
}
