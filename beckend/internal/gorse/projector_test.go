package gorse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iulian/soundcloud-go/internal/events"
	"github.com/iulian/soundcloud-go/internal/music"
)

func TestProjectorPublishesOnlyCatalogVerifiedEvents(t *testing.T) {
	t.Parallel()

	var lock sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		lock.Unlock()
		if r.URL.Path == "/api/items" {
			var items []item
			if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
				t.Fatalf("decode items: %v", err)
			}
			if len(items) != 1 || !strings.Contains(items[0].Comment, "Verified event track") || strings.Contains(items[0].Comment, "Forged event title") {
				t.Fatalf("catalog items = %#v", items)
			}
		}
		if r.URL.Path == "/api/feedback" {
			var feedback []Feedback
			if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
				t.Fatalf("decode feedback: %v", err)
			}
			if len(feedback) != 1 || feedback[0].ItemID != "soundcloud:42" || feedback[0].FeedbackType != "play" {
				t.Fatalf("feedback = %#v", feedback)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	verifier := &projectorTrackVerifier{tracks: []music.Track{{
		Source: "soundcloud", ID: "42", Title: "Verified event track", Artist: "Verified artist",
	}}}
	projector := NewProjector(client, verifier)
	err = projector.Project(context.Background(), []events.FeedbackAggregate{{
		UserID: "listener", Type: "play", Source: "soundcloud", TrackID: "soundcloud:tracks:42", Count: 2,
		LatestAt: time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC),
	}, {
		UserID: "listener", Type: "play", Source: "spotify", TrackID: "forged", Count: 1,
	}})
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	lock.Lock()
	gotCalls := append([]string(nil), calls...)
	lock.Unlock()
	wantCalls := []string{"POST /api/items", "POST /api/user", "PUT /api/feedback"}
	if len(gotCalls) != len(wantCalls) {
		t.Fatalf("calls = %#v, want %#v", gotCalls, wantCalls)
	}
	for index := range wantCalls {
		if gotCalls[index] != wantCalls[index] {
			t.Fatalf("calls[%d] = %q, want %q", index, gotCalls[index], wantCalls[index])
		}
	}
	if len(verifier.keys) != 1 || len(verifier.keys[0]) != 2 || verifier.keys[0][0] != "soundcloud:42" || verifier.keys[0][1] != "spotify:forged" {
		t.Fatalf("catalog lookup = %#v", verifier.keys)
	}
}

func TestProjectorSkipsUnverifiedEventWithoutCallingGorse(t *testing.T) {
	t.Parallel()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	verifier := &projectorTrackVerifier{}
	projector := NewProjector(client, verifier)
	err = projector.Project(context.Background(), []events.FeedbackAggregate{{
		UserID: "listener", Type: "play", Source: "spotify", TrackID: "forged", Count: 1,
	}})
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("unverified event called Gorse %d times", calls)
	}
}

func TestProjectorRetriesWhenCatalogVerificationFails(t *testing.T) {
	t.Parallel()

	verificationErr := errors.New("catalog temporarily unavailable")
	client, err := New(Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	projector := NewProjector(client, &projectorTrackVerifier{err: verificationErr})
	err = projector.Project(context.Background(), []events.FeedbackAggregate{{
		UserID: "listener", Type: "play", Source: "soundcloud", TrackID: "42", Count: 1,
	}})
	if !errors.Is(err, verificationErr) {
		t.Fatalf("Project() error = %v, want catalog verification error", err)
	}
}

type projectorTrackVerifier struct {
	tracks []music.Track
	err    error
	keys   [][]string
}

func (v *projectorTrackVerifier) Find(_ context.Context, keys []string) ([]music.Track, error) {
	v.keys = append(v.keys, append([]string(nil), keys...))
	return append([]music.Track(nil), v.tracks...), v.err
}
