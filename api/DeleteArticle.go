package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"gopress/inceptiondb"
)

func DeleteArticle(ctx context.Context, r *http.Request) (*Article, error) {
	article, _, _, err := FindAccessibleArticle(ctx)
	if err != nil {
		return nil, err
	}
	revision, err := decodeArticleMutationRevision(ctx, r)
	if err != nil {
		return nil, err
	}
	if revision != article.Revision {
		return nil, articleRevisionConflict(article.Revision)
	}
	filter, err := articleMutationFilter(ctx, article, revision)
	if err != nil {
		return nil, err
	}
	patched := &Article{}
	matched, err := GetInceptionClient(ctx).PatchOne("articles", inceptiondb.PatchQuery{Filter: filter, Patch: JSON{
		"revision": revision + 1, "_deleting_on": time.Now().UTC(),
	}}, patched)
	if err != nil {
		return nil, HttpError{Status: http.StatusInternalServerError, Title: "Article Delete Failed", Description: "No se pudo eliminar el artículo"}
	}
	if !matched {
		failure := articleMutationFailure(ctx, article, revision)
		return nil, failure
	}
	removed, err := GetInceptionClient(ctx).Remove("articles", inceptiondb.FindQuery{Limit: 1, Filter: JSON{
		"id": patched.Id, "blog_id": patched.BlogId, "createdOn": patched.CreatedOn, "revision": patched.Revision,
	}})
	if err != nil {
		return nil, HttpError{Status: http.StatusInternalServerError, Title: "Article Delete Failed", Description: "No se pudo eliminar el artículo"}
	}
	defer removed.Close()
	deleted := &Article{}
	if err := json.NewDecoder(removed).Decode(deleted); err != nil {
		return nil, HttpError{Status: http.StatusConflict, Title: "Article Delete Conflict", Code: "revision_conflict", Description: "El artículo cambió antes de eliminarse"}
	}

	if err := removeArticleRows(ctx, articleInteractionsCollection, JSON{
		"articleId": deleted.Id, "articleCreatedOn": deleted.CreatedOn,
	}); err != nil {
		return nil, HttpError{Status: http.StatusInternalServerError, Title: "Could Not Delete Article Feedback", Description: "El artículo se eliminó, pero no se pudo limpiar su feedback"}
	}
	if err := removeArticleRows(ctx, commentsCollection, JSON{"articleId": deleted.Id}); err != nil {
		return nil, HttpError{Status: http.StatusInternalServerError, Title: "Could Not Delete Article Comments", Description: "El artículo se eliminó, pero no se pudieron limpiar sus comentarios"}
	}
	return deleted, nil
}

func removeArticleRows(ctx context.Context, collection string, filter JSON) error {
	for {
		rows, err := GetInceptionClient(ctx).Remove(collection, inceptiondb.FindQuery{Filter: filter, Limit: 1000})
		if err != nil {
			return err
		}
		removed, readErr := io.Copy(io.Discard, rows)
		rows.Close()
		if readErr != nil {
			return readErr
		}
		if removed == 0 {
			return nil
		}
	}
}
