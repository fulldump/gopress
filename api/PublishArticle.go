package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func PublishArticle(ctx context.Context, r *http.Request) (*Article, error) {
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
	// Check the lease before moderation, then build a fresh CAS filter afterwards.
	if err := verifyArticleLease(ctx, article); err != nil {
		return nil, err
	}
	article.Banned = false
	moderator := GetContentModerator(ctx)
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
			log.Println("publish article: moderation:", err)
			return nil, HttpError{Status: http.StatusInternalServerError, Title: "Article Publish Failed", Description: "No se pudo validar el contenido del artículo"}
		}
		article.Banned = banned
		if !banned {
			if generator, ok := moderator.(interface {
				GenerateTags(context.Context, string) ([]string, error)
			}); ok {
				tags, err := generator.GenerateTags(ctx, content)
				if err != nil {
					log.Println("publish article: generate tags:", err)
				} else if len(tags) > 0 {
					article.Tags = tags
				}
			}
		}
	}
	filter, err := articleMutationFilter(ctx, article, revision)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	publishOn := article.PublishOn
	if publishOn.IsZero() {
		publishOn = now
	}
	auth := glueauth.GetAuth(ctx)
	patched := &Article{}
	matched, err := GetInceptionClient(ctx).PatchOne("articles", inceptiondb.PatchQuery{Filter: filter, Patch: JSON{
		"published": true, "publishOn": publishOn, "banned": article.Banned, "tags": article.Tags,
		"revision": revision + 1, "updatedOn": now,
		"updated_by_id": auth.User.ID, "updated_by_nick": auth.User.Nick,
	}}, patched)
	if err != nil {
		return nil, HttpError{Status: http.StatusInternalServerError, Title: "Article Publish Failed", Description: "No se pudo guardar el artículo"}
	}
	if !matched {
		failure := articleMutationFailure(ctx, article, revision)
		return nil, failure
	}
	setArticlePublicURL(patched)
	return patched, nil
}
