package main

import (
	"fmt"
	"net/http"

	"github.com/fulldump/goconfig"

	"gopress/api"
)

type Config struct {
	Addr string `usage:"Server http address"`
}

func main() {

	c := &Config{
		Addr: "127.0.0.1:9955",
	}
	goconfig.Read(c)

	articles := map[string]*api.Article{
		"hello": api.Hello,
	}

	a := api.NewApi(articles)

	server := http.Server{
		Addr:    c.Addr,
		Handler: a,
	}

	fmt.Println("Listening on", server.Addr)
	server.ListenAndServe()
}
