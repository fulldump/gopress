package googlefilestore

/*
import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

type GoogleFilestore struct {
	config GoogleCloudStorage
	client *storage.Client
}

type GoogleCloudStorage struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
	Bucket                  string `json:"bucket"`
}

func New(config GoogleCloudStorage) (*GoogleFilestore, error) {

	// Setup auth
	b, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("could not marshal config.GoogleCloudStorage: %s", err)
	}
	auth := option.WithCredentialsJSON(b)

	// Instantiate client
	ctx := context.Background()
	c, err := storage.NewClient(ctx, auth)
	if err != nil {
		return nil, err
	}

	// Return filestorager!!
	return &GoogleFilestore{
		config: config,
		client: c,
	}, nil
}

func (f *GoogleFilestore) OpenWriter(filename string) (io.WriteCloser, error) {

	ctx := context.Background()
	w := f.client.Bucket(f.config.Bucket).Object(filename).NewWriter(ctx)
	return w, nil
}

func (f *GoogleFilestore) OpenReader(filename string) (io.ReadCloser, error) {

	ctx := context.Background()
	return f.client.Bucket(f.config.Bucket).Object(filename).NewReader(ctx)
}
*/
