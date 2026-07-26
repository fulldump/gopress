package api

import (
	"context"
	"errors"
	"html/template"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fulldump/box"

	"gopress/i18n"
)

func generateArticleTranslations(ctx context.Context, article *Article, blog *Blog) ([]ArticleTranslation, error) {
	if article.ContentRevision == 0 {
		article.ContentRevision = 1
	}
	if article.SourceLanguage == "" {
		article.SourceLanguage = blog.Language
	}
	generator := GetTranslationGenerator(ctx)
	existing := map[BlogLanguage]ArticleTranslation{}
	for _, translation := range article.Translations {
		existing[translation.Language] = translation
	}
	targets := blogTranslationLanguages(blog)
	result := make([]ArticleTranslation, len(targets))
	errorsByTarget := make([]error, len(targets))
	var wait sync.WaitGroup
	for index, language := range targets {
		if language == article.SourceLanguage {
			continue
		}
		if translation, ok := existing[language]; ok && translation.SourceContentRevision == article.ContentRevision {
			result[index] = translation
			continue
		}
		if generator == nil {
			return nil, errors.New("translation generator is not configured")
		}
		wait.Add(1)
		go func(index int, language BlogLanguage) {
			defer wait.Done()
			title, contentHTML, err := generator.TranslateArticle(ctx, string(article.SourceLanguage), string(language), article.Title, string(article.ContentHTML))
			if err != nil {
				errorsByTarget[index] = err
				return
			}
			result[index] = ArticleTranslation{Language: language, Title: title,
				ContentHTML: template.HTML(sanitizeTranslatedHTML(contentHTML)), SourceContentRevision: article.ContentRevision, GeneratedOn: time.Now().UTC()}
		}(index, language)
	}
	wait.Wait()
	compacted := make([]ArticleTranslation, 0, len(result))
	for index, translation := range result {
		if errorsByTarget[index] != nil {
			log.Printf("publish article: translate to %s: %v", targets[index], errorsByTarget[index])
			return nil, errorsByTarget[index]
		}
		if translation.Language != "" {
			compacted = append(compacted, translation)
		}
	}
	sort.Slice(compacted, func(i, j int) bool { return compacted[i].Language < compacted[j].Language })
	return compacted, nil
}

func articleTranslation(article *Article, language BlogLanguage) *ArticleTranslation {
	for index := range article.Translations {
		translation := &article.Translations[index]
		if translation.Language == language && translation.SourceContentRevision == article.ContentRevision {
			return translation
		}
	}
	return nil
}

func blogOffersLanguage(blog *Blog, language BlogLanguage) bool {
	for _, available := range blogTranslationLanguages(blog) {
		if available == language {
			return true
		}
	}
	return false
}

func articleTranslationPath(blogSlug, articleURL string, language BlogLanguage) string {
	return articlePublicPath(blogSlug, articleURL) + "/" + url.PathEscape(string(language))
}

func preparePublicArticle(article *Article, blog *Blog, preferred BlogLanguage) *Article {
	copy := *article
	if copy.SourceLanguage == "" {
		copy.SourceLanguage = blog.Language
	}
	copy.AvailableLanguages = availableArticleLanguages(&copy, blog, preferred)
	copy.PublicUrl = articlePublicPath(blog.Slug, copy.Url)
	if translation := articleTranslation(&copy, preferred); blogOffersLanguage(blog, preferred) && translation != nil {
		copy.Title = translation.Title
		copy.ContentHTML = translation.ContentHTML
		copy.ContentSummary = ""
		copy.PublicUrl = articleTranslationPath(blog.Slug, copy.Url, preferred)
	}
	ensureArticleSummary(&copy)
	return &copy
}

func availableArticleLanguages(article *Article, blog *Blog, current BlogLanguage) []LanguageLink {
	source := article.SourceLanguage
	if source == "" {
		source = blog.Language
	}
	result := []LanguageLink{{Language: source, Url: articlePublicPath(blog.Slug, article.Url), Current: current == source}}
	for _, language := range blogTranslationLanguages(blog) {
		if translation := articleTranslation(article, language); translation != nil {
			result = append(result, LanguageLink{Language: language, Url: articleTranslationPath(blog.Slug, article.Url, language), Current: current == language})
		}
	}
	return result
}

func blogTranslationLanguages(blog *Blog) []BlogLanguage {
	if len(blog.TargetLanguages) == 0 {
		return nil
	}
	result := []BlogLanguage{blog.Language}
	seen := map[BlogLanguage]bool{blog.Language: true}
	for _, language := range blog.TargetLanguages {
		if !seen[language] {
			seen[language] = true
			result = append(result, language)
		}
	}
	return result
}

func preparePublicBlog(blog *Blog, preferred BlogLanguage) *Blog {
	copy := *blog
	copy.AvailableLanguages = []LanguageLink{{Language: blog.Language, Url: "/blog/" + url.PathEscape(blog.Slug), Current: preferred == blog.Language}}
	for _, language := range blog.TargetLanguages {
		copy.AvailableLanguages = append(copy.AvailableLanguages, LanguageLink{Language: language,
			Url: "/blog/" + url.PathEscape(blog.Slug) + "?lang=" + url.QueryEscape(string(language)), Current: preferred == language})
	}
	copy.PublicUrl = "/blog/" + url.PathEscape(blog.Slug)
	for _, link := range copy.AvailableLanguages {
		if link.Language == preferred {
			copy.PublicUrl = link.Url
			break
		}
	}
	return &copy
}

func contentLanguageFromLocale(locale string) BlogLanguage {
	language := BlogLanguage(strings.ToLower(locale))
	if IsSupportedContentLanguage(language) {
		return language
	}
	return BlogLanguageES
}

func preferredContentLanguage(ctx context.Context) BlogLanguage {
	if language := BlogLanguage(strings.ToLower(box.GetRequest(ctx).URL.Query().Get("lang"))); IsSupportedContentLanguage(language) {
		return language
	}
	return contentLanguageFromLocale(string(i18n.GetLocale(ctx)))
}
