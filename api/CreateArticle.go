package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

var (
	articleWriteMu   sync.Mutex
	articleIdPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
)

type CreateArticleRequest struct {
	Id    string `json:"id"`
	Title string `json:"title"`
}

func CreateArticle(input *CreateArticleRequest, ctx context.Context) any {

	if input.Id == "" {
		input.Id = "article_" + uuid.New().String()
	}
	if !articleIdPattern.MatchString(input.Id) {
		return invalidArticle("id de artículo no válido")
	}

	auth := glueauth.GetAuth(ctx)
	blogId := box.GetUrlParameter(ctx, "blogId")
	var blog *Blog
	var err error
	if blogId == "" {
		blog, err = EnsurePersonalBlog(ctx, auth)
	} else {
		blog, _, err = RequireBlogAccess(ctx, blogId)
	}
	if err != nil {
		return err
	}
	if blog.Banned {
		return blogBanned()
	}
	now := time.Now().UTC()

	newArticle := &Article{
		Id:            input.Id,
		BlogId:        blog.Id,
		BlogSlug:      blog.Slug,
		AuthorId:      auth.User.ID,
		AuthorNick:    auth.User.Nick,
		AuthorPicture: auth.User.Picture,
		CreatedOn:     now,
		UpdatedOn:     now,
		UpdatedById:   auth.User.ID,
		UpdatedByNick: auth.User.Nick,
		Revision:      1,
		Published:     false,
		ArticleUserFields: ArticleUserFields{
			Title:        input.Title,
			AutomaticUrl: true,
			TagsMode:     articleTagsAutomatic,
			Content: Content{
				Type: "editorjs",
				Data: json.RawMessage("{}"),
			},
		},
	}

	articleWriteMu.Lock()
	existing := &Article{}
	findErr := GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{Filter: JSON{"id": input.Id}}, existing)
	if findErr == nil {
		articleWriteMu.Unlock()
		return HttpError{Status: http.StatusConflict, Title: "Article Already Exists", Description: "Ya existe un artículo con ese id"}
	}
	if !errors.Is(findErr, io.EOF) {
		articleWriteMu.Unlock()
		return HttpError{Status: http.StatusInternalServerError, Title: "Article Create Failed", Description: "No se pudo comprobar el id del artículo"}
	}
	newArticle.Url, err = availableArticleURL(GetInceptionClient(ctx), blog.Id, input.Id, Slug(input.Title))
	if err != nil {
		articleWriteMu.Unlock()
		return HttpError{Status: http.StatusInternalServerError, Title: "Article Create Failed", Description: "No se pudo generar la URL del artículo"}
	}
	err = GetInceptionClient(ctx).Insert("articles", newArticle)
	articleWriteMu.Unlock()
	if err != nil {
		log.Println("create article: insert:", err.Error())
		return JSON{
			"error": "error creating article",
		}
	}

	setArticlePublicURL(newArticle)
	return newArticle
}
