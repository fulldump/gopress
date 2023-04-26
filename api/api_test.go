package api

import (
	"net/http"
	"testing"

	"github.com/fulldump/apitest"
	"github.com/fulldump/biff"
)

func TestHappyPath(t *testing.T) {

	biff.Alternative("Setup gopress", func(a *biff.A) {

		articles := map[string]*Article{}
		h := NewApi(articles)
		api := apitest.NewWithHandler(h)

		a.Alternative("create article", func(a *biff.A) {
			resp := api.Request("POST", "/v1/articles").WithBodyJson(JSON{
				"id":    "hello-world",
				"title": "Hello world",
			}).Do()

			biff.AssertEqual(resp.StatusCode, http.StatusOK)

			body := *resp.BodyJsonMap()
			biff.AssertEqual(body["title"], "Hello world")
			biff.AssertEqual(body["content"], "Start here")
			biff.AssertEqual(body["published"], false)

			a.Alternative("list articles", func(a *biff.A) {
				resp2 := api.Request("GET", "/v1/articles").Do()
				body := resp2.BodyJson().([]any)
				biff.AssertEqual(body[0].(map[string]any)["id"], "hello-world")
			})
			a.Alternative("create article - already exist", func(a *biff.A) {
				resp := api.Request("POST", "/v1/articles").WithBodyJson(JSON{
					"id":    "hello-world",
					"title": "Hello world",
				}).Do()

				body := *resp.BodyJsonMap()
				biff.AssertEqual(body["error"], "article id 'hello-world' already exists")
			})
			a.Alternative("retrieve article", func(a *biff.A) {
				resp := api.Request("GET", "/v1/articles/hello-world").Do()

				body := *resp.BodyJsonMap()
				biff.AssertEqual(body["title"], "Hello world")
			})
			a.Alternative("delete article", func(a *biff.A) {
				resp := api.Request("DELETE", "/v1/articles/hello-world").Do()

				biff.AssertEqual(resp.StatusCode, 200)
				body := *resp.BodyJsonMap()
				biff.AssertEqual(body["title"], "Hello world")

				a.Alternative("list articles - after delete", func(a *biff.A) {
					resp := api.Request("GET", "/v1/articles").Do()

					biff.AssertEqual(resp.StatusCode, 200)
					biff.AssertEqualJson(resp.BodyJson(), []any{})
				})
				a.Alternative("retrieve article - after delete", func(a *biff.A) {
					resp := api.Request("GET", "/v1/articles/hello-world").Do()

					biff.AssertEqual(resp.StatusCode, 404)
				})
			})
		})

		a.Alternative("list articles - empty list", func(a *biff.A) {
			resp := api.Request("GET", "/v1/articles").Do()
			biff.AssertEqual(len(resp.BodyJson().([]any)), 0)
		})

		a.Alternative("retrieve article - not found", func(a *biff.A) {
			resp := api.Request("GET", "/v1/articles/invented").Do()

			biff.AssertEqual(resp.StatusCode, 404)
			body := *resp.BodyJsonMap()
			biff.AssertEqual(body["error"], "article not found")
		})

		a.Alternative("delete article - not found", func(a *biff.A) {
			resp := api.Request("DELETE", "/v1/articles/invented").Do()

			biff.AssertEqual(resp.StatusCode, 404)
			body := *resp.BodyJsonMap()
			biff.AssertEqual(body["error"], "article not found")
		})

	})

}
