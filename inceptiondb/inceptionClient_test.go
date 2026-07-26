package inceptiondb

import (
	"errors"
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

func TestCollectionOperationsRecognizeNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := NewClient(Config{Base: server.URL})
	if _, err := client.GetCollection("missing"); !errors.Is(err, ErrorNotFound) {
		t.Fatalf("get collection: expected ErrorNotFound, got %v", err)
	}
	if err := client.DropCollection("missing"); !errors.Is(err, ErrorNotFound) {
		t.Fatalf("drop collection: expected ErrorNotFound, got %v", err)
	}
}
