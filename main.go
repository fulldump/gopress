package main

import (
	"github.com/fulldump/goconfig"

	"gopress/bootstrap"
	inceptiondbclient "gopress/inceptiondb"
)

func main() {

	c := &bootstrap.Config{
		Addr: "127.0.0.1:9955",
		Inception: inceptiondbclient.Config{
			Base:       "https://saas.inceptiondb.io/v1",
			DatabaseID: "fc84637c-6c31-400d-a312-7ec6de39fd2d",
			ApiKey:     "b76f9c1a-93fa-4988-84ee-cd761cee66f3",
			ApiSecret:  "52392c50-a2dc-4902-a492-062b210e5839",
		},
		StorageType:  "local", // todo: remove for clarity?
		LocalStorage: "./storage/",
		Standalone: bootstrap.Standalone{
			Enabled: false,
			Inception: bootstrap.InceptionStandaloneConfig{
				Addr: "127.0.0.1:9090",
				Dir:  "./database/",
			},
		},
	}
	goconfig.Read(c)

	runners := []bootstrap.Runner{}

	if c.Standalone.Enabled {
		runners = append(runners, bootstrap.InceptionDB(c.Standalone.Inception))
		c.Inception.Base = "http://" + c.Standalone.Inception.Addr + "/v1"
		c.Inception.DatabaseID = ""
	}

	runners = append(runners, bootstrap.Gopress(c))

	bootstrap.Run(runners...)
}
