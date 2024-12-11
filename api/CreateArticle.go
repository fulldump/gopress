package api

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"

	"gopress/glueauth"
)

type CreateArticleRequest struct {
	Id    string `json:"id"`
	Title string `json:"title"`
}

func CreateArticle(input *CreateArticleRequest, ctx context.Context) any {

	if input.Id == "" {
		input.Id = "article_" + uuid.New().String()
	}

	auth := glueauth.GetAuth(ctx)

	newArticle := &Article{
		Id:            input.Id,
		AuthorId:      auth.User.ID,
		AuthorNick:    auth.User.Nick,
		AuthorPicture: auth.User.Picture,
		CreatedOn:     time.Now(),
		Published:     false,
		ArticleUserFields: ArticleUserFields{
			Title: input.Title,
			Url:   Slug(input.Title) + "-" + uuid.New().String(),
			Content: Content{
				Type: "editorjs",
				Data: json.RawMessage("{}"),
			},
		},
	}

	err := GetInceptionClient(ctx).Insert("articles", newArticle)
	if err != nil {
		log.Println("create article: insert:", err.Error())
		return JSON{
			"error": "error creating article",
		}
	}

	return newArticle
}
