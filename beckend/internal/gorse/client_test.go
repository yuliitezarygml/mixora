package gorse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/iulian/soundcloud-go/internal/music"
)

func TestClientUsesAPIKeyAndGorseShapes(t *testing.T) {
	t.Parallel()
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("X-API-Key = %q", r.Header.Get("X-API-Key"))
		}
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		switch r.URL.Path {
		case "/api/user":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["UserId"] != "user/1" {
				t.Fatalf("user body = %#v", body)
			}
		case "/api/items":
			var body []map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 1 || body[0]["ItemId"] != "soundcloud:42" {
				t.Fatalf("items body = %#v", body)
			}
		case "/api/feedback":
			if r.Method != http.MethodPut {
				t.Fatalf("feedback method = %s", r.Method)
			}
		case "/api/recommend/user/1":
			_, _ = w.Write([]byte(`["soundcloud:42",{"Id":"soundcloud:7","Score":0.8}]`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"RowAffected":1}`))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.UpsertUser(ctx, "user/1"); err != nil {
		t.Fatal(err)
	}
	if err := client.UpsertItems(ctx, []music.Track{{ID: "42", Source: "soundcloud", Title: "Song", Artist: "Artist", Access: "playable"}}); err != nil {
		t.Fatal(err)
	}
	if err := client.PutFeedback(ctx, []Feedback{{FeedbackType: "like", UserID: "user/1", ItemID: "soundcloud:42", Value: 1}}); err != nil {
		t.Fatal(err)
	}
	got, err := client.Recommend(ctx, "user/1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"soundcloud:42", "soundcloud:7"}) {
		t.Fatalf("recommendations = %#v", got)
	}
	if len(paths) != 4 {
		t.Fatalf("requests = %#v", paths)
	}
}

func TestClientRejectsInvalidURL(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{BaseURL: "not-a-url"}); err == nil {
		t.Fatal("expected invalid URL error")
	}
}
