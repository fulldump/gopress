package api

import (
	"context"
	"net/http"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func GetArticle(ctx context.Context, w http.ResponseWriter) any {

	articleId := box.GetUrlParameter(ctx, "articleId")

	auth := glueauth.GetAuth(ctx)

	article := &Article{}
	err := GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{
		Filter: JSON{
			"id":        articleId,
			"author_id": auth.User.ID,
		},
	}, article)
	if err != nil {
		return HttpError{
			Status:      http.StatusNotFound,
			Title:       "Article Not Found",
			Description: "El artículo que intentas buscar no existe",
		}
	}

	return article
}
