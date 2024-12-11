package api

import (
	"context"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func ListArticles(ctx context.Context) any {

	auth := glueauth.GetAuth(ctx)
	query := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"author_id": auth.User.ID,
		},
	}

	result := []*ArticleShort{}
	GetInceptionClient(ctx).FindAll("articles", query, func(article *Article) {
		result = append(result, &ArticleShort{
			Id:        article.Id,
			Title:     article.Title,
			Url:       article.Url,
			Published: article.Published,
			Stats:     article.Stats,
			Tags:      article.Tags,
			CreatedOn: article.CreatedOn,
		})
	})

	return result
}
