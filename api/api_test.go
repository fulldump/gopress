package api

import (
	"net/http"
	"testing"

	"github.com/fulldump/apitest"
	"github.com/fulldump/biff"
)

type JSON = map[string]any

func TestHappyPath(t *testing.T) {

	biff.Alternative("Setup gopress", func(a *biff.A) {

		h := NewApi()
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
		})

		a.Alternative("list articles - empty list", func(a *biff.A) {
			resp := api.Request("GET", "/v1/articles").Do()
			biff.AssertEqual(len(resp.BodyJson().([]any)), 0)
		})

	})

}
