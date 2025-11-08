package api

import (
	"context"
	"errors"
	"log"
	"strings"
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

	moderator := GetContentModerator(ctx)
	article.Banned = false
	if moderator != nil {
		content := strings.TrimSpace(removeSpaces(html2text(string(article.ContentHTML))))
		if content == "" && len(article.Content.Data) > 0 {
			content = strings.TrimSpace(removeSpaces(string(article.Content.Data)))
		}
		if content == "" {
			content = article.Title
		}

		banned, err := moderator.Evaluate(ctx, content)
		if err != nil {
			log.Println("publish article: moderation:", err.Error())
			return nil, errors.New("could not validate article content")
		}
		article.Banned = banned

		if !article.Banned {
			type tagGenerator interface {
				GenerateTags(ctx context.Context, content string) ([]string, error)
			}

			if generator, ok := moderator.(tagGenerator); ok {
				tags, err := generator.GenerateTags(ctx, content)
				if err != nil {
					log.Println("publish article: generate tags:", err.Error())
				} else if len(tags) > 0 {
					article.Tags = tags
				}
			}
		}
	}

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
