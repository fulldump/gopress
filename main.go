package main

import (
	"log"
	"net/http"

	"github.com/fulldump/goconfig"

	"gopress/api"
)

type Config struct {
	Addr    string `usage:"Server http address"`
	Statics string `usage:"Use a directory to serve statics or even a http server"`
}

func main() {

	c := &Config{
		Addr: "127.0.0.1:9955",
	}
	goconfig.Read(c)

	articles := map[string]*api.Article{
		"hello": api.Hello,
		"hello2": {
			Title:   "Two",
			Content: "222",
		},
		"hello3": {
			Title:   "Three",
			Content: "333",
		},
	}

	a := api.NewApi(articles, c.Statics)

	server := http.Server{
		Addr:    c.Addr,
		Handler: a,
	}

	log.Println("Listening on", server.Addr)
	server.ListenAndServe()
}
