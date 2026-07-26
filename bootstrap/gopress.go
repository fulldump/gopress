package bootstrap

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"

	"gopress/api"
	"gopress/filestorage"
	"gopress/filestorage/localfilestore"
	inceptiondbclient "gopress/inceptiondb"
	"gopress/lib/deepseek"
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

	var moderator api.ContentModerator
	if c.DeepSeek.APIKey != "" {
		moderator, err = deepseek.NewClient(deepseek.Config{
			APIKey:  c.DeepSeek.APIKey,
			BaseURL: c.DeepSeek.BaseURL,
			Model:   c.DeepSeek.Model,
		})
		if err != nil {
			fmt.Println("ERROR: can not initialize DeepSeek client:", err)
			os.Exit(-1)
		}
	} else {
		log.Println("DeepSeek disabled: DEEPSEEK_APIKEY is not configured; automatic tags and translations are unavailable")
	}

	a := api.NewApi(c.Statics, version, db, fs, moderator)

	server := http.Server{
		Addr:    c.Addr,
		Handler: a,
	}

	return func() (start, stop func() error) {
		reconcilerStop := make(chan struct{})
		reconcilerDone := make(chan struct{})
		reconcilerStarted := make(chan struct{})
		var startReconciler sync.Once
		var stopReconciler sync.Once
		startWorker := func() {
			startReconciler.Do(func() {
				close(reconcilerStarted)
				go func() {
					defer close(reconcilerDone)
					api.RunBlogBanReconciler(db, reconcilerStop)
				}()
			})
		}
		stopWorker := func(wait bool) {
			stopReconciler.Do(func() { close(reconcilerStop) })
			select {
			case <-reconcilerStarted:
				if wait {
					<-reconcilerDone
				}
			default:
			}
		}

		start = func() error {
			startWorker()
			log.Println("Gopress listening on", server.Addr)
			startErr := server.ListenAndServe()
			stopWorker(true)
			if startErr == http.ErrServerClosed {
				startErr = nil
			}
			return startErr
		}

		stop = func() error {
			log.Println("Stop gopress")
			stopWorker(false)
			stopErr := server.Shutdown(context.Background())
			if stopErr != nil {
				log.Println("db server shutdown:", stopErr.Error())
			}
			stopWorker(true)

			return stopErr
		}

		return
	}
}
