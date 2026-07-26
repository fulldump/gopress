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

	"gopress/glueauth"
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
	ParentId         string    `json:"parentId"`
	Name             string    `json:"name"`
	Email            string    `json:"email,omitempty"`
	UserId           string    `json:"userId"`
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
	Mine      bool             `json:"mine"`
	Feedback  ArticleFeedback  `json:"feedback"`
	Url       string           `json:"url"`
	Replies   []*PublicComment `json:"replies"`
}

type CreateCommentRequest struct {
	ParentId string `json:"parentId"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Text     string `json:"text"`
}

type CommentLocation struct {
	Offset   int    `json:"offset"`
	Fragment string `json:"fragment"`
}

func GetCommentLocation(ctx context.Context) (*CommentLocation, error) {
	fragment := strings.TrimSpace(box.GetUrlParameter(ctx, "fragment"))
	if fragment == "" {
		return nil, commentNotFound()
	}
	comment := &Comment{}
	db := GetInceptionClient(ctx)
	for _, id := range []string{"comment_" + fragment, fragment} {
		if err := db.FindOne(commentsCollection, inceptiondb.FindQuery{Filter: JSON{"id": id}}, comment); err == nil {
			break
		}
		comment = &Comment{}
	}
	if comment.Id == "" {
		return nil, commentNotFound()
	}
	article, _, err := publicCommentContext(ctx, comment)
	if err != nil {
		return nil, err
	}
	rootId := comment.Id
	if comment.ParentId != "" {
		rootId = comment.ParentId
	}
	rootIndex := -1
	index := 0
	err = db.FindAll(commentsCollection, inceptiondb.FindQuery{
		Index: "comments-by-article-version-parent-created", Limit: maxBlogMigrationRows,
		Filter: JSON{"articleId": article.Id, "articleCreatedOn": article.CreatedOn, "parentId": ""},
	}, func(root *Comment) {
		if root.Id == rootId {
			rootIndex = index
		}
		index++
	})
	if err != nil {
		return nil, feedbackPersistenceError("No se pudo localizar el comentario", err)
	}
	if rootIndex < 0 {
		return nil, commentNotFound()
	}
	return &CommentLocation{Offset: rootIndex / listPageSize * listPageSize, Fragment: commentFragment(comment.Id)}, nil
}

func MigrateCommentDefaults(db *inceptiondb.Client) (int, error) {
	type commentPatch struct {
		id    string
		patch JSON
	}
	patches := []commentPatch{}
	if err := db.FindAll(commentsCollection, inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(row *JSON) {
		patch := JSON{}
		if _, exists := (*row)["parentId"]; !exists {
			patch["parentId"] = ""
		}
		if _, exists := (*row)["userId"]; !exists {
			patch["userId"] = ""
		}
		id, _ := (*row)["id"].(string)
		if id != "" && len(patch) > 0 {
			patches = append(patches, commentPatch{id: id, patch: patch})
		}
	}); err != nil {
		return 0, err
	}
	patchedCount := 0
	for _, item := range patches {
		patched := &Comment{}
		matched, err := db.PatchOne(commentsCollection, inceptiondb.PatchQuery{Filter: JSON{"id": item.id}, Patch: item.patch}, patched)
		if err != nil {
			return patchedCount, err
		}
		if matched {
			patchedCount++
		}
	}
	return patchedCount, nil
}

func ListComments(ctx context.Context, w http.ResponseWriter, r *http.Request) (*ListPage[*PublicComment], error) {
	offset, err := listOffset(r)
	if err != nil {
		return nil, err
	}
	articleId := box.GetUrlParameter(ctx, "articleId")
	article, err := getPublicArticle(ctx, articleId)
	if err != nil {
		return nil, err
	}
	blog, err := getPublicBlogById(ctx, article.BlogId)
	if err != nil {
		return nil, articleNotFound()
	}

	roots := make([]*Comment, 0, listPageSize+1)
	err = GetInceptionClient(ctx).FindAll(commentsCollection, inceptiondb.FindQuery{
		Index: "comments-by-article-version-parent-created", Skip: offset, Limit: listPageSize + 1,
		Filter: JSON{"articleId": articleId, "articleCreatedOn": article.CreatedOn, "parentId": ""},
	}, func(comment *Comment) {
		roots = append(roots, comment)
	})
	if err != nil {
		log.Println("list comments: db find:", err.Error())
		return nil, HttpError{
			Status:      http.StatusInternalServerError,
			Title:       "Could Not Load Comments",
			Description: "No se pudieron cargar los comentarios",
		}
	}

	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].CreatedOn.Equal(roots[j].CreatedOn) {
			return roots[i].Id < roots[j].Id
		}
		return roots[i].CreatedOn.Before(roots[j].CreatedOn)
	})
	page := pageFromLookahead(roots, offset)
	rootById := make(map[string]*Comment, len(page.Items))
	comments := append([]*Comment{}, page.Items...)
	for _, root := range page.Items {
		rootById[root.Id] = root
	}
	if len(rootById) > 0 {
		err = GetInceptionClient(ctx).FindAll(commentsCollection, inceptiondb.FindQuery{
			Limit: maxBlogMigrationRows, Filter: JSON{"articleId": articleId, "articleCreatedOn": article.CreatedOn},
		}, func(comment *Comment) {
			if comment.ParentId != "" && rootById[comment.ParentId] != nil {
				comments = append(comments, comment)
			}
		})
		if err != nil {
			return nil, feedbackPersistenceError("No se pudieron cargar las respuestas", err)
		}
	}
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].CreatedOn.Equal(comments[j].CreatedOn) {
			return comments[i].Id < comments[j].Id
		}
		return comments[i].CreatedOn.Before(comments[j].CreatedOn)
	})
	targets := make(map[string]time.Time, len(comments))
	for _, comment := range comments {
		targets[comment.Id] = comment.CreatedOn
	}
	feedback, err := feedbackForTargets(ctx, targetComment, targets)
	if err != nil {
		return nil, feedbackPersistenceError("No se pudo cargar la valoración de los comentarios", err)
	}
	userId := ""
	if auth := glueauth.GetAuth(ctx); auth != nil {
		userId = auth.User.ID
	}
	byId := make(map[string]*PublicComment, len(comments))
	for _, comment := range comments {
		byId[comment.Id] = publicComment(comment, article, blog, userId, feedback[comment.Id])
	}
	result := make([]*PublicComment, 0, len(page.Items))
	for _, root := range page.Items {
		result = append(result, byId[root.Id])
	}
	for _, comment := range comments {
		if comment.ParentId != "" && byId[comment.ParentId] != nil {
			byId[comment.ParentId].Replies = append(byId[comment.ParentId].Replies, byId[comment.Id])
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	return &ListPage[*PublicComment]{Items: result, HasMore: page.HasMore, NextOffset: page.NextOffset}, nil
}

func CreateComment(ctx context.Context, w http.ResponseWriter, r *http.Request) (*PublicComment, error) {
	articleId := box.GetUrlParameter(ctx, "articleId")
	article, err := getPublicArticle(ctx, articleId)
	if err != nil {
		return nil, err
	}
	blog, err := getPublicBlogById(ctx, article.BlogId)
	if err != nil {
		return nil, articleNotFound()
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

	userId := ""
	if auth := glueauth.GetAuth(ctx); auth != nil {
		userId = auth.User.ID
	}
	comment := &Comment{
		Id: "comment_" + uuid.New().String(), ArticleId: articleId, ArticleCreatedOn: article.CreatedOn,
		ParentId: input.ParentId, Name: input.Name, Email: input.Email, UserId: userId,
		Text: input.Text, CreatedOn: time.Now().UTC(),
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
	return publicComment(comment, article, blog, userId, ArticleFeedback{}), nil
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
	if !isPublicBlog(ctx, article.BlogId) {
		return nil, HttpError{Status: http.StatusNotFound, Title: "Article Not Found", Description: "El artículo no existe o no está publicado"}
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

func commentFragment(id string) string {
	return "comment-" + strings.TrimPrefix(id, "comment_")
}

func canonicalCommentURL(article *Article, blog *Blog, commentId string) string {
	return articlePublicPath(blog.Slug, article.Url) + "#" + commentFragment(commentId)
}

func publicComment(comment *Comment, article *Article, blog *Blog, userId string, feedback ArticleFeedback) *PublicComment {
	return &PublicComment{
		Id:        comment.Id,
		ArticleId: comment.ArticleId,
		ParentId:  comment.ParentId,
		Name:      comment.Name,
		Text:      comment.Text,
		CreatedOn: comment.CreatedOn,
		Mine:      userId != "" && comment.UserId == userId,
		Feedback:  feedback,
		Url:       canonicalCommentURL(article, blog, comment.Id),
		Replies:   []*PublicComment{},
	}
}
