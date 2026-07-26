package api

import (
	"context"
	"errors"
	"fmt"
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
	targetArticle                 = "article"
	targetBlog                    = "blog"
	targetComment                 = "comment"
	maxIndexString                = "\U0010FFFF"
)

// ArticleInteraction keeps its historical name and fields so existing rows and
// external maintenance tools remain readable while all new code uses Target*.
type ArticleInteraction struct {
	Id              string    `json:"id"`
	TargetKind      string    `json:"targetKind,omitempty"`
	TargetId        string    `json:"targetId,omitempty"`
	TargetCreatedOn time.Time `json:"targetCreatedOn,omitempty"`
	UserId          string    `json:"userId"`
	Type            string    `json:"type"`
	CreatedOn       time.Time `json:"createdOn"`

	ArticleId        string    `json:"articleId,omitempty"`
	ArticleCreatedOn time.Time `json:"articleCreatedOn,omitempty"`
}

type interactionTarget struct {
	Kind      string
	Id        string
	CreatedOn time.Time
}

type ArticleFeedback struct {
	Likes      int  `json:"likes"`
	Liked      bool `json:"liked"`
	Bookmarked bool `json:"bookmarked"`
}

type BookmarkedArticle struct {
	Id                 string         `json:"id"`
	Title              string         `json:"title"`
	Url                string         `json:"url"`
	BlogSlug           string         `json:"blogSlug"`
	AuthorNick         string         `json:"authorNick"`
	AuthorPicture      string         `json:"authorPicture"`
	ContentSummary     string         `json:"contentSummary"`
	Tags               []string       `json:"tags"`
	PublishOn          time.Time      `json:"publishOn"`
	BookmarkedOn       time.Time      `json:"bookmarkedOn"`
	PublicUrl          string         `json:"publicUrl"`
	AvailableLanguages []LanguageLink `json:"availableLanguages"`
}

func MigrateArticleInteractions(db *inceptiondb.Client) (int, error) {
	legacy := []*ArticleInteraction{}
	if err := db.FindAll(articleInteractionsCollection, inceptiondb.FindQuery{Limit: maxBlogMigrationRows}, func(row *ArticleInteraction) {
		if row.TargetKind == "" && row.ArticleId != "" && !row.ArticleCreatedOn.IsZero() {
			legacy = append(legacy, row)
		}
	}); err != nil {
		return 0, fmt.Errorf("scan article interactions: %w", err)
	}
	patchedCount := 0
	for _, row := range legacy {
		patched := &ArticleInteraction{}
		matched, err := db.PatchOne(articleInteractionsCollection, inceptiondb.PatchQuery{
			Filter: JSON{"id": row.Id, "articleId": row.ArticleId, "articleCreatedOn": row.ArticleCreatedOn},
			Patch:  JSON{"targetKind": targetArticle, "targetId": row.ArticleId, "targetCreatedOn": row.ArticleCreatedOn},
		}, patched)
		if err != nil {
			return patchedCount, fmt.Errorf("migrate interaction %q: %w", row.Id, err)
		}
		if matched {
			patchedCount++
		}
	}
	return patchedCount, nil
}

func GetArticleFeedback(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	article, err := getPublicArticle(ctx, box.GetUrlParameter(ctx, "articleId"))
	if err != nil {
		return nil, err
	}
	return getTargetFeedback(ctx, w, interactionTarget{Kind: targetArticle, Id: article.Id, CreatedOn: article.CreatedOn})
}

func LikeArticle(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateArticleInteraction(ctx, w, interactionLike, true)
}

func UnlikeArticle(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateArticleInteraction(ctx, w, interactionLike, false)
}

func BookmarkArticle(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateArticleInteraction(ctx, w, interactionBookmark, true)
}

func RemoveArticleBookmark(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateArticleInteraction(ctx, w, interactionBookmark, false)
}

