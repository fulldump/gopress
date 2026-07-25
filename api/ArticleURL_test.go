package api

import (
	"net/http"
	"testing"

	"github.com/fulldump/apitest"
	"github.com/fulldump/box"

	"gopress/filestorage/localfilestore"
	"gopress/glueauth"
)

func TestArticleURLCanBeGeneratedAndEdited(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(box.Box2Http(NewApi("", "test", db, fs, nil)))
	request := func(method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication,
			`{"user":{"id":"owner","nick":"ana"}}`)
	}
	createdBlog := request(http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "url-blog", "name": "URL Blog",
	}).Do()
	if createdBlog.StatusCode != http.StatusCreated {
		t.Fatalf("create blog: %d %s", createdBlog.StatusCode, createdBlog.BodyString())
	}
	blogId := createdBlog.BodyJsonMap()["id"].(string)
	articlesPath := "/v1/blogs/" + blogId + "/articles"

	created := request(http.MethodPost, articlesPath).WithBodyJson(JSON{"title": ""}).Do()
	if created.StatusCode != http.StatusOK {
		t.Fatalf("create article: %d %s", created.StatusCode, created.BodyString())
	}
	article := created.BodyJsonMap()
	if article["url"] != "article" || article["automatic_url"] != true {
		t.Fatalf("new article URL is not automatic: %v", article)
	}
	articleId := article["id"].(string)
	articlePath := articlesPath + "/" + articleId
	lock := request(http.MethodPost, articlePath+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "url-test"}).Do().BodyJsonMap()
	token := lock["token"].(string)
	patch := func(body JSON) *apitest.Response {
		return request(http.MethodPatch, articlePath).
			WithHeader(ArticleLockTokenHeader, token).WithBodyJson(body).Do()
	}

	automatic := patch(JSON{"revision": 1, "title": "Hello world"})
	if automatic.StatusCode != http.StatusOK {
		t.Fatalf("generate URL: %d %s", automatic.StatusCode, automatic.BodyString())
	}
	if body := automatic.BodyJsonMap(); body["url"] != "hello-world" || body["publicUrl"] != "/blog/url-blog/article/hello-world" {
		t.Fatalf("unexpected generated URL: %v", body)
	}

	manual := patch(JSON{"revision": 2, "url": "custom-path", "automatic_url": false})
	if manual.StatusCode != http.StatusOK || manual.BodyJsonMap()["url"] != "custom-path" {
		t.Fatalf("set manual URL: %d %s", manual.StatusCode, manual.BodyString())
	}
	preserved := patch(JSON{"revision": 3, "title": "A renamed article"})
	if preserved.StatusCode != http.StatusOK || preserved.BodyJsonMap()["url"] != "custom-path" {
		t.Fatalf("manual URL changed with title: %d %s", preserved.StatusCode, preserved.BodyString())
	}

	published := request(http.MethodPost, articlePath+"/publish").WithHeader(ArticleLockTokenHeader, token).
		WithBodyJson(JSON{"revision": 4}).Do()
	if published.StatusCode != http.StatusOK {
		t.Fatalf("publish: %d %s", published.StatusCode, published.BodyString())
	}
	changedPublished := patch(JSON{"revision": 5, "url": "published-path", "automatic_url": false})
	if changedPublished.StatusCode != http.StatusOK || changedPublished.BodyJsonMap()["url"] != "published-path" {
		t.Fatalf("edit published URL: %d %s", changedPublished.StatusCode, changedPublished.BodyString())
	}

	invalid := patch(JSON{"revision": 6, "url": "Invalid URL", "automatic_url": false})
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid URL accepted: %d %s", invalid.StatusCode, invalid.BodyString())
	}

	secondResponse := request(http.MethodPost, articlesPath).WithBodyJson(JSON{"title": "Published path"}).Do()
	if secondResponse.StatusCode != http.StatusOK {
		t.Fatalf("create article with repeated title: %d %s", secondResponse.StatusCode, secondResponse.BodyString())
	}
	second := secondResponse.BodyJsonMap()
	if second["url"] != "published-path-2" {
		t.Fatalf("automatic URL is not unique: %v", second)
	}
	secondPath := articlesPath + "/" + second["id"].(string)
	secondLock := request(http.MethodPost, secondPath+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "url-conflict-test"}).Do().BodyJsonMap()
	conflict := request(http.MethodPatch, secondPath).WithHeader(ArticleLockTokenHeader, secondLock["token"].(string)).
		WithBodyJson(JSON{"revision": 1, "url": "published-path", "automatic_url": false}).Do()
	if conflict.StatusCode != http.StatusConflict || errorCode(conflict) != "article_url_conflict" {
		t.Fatalf("duplicate URL accepted: %d %s", conflict.StatusCode, conflict.BodyString())
	}
}
