package api

import (
	"context"
	"io"
	"net/http"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func DeleteArticle(w http.ResponseWriter, ctx context.Context) any {

	articleId := box.GetUrlParameter(ctx, "articleId")

	auth := glueauth.GetAuth(ctx)

	r, err := GetInceptionClient(ctx).Remove("articles", inceptiondb.FindQuery{
		Filter: JSON{
			"id":        articleId,
			"author_id": auth.User.ID,
		},
	})
	if err != nil {
		return HttpError{
			Status:      http.StatusNotFound,
			Title:       "Article Not Found",
			Description: "El artículo que intentas buscar no existe",
		}
	}

	// todo: handle errors properly
	io.Copy(w, r)
	r.Close()

	return nil
}
