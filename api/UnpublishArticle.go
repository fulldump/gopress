package api

import (
	"context"
	"log"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func UnpublishArticle(ctx context.Context) any {

	db := GetInceptionClient(ctx)

	filter := JSON{
		"id":        box.GetUrlParameter(ctx, "articleId"),
		"author_id": glueauth.GetAuth(ctx).User.ID,
	}

	article := &Article{}
	err := db.FindOne("articles", inceptiondb.FindQuery{
		Filter: filter,
	}, article)
	if err != nil {
		log.Println("unpublish article: db find:", err.Error())
		return JSON{
			"error": "could not read from data storage",
		}
	}

	article.Published = false

	_, err = db.Patch("articles", inceptiondb.PatchQuery{
		Filter: filter,
		Patch:  article,
	})
	if err != nil {
		log.Println("unpublish article: db patch:", err.Error())
		return JSON{
			"error": "could not write to data storage",
		}
	}

	return article

}
