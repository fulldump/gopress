package api

import (
	"context"
	"errors"
	"log"
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
		log.Println("render article: db find:", err.Error())
		w.WriteHeader(http.StatusNotFound)
		return errors.New("article not found")
	}

	return article
}
