package api

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

const (
	articleInteractionsCollection = "articleInteractions"
	interactionLike               = "like"
	interactionBookmark           = "bookmark"
	maxIndexString                = "\U0010FFFF"
)

type ArticleInteraction struct {
	Id               string    `json:"id"`
	ArticleId        string    `json:"articleId"`
	ArticleCreatedOn time.Time `json:"articleCreatedOn"`
	UserId           string    `json:"userId"`
	Type             string    `json:"type"`
	CreatedOn        time.Time `json:"createdOn"`
}

type ArticleFeedback struct {
	Likes      int  `json:"likes"`
	Liked      bool `json:"liked"`
	Bookmarked bool `json:"bookmarked"`
}

type BookmarkedArticle struct {
	Id             string    `json:"id"`
	Title          string    `json:"title"`
	Url            string    `json:"url"`
	BlogSlug       string    `json:"blogSlug"`
	AuthorNick     string    `json:"authorNick"`
	AuthorPicture  string    `json:"authorPicture"`
	ContentSummary string    `json:"contentSummary"`
	Tags           []string  `json:"tags"`
	PublishOn      time.Time `json:"publishOn"`
	BookmarkedOn   time.Time `json:"bookmarkedOn"`
}

func GetArticleFeedback(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	articleId := box.GetUrlParameter(ctx, "articleId")
	article, err := getPublicArticle(ctx, articleId)
	if err != nil {
		return nil, err
	}

	w.Header().Set("Cache-Control", "no-store")
	return articleFeedback(ctx, article)
}

func LikeArticle(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return addArticleInteraction(ctx, w, interactionLike)
}

func UnlikeArticle(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return removeArticleInteraction(ctx, w, interactionLike)
}

func BookmarkArticle(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return addArticleInteraction(ctx, w, interactionBookmark)
}

func RemoveArticleBookmark(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return removeArticleInteraction(ctx, w, interactionBookmark)
}

func ListBookmarks(ctx context.Context, w http.ResponseWriter) ([]*BookmarkedArticle, error) {
	auth := glueauth.GetAuth(ctx)
	bookmarks := []*ArticleInteraction{}
	err := findArticleInteractions(ctx, inceptiondb.FindQuery{
		Index: "interactions-by-user-type",
		From: JSON{
			"userId": auth.User.ID,
			"type":   interactionBookmark,
			"id":     "",
		},
		To: JSON{
			"userId": auth.User.ID,
			"type":   interactionBookmark,
			"id":     maxIndexString,
		},
		Filter: JSON{
			"userId": auth.User.ID,
			"type":   interactionBookmark,
		},
	}, func(interaction *ArticleInteraction) {
		bookmarks = append(bookmarks, interaction)
	})
	if err != nil {
		return nil, feedbackPersistenceError("No se pudieron cargar los artículos guardados", err)
	}

	sort.SliceStable(bookmarks, func(i, j int) bool {
		return bookmarks[i].CreatedOn.After(bookmarks[j].CreatedOn)
	})

	result := []*BookmarkedArticle{}
	for _, bookmark := range bookmarks {
		article := &Article{}
		err := GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{
			Filter: JSON{
				"id":        bookmark.ArticleId,
				"createdOn": bookmark.ArticleCreatedOn,
				"published": true,
				"$ne":       JSON{"banned": true},
			},
		}, article)
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			return nil, feedbackPersistenceError("No se pudieron cargar los artículos guardados", err)
		}
		result = append(result, &BookmarkedArticle{
			Id:             article.Id,
			Title:          article.Title,
			Url:            article.Url,
			BlogSlug:       article.BlogSlug,
			AuthorNick:     article.AuthorNick,
			AuthorPicture:  article.AuthorPicture,
			ContentSummary: bookmarkSummary(article),
			Tags:           article.Tags,
			PublishOn:      article.PublishOn,
			BookmarkedOn:   bookmark.CreatedOn,
		})
	}

	w.Header().Set("Cache-Control", "no-store")
	return result, nil
}