func GetBlogFeedback(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	blog, err := getPublicBlogById(ctx, box.GetUrlParameter(ctx, "blogId"))
	if err != nil {
		return nil, err
	}
	return getTargetFeedback(ctx, w, interactionTarget{Kind: targetBlog, Id: blog.Id, CreatedOn: blog.CreatedOn})
}

func LikeBlog(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateBlogInteraction(ctx, w, interactionLike, true)
}

func UnlikeBlog(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateBlogInteraction(ctx, w, interactionLike, false)
}

func BookmarkBlog(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateBlogInteraction(ctx, w, interactionBookmark, true)
}

func RemoveBlogBookmark(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateBlogInteraction(ctx, w, interactionBookmark, false)
}

func GetCommentFeedback(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	comment, _, _, err := getPublicCommentTarget(ctx, box.GetUrlParameter(ctx, "commentId"))
	if err != nil {
		return nil, err
	}
	return getTargetFeedback(ctx, w, interactionTarget{Kind: targetComment, Id: comment.Id, CreatedOn: comment.CreatedOn})
}

func LikeComment(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateCommentInteraction(ctx, w, interactionLike, true)
}

func UnlikeComment(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateCommentInteraction(ctx, w, interactionLike, false)
}

func BookmarkComment(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateCommentInteraction(ctx, w, interactionBookmark, true)
}

func RemoveCommentBookmark(ctx context.Context, w http.ResponseWriter) (*ArticleFeedback, error) {
	return mutateCommentInteraction(ctx, w, interactionBookmark, false)
}

func mutateArticleInteraction(ctx context.Context, w http.ResponseWriter, interactionType string, add bool) (*ArticleFeedback, error) {
	article, err := getPublicArticle(ctx, box.GetUrlParameter(ctx, "articleId"))
	if err != nil {
		return nil, err
	}
	return mutateTargetInteraction(ctx, w, interactionTarget{Kind: targetArticle, Id: article.Id, CreatedOn: article.CreatedOn}, interactionType, add)
}

func mutateBlogInteraction(ctx context.Context, w http.ResponseWriter, interactionType string, add bool) (*ArticleFeedback, error) {
	blog, err := getPublicBlogById(ctx, box.GetUrlParameter(ctx, "blogId"))
	if err != nil {
		return nil, err
	}
	return mutateTargetInteraction(ctx, w, interactionTarget{Kind: targetBlog, Id: blog.Id, CreatedOn: blog.CreatedOn}, interactionType, add)
}

func mutateCommentInteraction(ctx context.Context, w http.ResponseWriter, interactionType string, add bool) (*ArticleFeedback, error) {
	comment, _, _, err := getPublicCommentTarget(ctx, box.GetUrlParameter(ctx, "commentId"))
	if err != nil {
		return nil, err
	}
	return mutateTargetInteraction(ctx, w, interactionTarget{Kind: targetComment, Id: comment.Id, CreatedOn: comment.CreatedOn}, interactionType, add)
}

func getTargetFeedback(ctx context.Context, w http.ResponseWriter, target interactionTarget) (*ArticleFeedback, error) {
	w.Header().Set("Cache-Control", "no-store")
	return targetFeedback(ctx, target)
}

