package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/fulldump/apitest"
	"github.com/fulldump/box"

	"gopress/filestorage/localfilestore"
	"gopress/glueauth"
	"gopress/lib/deepseek"
)

type tagGeneratingModerator struct{}

func (tagGeneratingModerator) Evaluate(context.Context, string) (bool, error) { return false, nil }
func (tagGeneratingModerator) GenerateTags(context.Context, string) ([]string, error) {
	return []string{"español", "tecnología", "tecnología"}, nil
}

type tagFailingModerator struct{ tagGeneratingModerator }

func (tagFailingModerator) GenerateTags(context.Context, string) ([]string, error) {
	return nil, errors.New("generation unavailable")
}

func TestArticleTagsUseConfiguredDeepSeekClient(t *testing.T) {
	var calls atomic.Int32
	deepSeekServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"false"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"español, ciencia"}}]}`))
	}))
	t.Cleanup(deepSeekServer.Close)
	deepSeekClient, err := deepseek.NewClient(deepseek.Config{APIKey: "test-key", BaseURL: deepSeekServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(box.Box2Http(NewApi("", "test", db, fs, deepSeekClient)))
	request := func(method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication,
			`{"user":{"id":"owner","nick":"ana"}}`)
	}
	capabilities := request(http.MethodGet, "/v1/capabilities").Do()
	if capabilities.StatusCode != http.StatusOK || capabilities.BodyJsonMap()["automaticTags"] != true {
		t.Fatalf("DeepSeek capability is unavailable: %d %s", capabilities.StatusCode, capabilities.BodyString())
	}
	blog := request(http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "deepseek-tags", "name": "DeepSeek Tags",
	}).Do().BodyJsonMap()
	articlesPath := "/v1/blogs/" + blog["id"].(string) + "/articles"
	created := request(http.MethodPost, articlesPath).WithBodyJson(JSON{"title": "Ciencia en español"}).Do().BodyJsonMap()
	articlePath := articlesPath + "/" + created["id"].(string)
	lock := request(http.MethodPost, articlePath+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "deepseek-tags"}).Do().BodyJsonMap()
	published := request(http.MethodPost, articlePath+"/publish").
		WithHeader(ArticleLockTokenHeader, lock["token"].(string)).WithBodyJson(JSON{"revision": 1}).Do()
	if published.StatusCode != http.StatusOK {
		t.Fatalf("publish with DeepSeek: %d %s", published.StatusCode, published.BodyString())
	}
	tags := published.BodyJsonMap()["tags"].([]any)
	if calls.Load() != 2 || len(tags) != 2 || tags[0] != "español" || tags[1] != "ciencia" {
		t.Fatalf("DeepSeek tags were not persisted: calls=%d body=%s", calls.Load(), published.BodyString())
	}
}

func TestArticleTagsCanBeAutomaticOrManual(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(box.Box2Http(NewApi("", "test", db, fs, tagGeneratingModerator{})))
	request := func(method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication,
			`{"user":{"id":"owner","nick":"ana"}}`)
	}
	blog := request(http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "tags-blog", "name": "Tags Blog",
	}).Do().BodyJsonMap()
	articlesPath := "/v1/blogs/" + blog["id"].(string) + "/articles"

	createWithLock := func(title, clientId string) (string, string) {
		t.Helper()
		created := request(http.MethodPost, articlesPath).WithBodyJson(JSON{"title": title}).Do()
		if created.StatusCode != http.StatusOK {
			t.Fatalf("create article: %d %s", created.StatusCode, created.BodyString())
		}
		path := articlesPath + "/" + created.BodyJsonMap()["id"].(string)
		lock := request(http.MethodPost, path+"/edit-lock/acquire").WithBodyJson(JSON{"clientId": clientId}).Do()
		if lock.StatusCode != http.StatusOK {
			t.Fatalf("acquire lock: %d %s", lock.StatusCode, lock.BodyString())
		}
		return path, lock.BodyJsonMap()["token"].(string)
	}

	automaticPath, automaticToken := createWithLock("Artículo automático", "automatic-tags")
	automatic := request(http.MethodPost, automaticPath+"/publish").WithHeader(ArticleLockTokenHeader, automaticToken).
		WithBodyJson(JSON{"revision": 1}).Do()
	if automatic.StatusCode != http.StatusOK {
		t.Fatalf("publish automatic tags: %d %s", automatic.StatusCode, automatic.BodyString())
	}
	automaticTags := automatic.BodyJsonMap()["tags"].([]any)
	if len(automaticTags) != 2 || automaticTags[0] != "español" || automatic.BodyJsonMap()["tags_mode"] != articleTagsAutomatic {
		t.Fatalf("unexpected automatic tags: %s", automatic.BodyString())
	}

	manualPath, manualToken := createWithLock("Artículo manual", "manual-tags")
	manualPatch := request(http.MethodPatch, manualPath).WithHeader(ArticleLockTokenHeader, manualToken).WithBodyJson(JSON{
		"revision": 1, "tags": []string{"Opinión", "Go"}, "tags_mode": articleTagsManual,
	}).Do()
	if manualPatch.StatusCode != http.StatusOK {
		t.Fatalf("save manual tags: %d %s", manualPatch.StatusCode, manualPatch.BodyString())
	}
	manual := request(http.MethodPost, manualPath+"/publish").WithHeader(ArticleLockTokenHeader, manualToken).
		WithBodyJson(JSON{"revision": 2}).Do()
	if manual.StatusCode != http.StatusOK || manual.BodyJsonMap()["tags_mode"] != articleTagsManual {
		t.Fatalf("publish manual tags: %d %s", manual.StatusCode, manual.BodyString())
	}
	manualTags := manual.BodyJsonMap()["tags"].([]any)
	if len(manualTags) != 2 || manualTags[0] != "Opinión" || manualTags[1] != "Go" {
		t.Fatalf("manual tags were overwritten: %s", manual.BodyString())
	}
}

func TestArticleIsNotPublishedWhenAutomaticTagsFail(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(box.Box2Http(NewApi("", "test", db, fs, tagFailingModerator{})))
	request := func(method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication,
			`{"user":{"id":"owner","nick":"ana"}}`)
	}
	blog := request(http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "failed-tags", "name": "Failed Tags",
	}).Do().BodyJsonMap()
	articlesPath := "/v1/blogs/" + blog["id"].(string) + "/articles"
	created := request(http.MethodPost, articlesPath).WithBodyJson(JSON{"title": "Automatic tags"}).Do().BodyJsonMap()
	articlePath := articlesPath + "/" + created["id"].(string)
	lock := request(http.MethodPost, articlePath+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "failed-tags"}).Do().BodyJsonMap()
	published := request(http.MethodPost, articlePath+"/publish").
		WithHeader(ArticleLockTokenHeader, lock["token"].(string)).WithBodyJson(JSON{"revision": 1}).Do()
	if published.StatusCode != http.StatusBadGateway || errorCode(published) != "tag_generation_failed" {
		t.Fatalf("tag generation failure was hidden: %d %s", published.StatusCode, published.BodyString())
	}
	article := request(http.MethodGet, articlePath).Do().BodyJsonMap()
	if article["published"] != false {
		t.Fatalf("article was modified after tag generation failed: %v", article)
	}
}
