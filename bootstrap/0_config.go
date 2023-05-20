package bootstrap

import (
	"gopress/filestorage/googlefilestore"
	inceptiondbclient "gopress/inceptiondb"
)

type Config struct {
	Addr      string `usage:"Server http address"`
	Statics   string `usage:"Use a directory to serve statics or even a http server"`
	Inception inceptiondbclient.Config

	Standalone Standalone

	// Storage
	GoogleCloudStorage googlefilestore.GoogleCloudStorage
	LocalStorage       string `usage:"Images directory"`
	StorageType        string `usage:"Select storage backend: 'GoogleCloud' for GoogleCloudStorage, otherwise local storage"`

	Version bool `usage:"Show version and exit"`
}

type Standalone struct {
	Enabled   bool `usage:"Configure an embedded database and authentication layer. It omits Inception configuration."`
	Inception InceptionStandaloneConfig
}
