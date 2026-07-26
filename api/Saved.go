package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

type SavedBlog struct {
	Id           string    `json:"id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	PublicUrl    string    `json:"publicUrl"`
	BookmarkedOn time.Time `json:"bookmarkedOn"`
}

type SavedArticleContext struct {
	Id        string `json:"id"`
	Title     string `json:"title"`
	PublicUrl string `json:"publicUrl"`
}

type SavedBlogContext struct {
	Id        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	PublicUrl string `json:"publicUrl"`
}

type SavedComment struct {
	Id           string              `json:"id"`
	ParentId     string              `json:"parentId,omitempty"`
	Name         string              `json:"name"`
	Text         string              `json:"text"`
	CreatedOn    time.Time           `json:"createdOn"`
	Mine         bool                `json:"mine"`
	Feedback     ArticleFeedback     `json:"feedback"`
	Url          string              `json:"url"`
	Article      SavedArticleContext `json:"article"`
	Blog         SavedBlogContext    `json:"blog"`
	BookmarkedOn *time.Time          `json:"bookmarkedOn,omitempty"`
}

func ListSaved(ctx context.Context, w http.ResponseWriter, r *http.Request) (any, error) {
	offset, err := listOffset(r)
	if err != nil {
		return nil, err
	}
	kind := box.GetUrlParameter(ctx, "kind")
	var result any
	switch kind {
	case "articles":
		items, err := bookmarkedArticles(ctx)
		if err != nil {
			return nil, err
		}
		page := pageSlice(items, offset)
		result = &page
	case "blogs":
		items, err := bookmarkedBlogs(ctx)
		if err != nil {
			return nil, err
		}
		page := pageSlice(items, offset)
		result = &page
	case "comments":
		items, err := savedComments(ctx)
		if err != nil {
			return nil, err
		}
		page := pageSlice(items, offset)
		result = &page
	case "own-comments":
		items, err := ownComments(ctx)
		if err != nil {
			return nil, err
		}
		page := pageSlice(items, offset)
		result = &page
	default:
		return nil, HttpError{Status: http.StatusNotFound, Title: "Saved Section Not Found", Description: "La sección de guardados no existe"}
	}
	w.Header().Set("Cache-Control", "no-store")
	return result, nil
}

func bookmarkedBlogs(ctx context.Context) ([]*SavedBlog, error) {
	auth := glueauth.GetAuth(ctx)
	rows, err := userInteractions(ctx, auth.User.ID, interactionBookmark, targetBlog)
	if err != nil {
		return nil, feedbackPersistenceError("No se pudieron cargar los blogs guardados", err)
	}
	result := []*SavedBlog{}
	for _, row := range rows {
		blog := &Blog{}
		err := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{
			"id": row.TargetId, "createdOn": row.TargetCreatedOn, "$ne": JSON{"banned": true},
		}}, blog)
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			return nil, feedbackPersistenceError("No se pudieron cargar los blogs guardados", err)
		}
		localized := preparePublicBlog(blog, preferredContentLanguage(ctx))
		result = append(result, &SavedBlog{Id: blog.Id, Slug: blog.Slug, Name: blog.Name,
			Description: blog.Description, PublicUrl: localized.PublicUrl, BookmarkedOn: row.CreatedOn})
	}
	return result, nil
}

func savedComments(ctx context.Context) ([]*SavedComment, error) {
	auth := glueauth.GetAuth(ctx)
	rows, err := userInteractions(ctx, auth.User.ID, interactionBookmark, targetComment)
	if err != nil {
		return nil, feedbackPersistenceError("No se pudieron cargar los comentarios guardados", err)
	}
	result := []*SavedComment{}
	for _, row := range rows {
		comment := &Comment{}
		err := GetInceptionClient(ctx).FindOne(commentsCollection, inceptiondb.FindQuery{Filter: JSON{
			"id": row.TargetId, "createdOn": row.TargetCreatedOn,
		}}, comment)
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			return nil, feedbackPersistenceError("No se pudieron cargar los comentarios guardados", err)
		}
		item, err := savedCommentDTO(ctx, comment, auth.User.ID)
		if err != nil {
			if errors.Is(err, errStaleSavedTarget) {
				continue
			}
			return nil, err
		}
		bookmarkedOn := row.CreatedOn
		item.BookmarkedOn = &bookmarkedOn
		result = append(result, item)
	}
	return result, nil
}

var errStaleSavedTarget = errors.New("saved target is no longer public")

func ownComments(ctx context.Context) ([]*SavedComment, error) {
	auth := glueauth.GetAuth(ctx)
	comments := []*Comment{}
	err := GetInceptionClient(ctx).FindAll(commentsCollection, inceptiondb.FindQuery{
		Index: "comments-by-user-created", Limit: maxBlogMigrationRows,
		From:   JSON{"userId": auth.User.ID, "createdOn": time.Time{}, "id": ""},
		To:     JSON{"userId": auth.User.ID, "createdOn": time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), "id": maxIndexString},
		Filter: JSON{"userId": auth.User.ID},
	}, func(comment *Comment) { comments = append(comments, comment) })
	if err != nil {
		return nil, feedbackPersistenceError("No se pudieron cargar tus comentarios", err)
	}
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].CreatedOn.Equal(comments[j].CreatedOn) {
			return comments[i].Id > comments[j].Id
		}
		return comments[i].CreatedOn.After(comments[j].CreatedOn)
	})
	result := []*SavedComment{}
	for _, comment := range comments {
		item, err := savedCommentDTO(ctx, comment, auth.User.ID)
		if errors.Is(err, errStaleSavedTarget) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func savedCommentDTO(ctx context.Context, comment *Comment, userId string) (*SavedComment, error) {
	article, blog, err := publicCommentContext(ctx, comment)
	if err != nil {
		return nil, errStaleSavedTarget
	}
	localizedArticle := preparePublicArticle(article, blog, preferredContentLanguage(ctx))
	localizedBlog := preparePublicBlog(blog, preferredContentLanguage(ctx))
	feedback, err := targetFeedback(ctx, interactionTarget{Kind: targetComment, Id: comment.Id, CreatedOn: comment.CreatedOn})
	if err != nil {
		return nil, err
	}
	return &SavedComment{
		Id: comment.Id, ParentId: comment.ParentId, Name: comment.Name, Text: comment.Text, CreatedOn: comment.CreatedOn,
		Mine: userId != "" && comment.UserId == userId, Feedback: *feedback, Url: canonicalCommentURL(article, blog, comment.Id),
		Article: SavedArticleContext{Id: article.Id, Title: localizedArticle.Title, PublicUrl: localizedArticle.PublicUrl},
		Blog:    SavedBlogContext{Id: blog.Id, Slug: blog.Slug, Name: blog.Name, PublicUrl: localizedBlog.PublicUrl},
	}, nil
}