func addArticleInteraction(ctx context.Context, w http.ResponseWriter, interactionType string) (*ArticleFeedback, error) {
	articleId := box.GetUrlParameter(ctx, "articleId")
	article, err := getPublicArticle(ctx, articleId)
	if err != nil {
		return nil, err
	}

	auth := glueauth.GetAuth(ctx)
	filter := interactionFilter(auth.User.ID, article, interactionType)
	existing := &ArticleInteraction{}
	err = GetInceptionClient(ctx).FindOne(articleInteractionsCollection, inceptiondb.FindQuery{Filter: filter}, existing)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, feedbackPersistenceError("No se pudo actualizar el artículo", err)
	}
	if errors.Is(err, io.EOF) {
		interaction := &ArticleInteraction{
			Id:               "interaction_" + uuid.New().String(),
			ArticleId:        articleId,
			ArticleCreatedOn: article.CreatedOn,
			UserId:           auth.User.ID,
			Type:             interactionType,
			CreatedOn:        time.Now().UTC(),
		}
		if err := GetInceptionClient(ctx).Insert(articleInteractionsCollection, interaction); err != nil && err != inceptiondb.ErrorAlreadyExist {
			return nil, feedbackPersistenceError("No se pudo actualizar el artículo", err)
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	return articleFeedback(ctx, article)
}

func removeArticleInteraction(ctx context.Context, w http.ResponseWriter, interactionType string) (*ArticleFeedback, error) {
	articleId := box.GetUrlParameter(ctx, "articleId")
	article, err := getPublicArticle(ctx, articleId)
	if err != nil {
		return nil, err
	}

	auth := glueauth.GetAuth(ctx)
	removed, err := GetInceptionClient(ctx).Remove(articleInteractionsCollection, inceptiondb.FindQuery{
		Filter: interactionFilter(auth.User.ID, article, interactionType),
		Limit:  1,
	})
	if err != nil {
		return nil, feedbackPersistenceError("No se pudo actualizar el artículo", err)
	}
	io.Copy(io.Discard, removed)
	removed.Close()

	w.Header().Set("Cache-Control", "no-store")
	return articleFeedback(ctx, article)
}

func articleFeedback(ctx context.Context, article *Article) (*ArticleFeedback, error) {
	auth := glueauth.GetAuth(ctx)
	userId := ""
	if auth != nil {
		userId = auth.User.ID
	}

	feedback := &ArticleFeedback{}
	err := findArticleInteractions(ctx, inceptiondb.FindQuery{
		Index: "interactions-by-article-version",
		From: JSON{
			"articleId":        article.Id,
			"articleCreatedOn": article.CreatedOn,
			"id":               "",
		},
		To: JSON{
			"articleId":        article.Id,
			"articleCreatedOn": article.CreatedOn,
			"id":               maxIndexString,
		},
		Filter: JSON{
			"articleId":        article.Id,
			"articleCreatedOn": article.CreatedOn,
		},
	}, func(interaction *ArticleInteraction) {
		switch interaction.Type {
		case interactionLike:
			feedback.Likes++
			if interaction.UserId == userId {
				feedback.Liked = true
			}
		case interactionBookmark:
			if interaction.UserId == userId {
				feedback.Bookmarked = true
			}
		}
	})
	if err != nil {
		return nil, feedbackPersistenceError("No se pudo cargar la valoración del artículo", err)
	}
	return feedback, nil
}

func findArticleInteractions(ctx context.Context, query inceptiondb.FindQuery, callback func(*ArticleInteraction)) error {
	query.Limit = int(^uint(0) >> 1)
	return GetInceptionClient(ctx).FindAll(articleInteractionsCollection, query, callback)
}

func bookmarkSummary(article *Article) string {
	summary := strings.TrimSpace(removeSpaces(html2text(string(article.ContentHTML))))
	runes := []rune(summary)
	if len(runes) > 240 {
		return string(runes[:240]) + "..."
	}
	return summary
}

func interactionFilter(userId string, article *Article, interactionType string) JSON {
	return JSON{
		"userId":           userId,
		"articleId":        article.Id,
		"articleCreatedOn": article.CreatedOn,
		"type":             interactionType,
	}
}

func feedbackPersistenceError(description string, err error) HttpError {
	log.Println("article feedback:", err.Error())
	return HttpError{
		Status:      http.StatusInternalServerError,
		Title:       "Could Not Update Article Feedback",
		Description: description,
	}
}
