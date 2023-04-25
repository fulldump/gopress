package main

import (
	"fmt"
	"net/http"

	"gopress/api"
)

func main() {

	a := api.NewApi()

	server := http.Server{
		Addr:    ":9955",
		Handler: a,
	}

	fmt.Println("Listening on", server.Addr)
	server.ListenAndServe()
}
