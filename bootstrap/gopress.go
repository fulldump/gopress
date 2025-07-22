package bootstrap

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"gopress/api"
	"gopress/filestorage"
	"gopress/filestorage/localfilestore"
	inceptiondbclient "gopress/inceptiondb"
	"gopress/templates"
)

func Gopress(c *Config, version string) Runner {
	// Database
	db := inceptiondbclient.NewClient(c.Inception)

	// Templates
	templates.GlobalData["headTrailCode"] = template.HTML(c.HeadTrailCode)

	// File storage
	var fs filestorage.Filestorager
	var err error

	if c.StorageType == "GoogleCloud" {
		fmt.Println("GoogleCloud")
		fmt.Println("ERROR: GoogleCloud Library disabled")
		os.Exit(-1)
		// fs, err = googlefilestore.New(c.GoogleCloudStorage)
		// if err != nil {
		// 	fmt.Println("ERROR: ", err)
		// 	os.Exit(-1)
		// }
	} else {
		fmt.Println("Local storage: ", c.LocalStorage)
		fs, err = localfilestore.New(c.LocalStorage)
		if err != nil {
			fmt.Println("ERROR: can not initialize LocalStorage:", err)
			os.Exit(-1)
		}
	}

	a := api.NewApi(c.Statics, version, db, fs)

	server := http.Server{
		Addr:    c.Addr,
		Handler: a,
	}

	return func() (start, stop func() error) {

		start = func() error {
			log.Println("Gopress listening on", server.Addr)
			err = server.ListenAndServe()
			if err == http.ErrServerClosed {
				err = nil
			}
			return err
		}

		stop = func() error {
			log.Println("Stop gopress")
			err = server.Shutdown(context.Background())
			if err != nil {
				log.Println("db server shutdown:", err.Error())
			}

			return err
		}

		return
	}
}
