package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iulian/soundcloud-go/internal/library"
)

func TestListenerJourneyPersistsAcrossReopen(t *testing.T) {
	fixture := newContractFixture()
	server := httptest.NewServer(fixture.handler())
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := server.Client()
	client.Jar = jar

	response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{
		"email": "journey@example.com", "password": "long-enough-password", "display_name": "Journey",
	})
	assertStatus(t, response, http.StatusCreated)
	closeResponse(t, response)

	response = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]any{
		"email": fixture.user.Email, "password": "long-enough-password",
	})
	assertStatus(t, response, http.StatusOK)
	closeResponse(t, response)

	tracks := []any{
		map[string]any{"source": "soundcloud", "id": "soundcloud:tracks:42", "title": "SoundCloud", "artist": "Artist", "permalink": "https://soundcloud.com/artist/track"},
		map[string]any{"source": "youtube", "id": "youtube-1", "title": "YouTube", "artist": "Artist", "permalink": "https://www.youtube.com/watch?v=youtube-1"},
		map[string]any{"source": "vk", "id": "vk-1", "title": "VK", "artist": "Artist", "permalink": "https://vkvideo.ru/video1_2"},
		map[string]any{"source": "bandcamp", "id": "bandcamp-1", "title": "Bandcamp", "artist": "Artist", "permalink": "https://artist.bandcamp.com/track/example"},
		map[string]any{"source": "spotify", "id": "spotify-1", "title": "Spotify", "artist": "Artist", "access": "preview", "permalink": "https://open.spotify.com/track/spotify-1"},
	}

	response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/track-preferences", map[string]any{
		"idempotency_key": "journey-like", "preference": "liked", "track": tracks[1],
	})
	assertStatus(t, response, http.StatusOK)
	closeResponse(t, response)

	playlistID := "123e4567-e89b-12d3-a456-426614174100"
	response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/playlists/"+playlistID, map[string]any{
		"idempotency_key": "journey-playlist", "expected_revision": 0,
		"name": "Journey mix", "pinned": true, "tracks": tracks,
	})
	assertStatus(t, response, http.StatusOK)
	closeResponse(t, response)

	response = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/events", map[string]any{"events": []any{
		map[string]any{"idempotency_key": "journey-play", "type": "play", "track_source": "youtube", "track_id": "youtube-1"},
		map[string]any{"idempotency_key": "journey-next", "type": "skip", "track_source": "youtube", "track_id": "youtube-1"},
		map[string]any{"idempotency_key": "journey-add", "type": "add_to_playlist", "track_source": "vk", "track_id": "vk-1", "context": map[string]any{"playlist_id": playlistID}},
	}})
	assertStatus(t, response, http.StatusNoContent)
	closeResponse(t, response)

	playbackState, err := json.Marshal(map[string]any{
		"type": "state", "playing": true, "position": 17.3,
		"track": tracks[1], "queue": tracks,
	})
	if err != nil {
		t.Fatalf("json.Marshal(playback state) error = %v", err)
	}
	websocketURL := "ws" + server.URL[len("http"):] + "/api/v1/playback/ws"
	header := http.Header{}
	header.Set("Origin", server.URL)
	header.Set("Cookie", sessionCookie+"="+fixture.sessionToken)
	connection, _, err := websocket.DefaultDialer.Dial(websocketURL, header)
	if err != nil {
		t.Fatalf("playback websocket dial error = %v", err)
	}
	if err := connection.WriteMessage(websocket.TextMessage, playbackState); err != nil {
		_ = connection.Close()
		t.Fatalf("playback websocket write error = %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if stored, ok := fixture.playback.LastState(fixture.user.ID); ok && bytes.Equal(stored, playbackState) {
			break
		}
		if time.Now().After(deadline) {
			_ = connection.Close()
			t.Fatal("playback state was not stored before reopen")
		}
		time.Sleep(time.Millisecond)
	}
	_ = connection.Close()

	baseURL, err := url.Parse(server.URL + "/api/v1")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	reopenedJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New(reopened) error = %v", err)
	}
	reopenedJar.SetCookies(baseURL, jar.Cookies(baseURL))
	reopenedClient := server.Client()
	reopenedClient.Jar = reopenedJar

	response = request(t, reopenedClient, http.MethodGet, server.URL+"/api/v1/auth/session", nil)
	assertStatus(t, response, http.StatusOK)
	closeResponse(t, response)

	response = request(t, reopenedClient, http.MethodGet, server.URL+"/api/v1/me/track-preferences", nil)
	assertStatus(t, response, http.StatusOK)
	var preferences struct {
		Preferences []library.TrackPreference `json:"preferences"`
	}
	decodeResponse(t, response, &preferences)
	if len(preferences.Preferences) != 1 || preferences.Preferences[0].Preference != library.PreferenceLiked || preferences.Preferences[0].Track.Source != "youtube" {
		t.Fatalf("reopened preferences = %#v", preferences.Preferences)
	}

	response = request(t, reopenedClient, http.MethodGet, server.URL+"/api/v1/me/playlists", nil)
	assertStatus(t, response, http.StatusOK)
	var playlists struct {
		Playlists []library.Playlist `json:"playlists"`
	}
	decodeResponse(t, response, &playlists)
	if len(playlists.Playlists) != 1 || len(playlists.Playlists[0].Tracks) != len(tracks) {
		t.Fatalf("reopened playlists = %#v", playlists.Playlists)
	}
	wantSources := []string{"soundcloud", "youtube", "vk", "bandcamp", "spotify"}
	for index, want := range wantSources {
		if got := playlists.Playlists[0].Tracks[index].Source; got != want {
			t.Fatalf("reopened playlist source[%d] = %q, want %q", index, got, want)
		}
	}
	if playlists.Playlists[0].Tracks[0].ID != "42" {
		t.Fatalf("canonical SoundCloud id = %q, want 42", playlists.Playlists[0].Tracks[0].ID)
	}

	reopenedSocket, _, err := websocket.DefaultDialer.Dial(websocketURL, header)
	if err != nil {
		t.Fatalf("reopened playback websocket dial error = %v", err)
	}
	t.Cleanup(func() { _ = reopenedSocket.Close() })
	if err := reopenedSocket.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	messageType, replayed, err := reopenedSocket.ReadMessage()
	if err != nil {
		t.Fatalf("reopened playback read error = %v", err)
	}
	if messageType != websocket.TextMessage || !bytes.Equal(replayed, playbackState) {
		t.Fatalf("reopened playback state = type %d, %s; want %s", messageType, replayed, playbackState)
	}

	if len(fixture.events.batches) != 1 || len(fixture.events.batches[0]) != 3 {
		t.Fatalf("journey events = %#v", fixture.events.batches)
	}
}