func mutateTargetInteraction(ctx context.Context, w http.ResponseWriter, target interactionTarget, interactionType string, add bool) (*ArticleFeedback, error) {
	auth := glueauth.GetAuth(ctx)
	filter := interactionFilter(auth.User.ID, target, interactionType)
	db := GetInceptionClient(ctx)
	if add {
		existing := &ArticleInteraction{}
		err := db.FindOne(articleInteractionsCollection, inceptiondb.FindQuery{Filter: filter}, existing)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, feedbackPersistenceError("No se pudo actualizar la interacción", err)
		}
		if errors.Is(err, io.EOF) {
			interaction := &ArticleInteraction{
				Id: "interaction_" + uuid.New().String(), TargetKind: target.Kind, TargetId: target.Id,
				TargetCreatedOn: target.CreatedOn, UserId: auth.User.ID, Type: interactionType, CreatedOn: time.Now().UTC(),
				ArticleId: target.Id, ArticleCreatedOn: target.CreatedOn,
			}
			if err := db.Insert(articleInteractionsCollection, interaction); err != nil && err != inceptiondb.ErrorAlreadyExist {
				return nil, feedbackPersistenceError("No se pudo actualizar la interacción", err)
			}
		}
	} else {
		removed, err := db.Remove(articleInteractionsCollection, inceptiondb.FindQuery{Filter: filter, Limit: 1})
		if err != nil {
			return nil, feedbackPersistenceError("No se pudo actualizar la interacción", err)
		}
		io.Copy(io.Discard, removed)
		removed.Close()
	}
	w.Header().Set("Cache-Control", "no-store")
	return targetFeedback(ctx, target)
}

func targetFeedback(ctx context.Context, target interactionTarget) (*ArticleFeedback, error) {
	userId := ""
	if auth := glueauth.GetAuth(ctx); auth != nil {
		userId = auth.User.ID
	}
	feedback := &ArticleFeedback{}
	err := findArticleInteractions(ctx, inceptiondb.FindQuery{
		Index:  "interactions-by-target-version",
		From:   JSON{"targetKind": target.Kind, "targetId": target.Id, "targetCreatedOn": target.CreatedOn, "id": ""},
		To:     JSON{"targetKind": target.Kind, "targetId": target.Id, "targetCreatedOn": target.CreatedOn, "id": maxIndexString},
		Filter: targetFilter(target),
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
		return nil, feedbackPersistenceError("No se pudo cargar la valoración", err)
	}
	return feedback, nil
}

func feedbackForTargets(ctx context.Context, kind string, targets map[string]time.Time) (map[string]ArticleFeedback, error) {
	result := make(map[string]ArticleFeedback, len(targets))
	if len(targets) == 0 {
		return result, nil
	}
	userId := ""
	if auth := glueauth.GetAuth(ctx); auth != nil {
		userId = auth.User.ID
	}
	err := findArticleInteractions(ctx, inceptiondb.FindQuery{
		Index: "interactions-by-target-kind", From: JSON{"targetKind": kind, "targetId": "", "id": ""},
		To: JSON{"targetKind": kind, "targetId": maxIndexString, "id": maxIndexString}, Filter: JSON{"targetKind": kind},
	}, func(row *ArticleInteraction) {
		createdOn, wanted := targets[row.TargetId]
		if !wanted || !row.TargetCreatedOn.Equal(createdOn) {
			return
		}
		feedback := result[row.TargetId]
		if row.Type == interactionLike {
			feedback.Likes++
			feedback.Liked = feedback.Liked || row.UserId == userId
		} else if row.Type == interactionBookmark && row.UserId == userId {
			feedback.Bookmarked = true
		}
		result[row.TargetId] = feedback
	})
	return result, err
}

func ListBookmarks(ctx context.Context, w http.ResponseWriter) ([]*BookmarkedArticle, error) {
	items, err := bookmarkedArticles(ctx)
	if err != nil {
		return nil, err
	}
	w.Header().Set("Cache-Control", "no-store")
	return items, nil
}

func bookmarkedArticles(ctx context.Context) ([]*BookmarkedArticle, error) {
	auth := glueauth.GetAuth(ctx)
	bookmarks, err := userInteractions(ctx, auth.User.ID, interactionBookmark, targetArticle)
	if err != nil {
		return nil, feedbackPersistenceError("No se pudieron cargar los artículos guardados", err)
	}
	result := []*BookmarkedArticle{}
	for _, bookmark := range bookmarks {
		article := &Article{}
		err := GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{Filter: JSON{
			"id": bookmark.TargetId, "createdOn": bookmark.TargetCreatedOn, "published": true, "$ne": JSON{"banned": true},
		}}, article)
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			return nil, feedbackPersistenceError("No se pudieron cargar los artículos guardados", err)
		}
		blog := &Blog{}
		if GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": article.BlogId, "$ne": JSON{"banned": true}}}, blog) != nil {
			continue
		}
		localized := preparePublicArticle(article, blog, preferredContentLanguage(ctx))
		result = append(result, &BookmarkedArticle{
			Id: article.Id, Title: localized.Title, Url: article.Url, BlogSlug: article.BlogSlug,
			AuthorNick: article.AuthorNick, AuthorPicture: article.AuthorPicture, ContentSummary: bookmarkSummary(localized),
			Tags: article.Tags, PublishOn: article.PublishOn, BookmarkedOn: bookmark.CreatedOn,
			PublicUrl: localized.PublicUrl, AvailableLanguages: localized.AvailableLanguages,
		})
	}
	return result, nil
}

