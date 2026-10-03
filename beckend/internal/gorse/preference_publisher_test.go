package gorse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iulian/soundcloud-go/internal/library"
)

func TestPreferencePublisherTranslatesDesiredStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		preference library.Preference
		feedback   string
		want       []string
	}{
		{name: "liked", preference: library.PreferenceLiked, feedback: "like", want: []string{"POST /api/user", "DELETE /api/feedback/dislike/listener/soundcloud:42", "PUT /api/feedback"}},
		{name: "disliked", preference: library.PreferenceDisliked, feedback: "dislike", want: []string{"POST /api/user", "DELETE /api/feedback/like/listener/soundcloud:42", "PUT /api/feedback"}},
		{name: "neutral", preference: library.PreferenceNeutral, want: []string{"POST /api/user", "DELETE /api/feedback/like/listener/soundcloud:42", "DELETE /api/feedback/dislike/listener/soundcloud:42"}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
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
			publisher := NewPreferencePublisher(client)
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
		})
	}
}
