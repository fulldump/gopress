package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/inceptiondb"
)

const (
	commentsCollection = "comments"
	maxCommentBodySize = 64 << 10
	maxCommentName     = 100
	maxCommentText     = 10000
)

type Comment struct {
	Id               string    `json:"id"`
	ArticleId        string    `json:"articleId"`
	ArticleCreatedOn time.Time `json:"articleCreatedOn,omitempty"`
	ParentId         string    `json:"parentId,omitempty"`
	Name             string    `json:"name"`
	Email            string    `json:"email,omitempty"`
	Text             string    `json:"text"`
	CreatedOn        time.Time `json:"createdOn"`
}

type PublicComment struct {
	Id        string           `json:"id"`
	ArticleId string           `json:"articleId"`
	ParentId  string           `json:"parentId,omitempty"`
	Name      string           `json:"name"`
	Text      string           `json:"text"`
	CreatedOn time.Time        `json:"createdOn"`
	Replies   []*PublicComment `json:"replies"`
}

type CreateCommentRequest struct {
	ParentId string `json:"parentId"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Text     string `json:"text"`
}

func ListComments(ctx context.Context, w http.ResponseWriter) ([]*PublicComment, error) {
	articleId := box.GetUrlParameter(ctx, "articleId")
	article, err := getPublicArticle(ctx, articleId)
	if err != nil {
		return nil, err
	}

	comments := []*Comment{}
	err = GetInceptionClient(ctx).FindAll(commentsCollection, inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"articleId": articleId, "articleCreatedOn": article.CreatedOn,
		},
	}, func(comment *Comment) {
		comments = append(comments, comment)
	})
	if err != nil {
		log.Println("list comments: db find:", err.Error())
		return nil, HttpError{
			Status:      http.StatusInternalServerError,
			Title:       "Could Not Load Comments",
			Description: "No se pudieron cargar los comentarios",
		}
	}

	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].CreatedOn.Before(comments[j].CreatedOn)
	})

	byId := make(map[string]*PublicComment, len(comments))
	for _, comment := range comments {
		byId[comment.Id] = publicComment(comment)
	}

	result := []*PublicComment{}
	for _, comment := range comments {
		item := byId[comment.Id]
		if comment.ParentId == "" {
			result = append(result, item)
			continue
		}
		if parent := byId[comment.ParentId]; parent != nil {
			parent.Replies = append(parent.Replies, item)
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	return result, nil
}

func CreateComment(ctx context.Context, w http.ResponseWriter, r *http.Request) (*PublicComment, error) {
	articleId := box.GetUrlParameter(ctx, "articleId")
	article, err := getPublicArticle(ctx, articleId)
	if err != nil {
		return nil, err
	}

	input := &CreateCommentRequest{}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCommentBodySize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return nil, invalidComment("El cuerpo de la petición no es JSON válido")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, invalidComment("El cuerpo de la petición debe contener un único objeto JSON")
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)
	input.Text = strings.TrimSpace(input.Text)
	input.ParentId = strings.TrimSpace(input.ParentId)

	if input.Name == "" {
		return nil, invalidComment("El nombre es obligatorio")
	}
	if utf8.RuneCountInString(input.Name) > maxCommentName {
		return nil, invalidComment("El nombre no puede superar los 100 caracteres")
	}
	if input.Email != "" {
		if utf8.RuneCountInString(input.Email) > 254 {
			return nil, invalidComment("El email no es válido")
		}
		address, err := mail.ParseAddress(input.Email)
		if err != nil || address.Address != input.Email {
			return nil, invalidComment("El email no es válido")
		}
	}
	if input.Text == "" {
		return nil, invalidComment("El comentario es obligatorio")
	}
	if utf8.RuneCountInString(input.Text) > maxCommentText {
		return nil, invalidComment("El comentario no puede superar los 10000 caracteres")
	}

	if input.ParentId != "" {
		parent := &Comment{}
		err := GetInceptionClient(ctx).FindOne(commentsCollection, inceptiondb.FindQuery{
			Filter: JSON{
				"id": input.ParentId, "articleId": articleId, "articleCreatedOn": article.CreatedOn,
			},
		}, parent)
		if err != nil {
			return nil, invalidComment("El comentario al que intentas responder no existe")
		}
		if parent.ParentId != "" {
			return nil, invalidComment("Solo se permite un nivel de respuestas")
		}
	}

	comment := &Comment{
		Id: "comment_" + uuid.New().String(), ArticleId: articleId, ArticleCreatedOn: article.CreatedOn,
		ParentId: input.ParentId, Name: input.Name, Email: input.Email, Text: input.Text, CreatedOn: time.Now().UTC(),
	}
	if err := GetInceptionClient(ctx).Insert(commentsCollection, comment); err != nil {
		log.Println("create comment: db insert:", err.Error())
		return nil, HttpError{
			Status:      http.StatusInternalServerError,
			Title:       "Could Not Create Comment",
			Description: "No se pudo publicar el comentario",
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	return publicComment(comment), nil
}

func requirePublicArticle(ctx context.Context, articleId string) error {
	_, err := getPublicArticle(ctx, articleId)
	return err
}

func getPublicArticle(ctx context.Context, articleId string) (*Article, error) {
	article := &Article{}
	err := GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{
		Filter: JSON{
			"id":        articleId,
			"published": true,
			"$ne":       JSON{"banned": true},
		},
	}, article)
	if err != nil {
		return nil, HttpError{
			Status:      http.StatusNotFound,
			Title:       "Article Not Found",
			Description: "El artículo no existe o no está publicado",
		}
	}
	return article, nil
}

func invalidComment(description string) HttpError {
	return HttpError{
		Status:      http.StatusBadRequest,
		Title:       "Invalid Comment",
		Description: description,
	}
}

func publicComment(comment *Comment) *PublicComment {
	return &PublicComment{
		Id:        comment.Id,
		ArticleId: comment.ArticleId,
		ParentId:  comment.ParentId,
		Name:      comment.Name,
		Text:      comment.Text,
		CreatedOn: comment.CreatedOn,
		Replies:   []*PublicComment{},
	}
}