func userInteractions(ctx context.Context, userId, interactionType, kind string) ([]*ArticleInteraction, error) {
	rows := []*ArticleInteraction{}
	err := findArticleInteractions(ctx, inceptiondb.FindQuery{
		Index: "interactions-by-user-type-kind", From: JSON{"userId": userId, "type": interactionType, "targetKind": kind, "id": ""},
		To:     JSON{"userId": userId, "type": interactionType, "targetKind": kind, "id": maxIndexString},
		Filter: JSON{"userId": userId, "type": interactionType, "targetKind": kind},
	}, func(row *ArticleInteraction) { rows = append(rows, row) })
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CreatedOn.Equal(rows[j].CreatedOn) {
			return rows[i].Id > rows[j].Id
		}
		return rows[i].CreatedOn.After(rows[j].CreatedOn)
	})
	return rows, err
}

func findArticleInteractions(ctx context.Context, query inceptiondb.FindQuery, callback func(*ArticleInteraction)) error {
	query.Limit = maxBlogMigrationRows
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

func interactionFilter(userId string, target interactionTarget, interactionType string) JSON {
	filter := targetFilter(target)
	filter["userId"] = userId
	filter["type"] = interactionType
	return filter
}

func targetFilter(target interactionTarget) JSON {
	return JSON{"targetKind": target.Kind, "targetId": target.Id, "targetCreatedOn": target.CreatedOn}
}

func getPublicBlogById(ctx context.Context, id string) (*Blog, error) {
	blog := &Blog{}
	if err := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{
		"id": id, "$ne": JSON{"banned": true},
	}}, blog); err != nil {
		return nil, blogNotFound()
	}
	return blog, nil
}

func getPublicCommentTarget(ctx context.Context, id string) (*Comment, *Article, *Blog, error) {
	comment := &Comment{}
	if err := GetInceptionClient(ctx).FindOne(commentsCollection, inceptiondb.FindQuery{Filter: JSON{"id": id}}, comment); err != nil {
		return nil, nil, nil, commentNotFound()
	}
	article, blog, err := publicCommentContext(ctx, comment)
	if err != nil {
		return nil, nil, nil, err
	}
	return comment, article, blog, nil
}

func publicCommentContext(ctx context.Context, comment *Comment) (*Article, *Blog, error) {
	article := &Article{}
	if err := GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{Filter: JSON{
		"id": comment.ArticleId, "createdOn": comment.ArticleCreatedOn, "published": true, "$ne": JSON{"banned": true},
	}}, article); err != nil {
		return nil, nil, commentNotFound()
	}
	blog, err := getPublicBlogById(ctx, article.BlogId)
	if err != nil {
		return nil, nil, commentNotFound()
	}
	return article, blog, nil
}

func commentNotFound() HttpError {
	return HttpError{Status: http.StatusNotFound, Title: "Comment Not Found", Description: "El comentario no existe o ya no es público"}
}

func feedbackPersistenceError(description string, err error) HttpError {
	log.Println("interactions:", err.Error())
	return HttpError{Status: http.StatusInternalServerError, Title: "Could Not Update Feedback", Description: description}
}
