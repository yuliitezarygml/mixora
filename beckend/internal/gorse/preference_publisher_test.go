package gorse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iulian/soundcloud-go/internal/library"
	"github.com/iulian/soundcloud-go/internal/music"
)

func TestPreferencePublisherTranslatesDesiredStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		preference library.Preference
		feedback   string
		want       []string
	}{
		{name: "liked", preference: library.PreferenceLiked, feedback: "like", want: []string{"POST /api/items", "POST /api/user", "DELETE /api/feedback/dislike/listener/soundcloud:42", "PUT /api/feedback"}},
		{name: "disliked", preference: library.PreferenceDisliked, feedback: "dislike", want: []string{"POST /api/items", "POST /api/user", "DELETE /api/feedback/like/listener/soundcloud:42", "PUT /api/feedback"}},
		{name: "neutral", preference: library.PreferenceNeutral, want: []string{"POST /api/items", "POST /api/user", "DELETE /api/feedback/like/listener/soundcloud:42", "DELETE /api/feedback/dislike/listener/soundcloud:42"}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				if r.URL.Path == "/api/items" {
					var items []item
					if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
						t.Fatalf("decode items: %v", err)
					}
					if len(items) != 1 || !strings.Contains(items[0].Comment, "Verified track") || strings.Contains(items[0].Comment, `"title":"Track"`) {
						t.Fatalf("catalog item = %#v", items)
					}
				}
				if r.Method == http.MethodPut {
					var feedback []Feedback
					if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
						t.Fatalf("decode feedback: %v", err)
					}
					if len(feedback) != 1 || feedback[0].FeedbackType != test.feedback {
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
			verifier := &preferenceTrackVerifier{tracks: []music.Track{{
				Source: "soundcloud", ID: "42", Title: "Verified track", Artist: "Verified artist",
			}}}
			publisher := NewPreferencePublisher(client, verifier)
			err = publisher.PublishTrackPreference(context.Background(), library.TrackPreferencePublication{
				UserID: "listener",
				Preference: library.TrackPreference{
					Track:      library.TrackSnapshot{Source: "soundcloud", ID: "42", Title: "Track", Artist: "Artist"},
					Preference: test.preference,
					UpdatedAt:  time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC),
				},
			})
			if err != nil {
				t.Fatalf("PublishTrackPreference() error = %v", err)
			}
			if len(calls) != len(test.want) {
				t.Fatalf("calls = %#v, want %#v", calls, test.want)
			}
			for index := range test.want {
				if calls[index] != test.want[index] {
					t.Fatalf("calls[%d] = %q, want %q (all %#v)", index, calls[index], test.want[index], calls)
				}
			}
			if len(verifier.keys) != 1 || len(verifier.keys[0]) != 1 || verifier.keys[0][0] != "soundcloud:42" {
				t.Fatalf("catalog lookup = %#v", verifier.keys)
			}
		})
	}
}

func TestPreferencePublisherDefersUnverifiedClientSnapshotForCatalogReconciliation(t *testing.T) {
	t.Parallel()

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	verifier := &preferenceTrackVerifier{}
	publisher := NewPreferencePublisher(client, verifier)
	err = publisher.PublishTrackPreference(context.Background(), library.TrackPreferencePublication{
		UserID: "listener",
		Preference: library.TrackPreference{
			Track:      library.TrackSnapshot{Source: "spotify", ID: "forged-id", Title: "Forged title", Artist: "Forged artist"},
			Preference: library.PreferenceLiked,
			UpdatedAt:  time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC),
		},
	})
	if !errors.Is(err, library.ErrPreferenceTrackUnverified) {
		t.Fatalf("PublishTrackPreference() error = %v, want unverified catalog error", err)
	}
	if len(calls) != 0 {
		t.Fatalf("unverified snapshot called Gorse: %#v", calls)
	}
	if len(verifier.keys) != 1 || verifier.keys[0][0] != "spotify:forged-id" {
		t.Fatalf("catalog lookup = %#v", verifier.keys)
	}
}

func TestPreferencePublisherRemovesLegacyFeedbackForUnverifiedNeutralSnapshot(t *testing.T) {
	t.Parallel()

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	publisher := NewPreferencePublisher(client, &preferenceTrackVerifier{})
	err = publisher.PublishTrackPreference(context.Background(), library.TrackPreferencePublication{
		UserID: "listener",
		Preference: library.TrackPreference{
			Track:      library.TrackSnapshot{Source: "spotify", ID: "legacy-id"},
			Preference: library.PreferenceNeutral,
		},
	})
	if err != nil {
		t.Fatalf("PublishTrackPreference() error = %v", err)
	}
	want := []string{
		"DELETE /api/feedback/like/listener/spotify:legacy-id",
		"DELETE /api/feedback/dislike/listener/spotify:legacy-id",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("calls[%d] = %q, want %q (all %#v)", index, calls[index], want[index], calls)
		}
	}
}

func TestPreferencePublisherRetriesWhenCatalogVerificationIsUnavailable(t *testing.T) {
	t.Parallel()

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	verificationErr := errors.New("catalog temporarily unavailable")
	verifier := &preferenceTrackVerifier{err: verificationErr}
	publisher := NewPreferencePublisher(client, verifier)
	err = publisher.PublishTrackPreference(context.Background(), library.TrackPreferencePublication{
		UserID: "listener",
		Preference: library.TrackPreference{
			Track:      library.TrackSnapshot{Source: "soundcloud", ID: "42"},
			Preference: library.PreferenceLiked,
		},
	})
	if !errors.Is(err, verificationErr) {
		t.Fatalf("PublishTrackPreference() error = %v, want catalog verification error", err)
	}
	if len(calls) != 0 {
		t.Fatalf("catalog verification failure called Gorse: %#v", calls)
	}
}

type preferenceTrackVerifier struct {
	tracks []music.Track
	err    error
	keys   [][]string
}

func (v *preferenceTrackVerifier) Find(_ context.Context, keys []string) ([]music.Track, error) {
	v.keys = append(v.keys, append([]string(nil), keys...))
	return append([]music.Track(nil), v.tracks...), v.err
}
