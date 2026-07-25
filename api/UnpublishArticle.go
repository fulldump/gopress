package api

import (
	"context"
	"net/http"
	"time"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func UnpublishArticle(ctx context.Context, r *http.Request) any {
	article, _, _, err := FindAccessibleArticle(ctx)
	if err != nil {
		return err
	}
	revision, err := decodeArticleMutationRevision(ctx, r)
	if err != nil {
		return err
	}
	if revision != article.Revision {
		return articleRevisionConflict(article.Revision)
	}
	filter, err := articleMutationFilter(ctx, article, revision)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	auth := glueauth.GetAuth(ctx)
	patched := &Article{}
	matched, err := GetInceptionClient(ctx).PatchOne("articles", inceptiondb.PatchQuery{Filter: filter, Patch: JSON{
		"published": false, "revision": revision + 1, "updatedOn": now,
		"updated_by_id": auth.User.ID, "updated_by_nick": auth.User.Nick,
	}}, patched)
	if err != nil {
		return HttpError{Status: http.StatusInternalServerError, Title: "Article Unpublish Failed", Description: "No se pudo guardar el artículo"}
	}
	if !matched {
		return articleMutationFailure(ctx, article, revision)
	}
	setArticlePublicURL(patched)
	return patched
}
