package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var request embedRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "embeddinggemma" || request.Dimensions != 768 || len(request.Input) != 2 {
			t.Fatalf("unexpected payload: %+v", request)
		}
		vectors := make([][]float32, 2)
		for index := range vectors {
			vectors[index] = make([]float32, 768)
			vectors[index][index] = 1
		}
		_ = json.NewEncoder(w).Encode(embedResponse{Embeddings: vectors})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL, Model: "embeddinggemma", Dimensions: 768, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := client.Embed(context.Background(), []string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || vectors[1][1] != 1 {
		t.Fatalf("unexpected vectors: %d", len(vectors))
	}
}

func TestClientRejectsWrongDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(embedResponse{Embeddings: [][]float32{{1, 2, 3}}})
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL, Model: "embeddinggemma", Dimensions: 768, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Embed(context.Background(), []string{"one"}); err == nil {
		t.Fatal("expected dimension validation error")
	}
}
