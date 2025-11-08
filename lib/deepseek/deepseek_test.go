package deepseek

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseAnswer(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		{"exact true", "true", true, false},
		{"exact false", "false", false, false},
		{"contains true", "The answer is TRUE.", true, false},
		{"contains false", "I think it is false", false, false},
		{"both true and false", "true but also false", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAnswer(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected result: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestClientEvaluate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"true"}}]}`))
	}))
	defer srv.Close()

	client, err := NewClient(Config{APIKey: "secret", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	got, err := client.Evaluate(context.Background(), "hola mundo")
	if err != nil {
		t.Fatalf("evaluate returned error: %v", err)
	}
	if !got {
		t.Fatalf("expected true, got %v", got)
	}
}

func TestClientGenerateTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"tecnología, inteligencia artificial, español"}}]}`))
	}))
	defer srv.Close()

	client, err := NewClient(Config{APIKey: "secret", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	got, err := client.GenerateTags(context.Background(), "contenido de prueba")
	if err != nil {
		t.Fatalf("generate tags returned error: %v", err)
	}

	want := []string{"tecnología", "inteligencia artificial", "español"}
	if len(got) != len(want) {
		t.Fatalf("unexpected tags length: want %d, got %d", len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("unexpected tag at %d: want %q, got %q", i, want[i], got[i])
		}
	}
}
