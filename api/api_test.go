package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/fulldump/apitest"
	"github.com/fulldump/biff"
	"github.com/fulldump/box"
	"github.com/fulldump/inceptiondb/api"
	"github.com/fulldump/inceptiondb/database"
	"github.com/fulldump/inceptiondb/service"

	"gopress/filestorage/localfilestore"
	"gopress/glueauth"
	"gopress/inceptiondb"
)

func TestHappyPath(t *testing.T) {

	inceptionStandalone(t.TempDir(), "127.0.0.1:5555")

	biff.Alternative("Setup gopress", func(a *biff.A) {

		db := inceptiondb.NewClient(inceptiondb.Config{
			// Base: "https://inceptiondb.io/v1",
			Base:       "http://" + "127.0.0.1:5555" + "/v1",
			DatabaseID: "",
		})

		db.DropCollection("articles")

		fs, err := localfilestore.New(t.TempDir())
		biff.AssertNil(err)

		h := NewApi("", "test-version", db, fs)
		api := apitest.NewWithHandler(h)

		apiRequestJohn := func(method, path string) *apitest.Request {
			return api.Request(method, path).
				WithHeader(glueauth.XGlueAuthentication, `{"user":{"id":"my-user-id","nick":"john"}}`)
		}

		a.Alternative("create article", func(a *biff.A) {
			resp := apiRequestJohn("POST", "/v1/articles").WithBodyJson(JSON{
				"id":    "hello-world",
				"title": "Hello world",
			}).Do()

			biff.AssertEqual(resp.StatusCode, http.StatusOK)

			body := resp.BodyJsonMap()
			biff.AssertEqual(body["title"], "Hello world")
			biff.AssertNotNil(body["content"])
			biff.AssertEqual(body["published"], false)

			a.Alternative("list articles", func(a *biff.A) {
				resp2 := apiRequestJohn("GET", "/v1/articles").Do()
				body := resp2.BodyJson().([]any)
				biff.AssertEqual(body[0].(map[string]any)["id"], "hello-world")
			})
			// a.Alternative("create article - already exist", func(a *biff.A) {
			// 	resp := apiRequestJohn("POST", "/v1/articles").WithBodyJson(JSON{
			// 		"id":    "hello-world",
			// 		"title": "Hello world",
			// 	}).Do()
			//
			// 	body := resp.BodyJsonMap()
			// 	biff.AssertEqual(body["error"], "article id 'hello-world' already exists")
			// })
			a.Alternative("retrieve article", func(a *biff.A) {
				resp := apiRequestJohn("GET", "/v1/articles/hello-world").Do()

				body := resp.BodyJsonMap()
				biff.AssertEqual(body["title"], "Hello world")
			})
			a.Alternative("delete article", func(a *biff.A) {
				resp := apiRequestJohn("DELETE", "/v1/articles/hello-world").Do()

				biff.AssertEqual(resp.StatusCode, 200)
				body := resp.BodyJsonMap()
				biff.AssertEqual(body["title"], "Hello world")

				a.Alternative("list articles - after delete", func(a *biff.A) {
					resp := apiRequestJohn("GET", "/v1/articles").Do()

					biff.AssertEqual(resp.StatusCode, 200)
					biff.AssertEqualJson(resp.BodyJson(), []any{})
				})
				a.Alternative("retrieve article - after delete", func(a *biff.A) {
					resp := apiRequestJohn("GET", "/v1/articles/hello-world").Do()

					biff.AssertEqual(resp.StatusCode, 404)
				})
			})
			a.Alternative("modify article - content", func(a *biff.A) {
				resp := apiRequestJohn("PATCH", "/v1/articles/hello-world").
					WithBodyJson(JSON{
						"content": JSON{
							"type": "editorjs",
							"data": JSON{
								"blocks": []JSON{},
							},
						},
					}).Do()

				biff.AssertEqual(resp.StatusCode, 200)
				body := resp.BodyJsonMap()
				biff.AssertEqual(body["title"], "Hello world")
				biff.AssertEqualJson(body["content"], JSON{
					"type": "editorjs",
					"data": JSON{
						"blocks": []JSON{},
					},
				})
			})
		})

		a.Alternative("list articles - empty list", func(a *biff.A) {
			resp := apiRequestJohn("GET", "/v1/articles").Do()
			biff.AssertEqual(len(resp.BodyJson().([]any)), 0)
		})

		a.Alternative("retrieve article - not found", func(a *biff.A) {
			resp := apiRequestJohn("GET", "/v1/articles/invented").Do()

			biff.AssertEqual(resp.StatusCode, 404)
			body := resp.BodyJsonMap()
			biff.AssertEqual(body["error"], "article not found")
		})

		a.Alternative("delete article - not found", func(a *biff.A) {
			resp := apiRequestJohn("DELETE", "/v1/articles/invented").Do()

			biff.AssertEqual(resp.StatusCode, 404)
			body := resp.BodyJsonMap()
			biff.AssertEqual(body["error"], "article not found")
		})

	})

}

func TestEditorJs(t *testing.T) {

	for _, c := range []struct {
		block  JSON
		output string
	}{
		{
			block: JSON{
				"type": "header",
				"data": JSON{
					"level": 2,
					"text":  "My title",
				},
			},
			output: "<h2>My title</h2>\n",
		},
		{
			block: JSON{
				"type": "paragraph",
				"data": JSON{
					"text": "My paragraph",
				},
			},
			output: "<p>My paragraph</p>\n",
		},
		{
			block: JSON{
				"type": "list",
				"data": JSON{
					"style": "ordered",
					"items": []string{"one", "two"},
				},
			},
			output: "<ol>\n<li>one</li>\n<li>two</li>\n</ol>\n",
		},
	} {

		t.Run(c.block["type"].(string), func(t *testing.T) {
			data, marshalErr := json.Marshal(JSON{"blocks": []JSON{c.block}})
			biff.AssertNil(marshalErr)

			output, editor2htmlErr := editorjs2HTML(data)
			biff.AssertNil(editor2htmlErr)
			biff.AssertEqual(output, c.output)
		})

	}

}

func inceptionStandalone(dir, addr string) {
	db := database.NewDatabase(&database.Config{
		Dir: dir,
	})

	b := api.Build(service.NewService(db), "", "embedded")
	b.WithInterceptors(
		api.AccessLog(log.New(os.Stdout, "ACCESS: ", log.Lshortfile)),
		api.RecoverFromPanic,
		api.PrettyErrorInterceptor,
		api.InterceptorUnavailable(db),
	)

	s := &http.Server{
		Addr:    addr,
		Handler: box.Box2Http(b),
	}

	err := db.Load()
	if err != nil {
		panic(err)
	}

	go func() {
		err := s.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
}
