package main

import (
	"log"
	"net/http"

	"github.com/fulldump/goconfig"

	"gopress/api"
	"gopress/inceptiondb"
)

type Config struct {
	Addr      string `usage:"Server http address"`
	Statics   string `usage:"Use a directory to serve statics or even a http server"`
	Inception inceptiondb.Config
}

func main() {

	c := &Config{
		Addr: "127.0.0.1:9955",
		Inception: inceptiondb.Config{
			Base:       "https://saas.inceptiondb.io/v1",
			DatabaseID: "fc84637c-6c31-400d-a312-7ec6de39fd2d",
			ApiKey:     "b76f9c1a-93fa-4988-84ee-cd761cee66f3",
			ApiSecret:  "52392c50-a2dc-4902-a492-062b210e5839",
		},
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

	db := inceptiondb.NewClient(c.Inception)

	a := api.NewApi(articles, c.Statics, db)

	server := http.Server{
		Addr:    c.Addr,
		Handler: a,
	}

	log.Println("Listening on", server.Addr)
	server.ListenAndServe()
}
