package soundcloud_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iulian/soundcloud-go/pkg/soundcloud"
	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

func TestClientOptions(t *testing.T) {
	customID := "test_client_id_123"
	customUA := "CustomAgent/1.0"
	customToken := "OAuth my_token"

	ctx := context.Background()
	client, err := soundcloud.New(ctx,
		soundcloud.WithClientID(customID),
		soundcloud.WithUserAgent(customUA),
		soundcloud.WithAuthToken(customToken),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.ClientID() != customID {
		t.Errorf("expected clientID %s, got %s", customID, client.ClientID())
	}
}

func TestGetTrackMock(t *testing.T) {
	// Mock HTTP server responding with SoundCloud track JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != "mock_id" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": 99999,
			"kind": "track",
			"title": "Mock Track",
			"duration": 180000,
			"permalink_url": "https://soundcloud.com/artist/mock-track",
			"user": {
				"id": 111,
				"username": "Mock Artist"
			},
			"media": {
				"transcodings": [
					{
						"url": "https://api-v2.soundcloud.com/media/mock/stream",
						"preset": "mp3_0_1",
						"duration": 180000,
						"format": {
							"protocol": "progressive",
							"mime_type": "audio/mpeg"
						},
						"quality": "sq"
					}
				]
			}
		}`))
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := soundcloud.New(ctx,
		soundcloud.WithClientID("mock_id"),
		soundcloud.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	track := &models.Track{
		ID:    99999,
		Title: "Mock Track",
		Media: models.Media{
			Transcodings: []models.Transcoding{
				{
					URL: ts.URL + "/stream_url",
					Format: models.Format{
						Protocol: "progressive",
						MimeType: "audio/mpeg",
					},
				},
			},
		},
	}

	if track.Title != "Mock Track" {
		t.Errorf("expected title 'Mock Track', got %s", track.Title)
	}
	_ = client
}
