package spotify_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iulian/soundcloud-go/pkg/spotify"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		input    string
		wantType spotify.ResourceType
		wantID   string
		wantURI  string
		wantErr  bool
	}{
		{
			input:    "https://open.spotify.com/track/6rqhFgbbKwnb9MLmUQDhG6?si=abc123",
			wantType: spotify.TypeTrack,
			wantID:   "6rqhFgbbKwnb9MLmUQDhG6",
			wantURI:  "spotify:track:6rqhFgbbKwnb9MLmUQDhG6",
		},
		{
			input:    "spotify:track:6rqhFgbbKwnb9MLmUQDhG6",
			wantType: spotify.TypeTrack,
			wantID:   "6rqhFgbbKwnb9MLmUQDhG6",
			wantURI:  "spotify:track:6rqhFgbbKwnb9MLmUQDhG6",
		},
		{
			input:    "https://open.spotify.com/album/4m2880jivSbbyEGAKfITCa",
			wantType: spotify.TypeAlbum,
			wantID:   "4m2880jivSbbyEGAKfITCa",
			wantURI:  "spotify:album:4m2880jivSbbyEGAKfITCa",
		},
		{
			input:    "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
			wantType: spotify.TypePlaylist,
			wantID:   "37i9dQZF1DXcBWIGoYBM5M",
			wantURI:  "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
		},
		{
			input:    "https://open.spotify.com/artist/0k17h0D3J5VfsdmQ1iZtE9",
			wantType: spotify.TypeArtist,
			wantID:   "0k17h0D3J5VfsdmQ1iZtE9",
			wantURI:  "spotify:artist:0k17h0D3J5VfsdmQ1iZtE9",
		},
		{
			input:   "https://invalid.com/track/123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res, err := spotify.ParseURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", tt.input, err)
			}
			if res.Type != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, res.Type)
			}
			if res.ID != tt.wantID {
				t.Errorf("expected ID %s, got %s", tt.wantID, res.ID)
			}
			if res.URI != tt.wantURI {
				t.Errorf("expected URI %s, got %s", tt.wantURI, res.URI)
			}
		})
	}
}

func TestConnectClient(t *testing.T) {
	var lastPath string
	var lastQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		lastQuery = r.URL.RawQuery

		switch r.URL.Path {
		case "/status":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"username": "testuser",
				"device_id": "dev123",
				"device_name": "Go-Librespot-Speaker",
				"device_type": "SPEAKER",
				"stopped": false,
				"paused": false,
				"buffering": false,
				"volume": 32768,
				"volume_steps": 64,
				"track": {
					"uri": "spotify:track:6rqhFgbbKwnb9MLmUQDhG6",
					"name": "Speak To Me",
					"artist_names": ["Pink Floyd"],
					"album_name": "The Dark Side of the Moon",
					"duration": 65314,
					"position": 12000
				}
			}`))
		case "/player/load", "/player/play", "/player/pause", "/player/play-pause", "/player/next", "/player/prev", "/player/seek", "/player/volume":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := spotify.NewConnectClient(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. IsAvailable
	if !client.IsAvailable(ctx) {
		t.Fatal("expected connect client to be available")
	}

	// 2. Status
	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("unexpected error getting status: %v", err)
	}
	if status.DeviceName != "Go-Librespot-Speaker" {
		t.Errorf("expected device name 'Go-Librespot-Speaker', got %s", status.DeviceName)
	}
	if status.Track == nil || status.Track.Name != "Speak To Me" {
		t.Errorf("expected track 'Speak To Me', got %+v", status.Track)
	}

	// 3. Load
	if err := client.Load(ctx, "spotify:track:6rqhFgbbKwnb9MLmUQDhG6", true); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if lastPath != "/player/load" || lastQuery != "play=true&uri=spotify%3Atrack%3A6rqhFgbbKwnb9MLmUQDhG6" {
		t.Errorf("unexpected load request: path=%s, query=%s", lastPath, lastQuery)
	}

	// 4. Play, Pause, PlayPause, Next, Prev
	if err := client.Play(ctx); err != nil {
		t.Fatalf("play failed: %v", err)
	}
	if lastPath != "/player/play" {
		t.Errorf("expected /player/play, got %s", lastPath)
	}

	if err := client.Pause(ctx); err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	if lastPath != "/player/pause" {
		t.Errorf("expected /player/pause, got %s", lastPath)
	}

	if err := client.Next(ctx); err != nil {
		t.Fatalf("next failed: %v", err)
	}
	if lastPath != "/player/next" {
		t.Errorf("expected /player/next, got %s", lastPath)
	}

	if err := client.Prev(ctx); err != nil {
		t.Fatalf("prev failed: %v", err)
	}
	if lastPath != "/player/prev" {
		t.Errorf("expected /player/prev, got %s", lastPath)
	}

	// 5. Volume
	if err := client.SetVolume(ctx, 50); err != nil {
		t.Fatalf("volume failed: %v", err)
	}
	if lastPath != "/player/volume" || lastQuery != "volume=32767" {
		t.Errorf("unexpected volume request: path=%s, query=%s", lastPath, lastQuery)
	}

	// 6. Seek
	if err := client.Seek(ctx, 15000); err != nil {
		t.Fatalf("seek failed: %v", err)
	}
	if lastPath != "/player/seek" || lastQuery != "position=15000" {
		t.Errorf("unexpected seek request: path=%s, query=%s", lastPath, lastQuery)
	}
}
