package api

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fulldump/apitest"
	"github.com/fulldump/box"

	"gopress/filestorage/localfilestore"
	"gopress/glueauth"
)

type translationModerator struct{ tagGeneratingModerator }

func (translationModerator) TranslateArticle(_ context.Context, source, target, title, content string) (string, string, error) {
	return "English: " + title, `<p lang="` + target + `">Translated from ` + source + `</p>` + content, nil
}

type blockingTranslationModerator struct {
	started chan BlogLanguage
	release chan struct{}
}

func (moderator blockingTranslationModerator) Evaluate(context.Context, string) (bool, error) {
	return true, nil
}

func (moderator blockingTranslationModerator) TranslateArticle(ctx context.Context, _, target, title, content string) (string, string, error) {
	select {
	case moderator.started <- BlogLanguage(target):
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
	select {
	case <-moderator.release:
		return title + " " + target, content, nil
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

func TestArticleTranslationsArePublishedWithSEOAlternates(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(box.Box2Http(NewApi("", "test", db, fs, translationModerator{})))
	request := func(method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication,
			`{"user":{"id":"owner","nick":"ana"}}`)
	}
	blogResponse := request(http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "translated-blog", "name": "Translated", "language": "es", "target_languages": []string{"en", "fr"},
	}).Do()
	if blogResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create translated blog: %d %s", blogResponse.StatusCode, blogResponse.BodyString())
	}
	blog := blogResponse.BodyJsonMap()
	articlesPath := "/v1/blogs/" + blog["id"].(string) + "/articles"
	created := request(http.MethodPost, articlesPath).WithBodyJson(JSON{"title": "Articulo original"}).Do().BodyJsonMap()
	articlePath := articlesPath + "/" + created["id"].(string)
	lock := request(http.MethodPost, articlePath+"/edit-lock/acquire").WithBodyJson(JSON{"clientId": "translations"}).Do().BodyJsonMap()
	published := request(http.MethodPost, articlePath+"/publish").WithHeader(ArticleLockTokenHeader, lock["token"].(string)).
		WithBodyJson(JSON{"revision": 1}).Do()
	if published.StatusCode != http.StatusOK {
		t.Fatalf("publish translations: %d %s", published.StatusCode, published.BodyString())
	}
	translations := published.BodyJsonMap()["translations"].([]any)
	if len(translations) != 2 {
		t.Fatalf("missing translations: %s", published.BodyString())
	}

	translated := client.Request(http.MethodGet, "/blog/translated-blog/article/articulo-original/en").Do()
	if translated.StatusCode != http.StatusOK || !containsAll(translated.BodyString(),
		"English: Articulo original", `lang="en"`, `rel="canonical" href="https://gopress.org/blog/translated-blog/article/articulo-original/en"`,
		`hreflang="es"`, `hreflang="en"`, `hreflang="fr"`, `hreflang="x-default"`) {
		t.Fatalf("translated SEO page: %d %s", translated.StatusCode, translated.BodyString())
	}
	original := client.Request(http.MethodGet, "/blog/translated-blog/article/articulo-original").Do()
	if original.StatusCode != http.StatusOK || !containsAll(original.BodyString(), `hreflang="en"`, `hreflang="fr"`) {
		t.Fatalf("original alternates: %d %s", original.StatusCode, original.BodyString())
	}
	removeFrench := request(http.MethodPatch, "/v1/blogs/"+blog["id"].(string)).WithBodyJson(JSON{"target_languages": []string{"en"}}).Do()
	if removeFrench.StatusCode != http.StatusOK {
		t.Fatalf("remove target language: %d %s", removeFrench.StatusCode, removeFrench.BodyString())
	}
	removed := client.Request(http.MethodGet, "/blog/translated-blog/article/articulo-original/fr").Do()
	if removed.StatusCode != http.StatusNotFound {
		t.Fatalf("removed language remained public: %d %s", removed.StatusCode, removed.BodyString())
	}
	patched := request(http.MethodPatch, articlePath).WithHeader(ArticleLockTokenHeader, lock["token"].(string)).
		WithBodyJson(JSON{"revision": 2, "title": "Articulo actualizado"}).Do()
	if patched.StatusCode != http.StatusOK {
		t.Fatalf("update source: %d %s", patched.StatusCode, patched.BodyString())
	}
	stale := client.Request(http.MethodGet, "/blog/translated-blog/article/articulo-original/en").Do()
	if stale.StatusCode != http.StatusNotFound {
		t.Fatalf("stale translation remained public: %d %s", stale.StatusCode, stale.BodyString())
	}
}

func TestArticleContentEqualityIgnoresEditorMetadata(t *testing.T) {
	a := Content{Type: "editorjs", Data: []byte(`{"time":1,"version":"1","blocks":[{"type":"paragraph","data":{"text":"Same"}}]}`)}
	b := Content{Type: "editorjs", Data: []byte(`{"version":"2","blocks":[{"data":{"text":"Same"},"type":"paragraph"}],"time":2}`)}
	if !articleContentEqual(a, b) {
		t.Fatal("EditorJS metadata invalidated unchanged content")
	}
}

func TestTranslatedHTMLUsesStrictAllowlist(t *testing.T) {
	result := sanitizeTranslatedHTML(`<p onclick="alert(1)">Safe</p><iframe srcdoc="<script>alert(1)</script>"></iframe><a href="javascript:alert(1)">link</a>`)
	if strings.Contains(result, "onclick") || strings.Contains(result, "iframe") || strings.Contains(result, "javascript:") || !strings.Contains(result, "Safe") {
		t.Fatalf("unsafe translated HTML: %s", result)
	}
}

func TestArticleTranslationsStartTargetsConcurrently(t *testing.T) {
	moderator := blockingTranslationModerator{started: make(chan BlogLanguage, 2), release: make(chan struct{})}
	ctx := context.WithValue(context.Background(), contentModeratorKey, moderator)
	article := &Article{ArticleUserFields: ArticleUserFields{Title: "Original", ContentHTML: template.HTML("<p>Content</p>")}, SourceLanguage: BlogLanguageES, ContentRevision: 1}
	blog := &Blog{Language: BlogLanguageES, TargetLanguages: []BlogLanguage{BlogLanguageEN, BlogLanguageFR}}
	result := make(chan []ArticleTranslation, 1)
	errors := make(chan error, 1)
	go func() {
		translations, err := generateArticleTranslations(ctx, article, blog)
		result <- translations
		errors <- err
	}()
	released := false
	defer func() {
		if !released {
			close(moderator.release)
		}
	}()

	started := map[BlogLanguage]bool{}
	for len(started) < 2 {
		select {
		case language := <-moderator.started:
			started[language] = true
		case <-time.After(time.Second):
			t.Fatal("translation targets did not start concurrently")
		}
	}
	close(moderator.release)
	released = true
	if err := <-errors; err != nil {
		t.Fatal(err)
	}
	if translations := <-result; len(translations) != 2 {
		t.Fatalf("expected 2 translations, got %d", len(translations))
	}
}

func TestArticleTranslationsAreDisabledWithoutTargets(t *testing.T) {
	article := &Article{SourceLanguage: BlogLanguageEN, ContentRevision: 1}
	blog := &Blog{Language: BlogLanguageES}
	translations, err := generateArticleTranslations(context.Background(), article, blog)
	if err != nil {
		t.Fatal(err)
	}
	if len(translations) != 0 {
		t.Fatalf("expected no translations, got %d", len(translations))
	}
}

func containsAll(value string, expected ...string) bool {
	for _, item := range expected {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}
