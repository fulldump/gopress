package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/fulldump/goconfig"

	"gopress/api"
	"gopress/filestorage"
	"gopress/filestorage/googlefilestore"
	"gopress/filestorage/localfilestore"
	"gopress/inceptiondb"
)

type Config struct {
	Addr      string `usage:"Server http address"`
	Statics   string `usage:"Use a directory to serve statics or even a http server"`
	Inception inceptiondb.Config

	// Storage
	GoogleCloudStorage googlefilestore.GoogleCloudStorage
	LocalStorage       string `usage:"Images directory"`
	StorageType        string `usage:"Select storage backend: 'GoogleCloud' for GoogleCloudStorage, otherwise local storage"`
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
		StorageType:  "local", // todo: remove for clarity?
		LocalStorage: "./storage/",
	}
	goconfig.Read(c)

	// Database
	db := inceptiondb.NewClient(c.Inception)

	// File storage
	var fs filestorage.Filestorager
	var err error

	if c.StorageType == "GoogleCloud" {
		fmt.Println("GoogleCloud")
		fs, err = googlefilestore.New(c.GoogleCloudStorage)
		if err != nil {
			fmt.Println("ERROR: ", err)
			os.Exit(-1)
		}
	} else {
		fmt.Println("Local: ", c.LocalStorage)
		fs, err = localfilestore.New(c.LocalStorage)
		if err != nil {
			fmt.Println("ERROR: can not initialize LocalStorage:", err)
			os.Exit(-1)
		}
	}

	a := api.NewApi(c.Statics, db, fs)

	server := http.Server{
		Addr:    c.Addr,
		Handler: a,
	}

	log.Println("Listening on", server.Addr)
	server.ListenAndServe()
}
