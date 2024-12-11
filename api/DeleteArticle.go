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
		w.WriteHeader(http.StatusNotFound)
		return JSON{
			"error": "article not found",
		}
	}

	// todo: handle errors properly
	io.Copy(w, r)
	r.Close()

	return nil
}
