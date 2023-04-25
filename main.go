package main

import (
	"fmt"
	"net/http"

	"github.com/fulldump/box"
)

func main() {

	b := box.NewBox()

	b.Handle("GET", "/v1/articles", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: list articles"
	})

	b.Handle("POST", "/v1/articles", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: create article"
	})

	b.Handle("GET", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: get article"
	})

	b.Handle("PATCH", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: modify article"
	})

	b.Handle("DELETE", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: delete article"
	})

	b.Handle("POST", "/v1/articles/{articleId}/publish", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: publish article"
	})

	server := http.Server{
		Addr:    ":9955",
		Handler: b,
	}

	fmt.Println("Listening on", server.Addr)
	server.ListenAndServe()
}
