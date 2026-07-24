package api

import (
	"bytes"
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

	body, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		return HttpError{
			Status:      http.StatusInternalServerError,
			Title:       "Could Not Delete Article",
			Description: "No se pudo eliminar el artículo",
		}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return HttpError{
			Status:      http.StatusNotFound,
			Title:       "Article Not Found",
			Description: "El artículo que intentas buscar no existe",
		}
	}

	for {
		interactions, err := GetInceptionClient(ctx).Remove(articleInteractionsCollection, inceptiondb.FindQuery{
			Filter: JSON{"articleId": articleId},
			Limit:  1000,
		})
		if err != nil {
			return HttpError{
				Status:      http.StatusInternalServerError,
				Title:       "Could Not Delete Article Feedback",
				Description: "El artículo se eliminó, pero no se pudo limpiar su feedback",
			}
		}
		removed, _ := io.Copy(io.Discard, interactions)
		interactions.Close()
		if removed == 0 {
			break
		}
	}
	w.Write(body)

	return nil
}
