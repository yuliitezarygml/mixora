package gorse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
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

func TestClientDeleteFeedbackUsesEscapedRoute(t *testing.T) {
	t.Parallel()
	feedbackType := "skip / special"
	userID := "user/a b?c"
	itemID := "soundcloud:42/track?part=1"
	expectedPath := "/api/feedback/" + url.PathEscape(feedbackType) + "/" + url.PathEscape(userID) + "/" + url.PathEscape(itemID)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.RequestURI() != expectedPath {
			t.Fatalf("request URI = %q, want %q", r.URL.RequestURI(), expectedPath)
		}
		if r.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("X-API-Key = %q", r.Header.Get("X-API-Key"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteFeedback(context.Background(), feedbackType, userID, itemID); err != nil {
		t.Fatal(err)
	}
}

func TestClientDeleteFeedbackRejectsBlankIdentifiers(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		feedbackType string
		userID       string
		itemID       string
		wantError    string
	}{
		{name: "feedback type", feedbackType: " \t", userID: "user", itemID: "item", wantError: "feedback type"},
		{name: "user", feedbackType: "skip", userID: "\n", itemID: "item", wantError: "user ID"},
		{name: "item", feedbackType: "skip", userID: "user", itemID: " ", wantError: "item ID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := client.DeleteFeedback(context.Background(), test.feedbackType, test.userID, test.itemID)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("DeleteFeedback() error = %v, want error containing %q", err, test.wantError)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}
