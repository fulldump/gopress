package inceptiondb

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateCollectionRecognizesHostedAlreadyExistsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"collection 'articles' already exists"}}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(Config{Base: server.URL})
	if err := client.CreateCollection("articles"); err != ErrorAlreadyExist {
		t.Fatalf("expected ErrorAlreadyExist, got %v", err)
	}
}
