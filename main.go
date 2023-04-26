package main

import (
	"fmt"
	"net/http"

	"gopress/api"
)

func main() {

	articles := map[string]*api.Article{
		"hello": api.Hello,
	}

	a := api.NewApi(articles)

	server := http.Server{
		Addr:    ":9955",
		Handler: a,
	}

	fmt.Println("Listening on", server.Addr)
	server.ListenAndServe()
}
