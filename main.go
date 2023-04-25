package main

import (
	"net/http"
)

func main() {

	server := http.Server{
		Addr: ":9955",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Hello, this is gopress.org!!"))
		}),
	}

	server.ListenAndServe()
}
