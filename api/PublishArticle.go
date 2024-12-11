package api

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func PublishArticle(ctx context.Context) (*Article, error) {

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
		log.Println("publish article: db find:", err.Error())
		return nil, errors.New("could not read from data storage")
	}

	article.Published = true
	article.PublishOn = time.Now()

	_, err = db.Patch("articles", inceptiondb.PatchQuery{
		Filter: filter,
		Patch:  article,
	})
	if err != nil {
		log.Println("publish article: db patch:", err.Error())
		return nil, errors.New("could not write to data storage")
	}

	return article, nil
}
