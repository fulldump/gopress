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
	article, blog, _, err := FindAccessibleArticle(ctx)
	if err != nil {
		return nil, err
	}
	if blog.Banned {
		return nil, blogBanned()
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
		if !banned && articleTagsAreAutomatic(article.TagsMode) {
			if generator := GetTagGenerator(ctx); generator != nil {
				tagContent := strings.TrimSpace(article.Title + "\n" + content)
				tags, err := generator.GenerateTags(ctx, tagContent)
				if err != nil {
					log.Println("publish article: generate tags:", err)
					return nil, articleTagGenerationFailed()
				}
				tags, err = normalizeArticleTags(tags)
				if err != nil {
					log.Println("publish article: invalid generated tags:", err)
					return nil, articleTagGenerationFailed()
				}
				if len(tags) == 0 {
					log.Println("publish article: tag generator returned no tags")
					return nil, articleTagGenerationFailed()
				}
				article.Tags = tags
				article.TagsMode = articleTagsAutomatic
			}
		}
	}
	filter, err := articleMutationFilter(ctx, article, revision)
	if err != nil {
		return nil, err
	}
	if article.Banned {
		// Fail closed: the moderation decision itself is enough to hide the blog, even if article persistence later conflicts.
		if _, err := banBlogForArticle(GetInceptionClient(ctx), article); err != nil {
			log.Printf("publish article: ban blog: %v", err)
			return nil, HttpError{Status: http.StatusInternalServerError, Title: "Article Publish Failed", Description: "No se pudo bloquear el blog tras detectar contenido prohibido"}
		}
	} else {
		article.Translations, err = generateArticleTranslations(ctx, article, blog)
		if err != nil {
			log.Println("publish article: generate translations:", err)
			return nil, HttpError{Status: http.StatusBadGateway, Title: "Article Translation Failed", Code: "translation_generation_failed",
				Description: "No se pudieron generar las traducciones del artículo. Inténtalo de nuevo"}
		}
	}
	now := time.Now().UTC()
	publishOn := article.PublishOn
	if publishOn.IsZero() {
		publishOn = now
	}
	auth := glueauth.GetAuth(ctx)
	patched := &Article{}
	matched, err := GetInceptionClient(ctx).PatchOne("articles", inceptiondb.PatchQuery{Filter: filter, Patch: JSON{
		"published": true, "publishOn": publishOn, "banned": article.Banned, "tags": article.Tags, "tags_mode": article.TagsMode,
		"source_language": article.SourceLanguage, "content_revision": article.ContentRevision, "translations": article.Translations,
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

func articleTagGenerationFailed() HttpError {
	return HttpError{Status: http.StatusBadGateway, Title: "Article Tag Generation Failed", Code: "tag_generation_failed",
		Description: "No se pudieron generar los tags con IA. Inténtalo de nuevo o añade tags manualmente"}
}
