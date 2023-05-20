package bootstrap

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/fulldump/box"
	"github.com/fulldump/inceptiondb/api"
	"github.com/fulldump/inceptiondb/database"
	"github.com/fulldump/inceptiondb/service"
)

type InceptionStandaloneConfig struct {
	Dir  string `usage:"TODO"`
	Addr string `usage:"TODO"`
}

func InceptionDB(c InceptionStandaloneConfig) Runner {

	db := database.NewDatabase(&database.Config{
		Dir: c.Dir,
	})

	b := api.Build(service.NewService(db), "", "embedded")
	b.WithInterceptors(
		api.AccessLog(log.New(os.Stdout, "ACCESS: ", log.Lshortfile)),
		api.InterceptorUnavailable(db),
		api.RecoverFromPanic,
		api.PrettyErrorInterceptor,
	)

	s := &http.Server{
		Addr:    c.Addr,
		Handler: box.Box2Http(b),
	}

	return func() (start, stop func() error) {

		start = func() error {
			log.Println("InceptionDB listening on", s.Addr)

			err := db.Load()
			if err != nil {
				return err
			}

			err = s.ListenAndServe()
			if err == http.ErrServerClosed {
				err = nil
			}
			return err
		}

		stop = func() error {
			log.Println("stop inceptiondb")

			err := db.Stop()
			if err != nil {
				log.Println("db.Stop():", err.Error())
			}

			err = s.Shutdown(context.Background())
			if err != nil {
				log.Println("db server shutdown:", err.Error())
			}

			return nil
		}

		return
	}
}
