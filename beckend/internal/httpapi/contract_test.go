package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iulian/soundcloud-go/internal/auth"
	"github.com/iulian/soundcloud-go/internal/events"
	"github.com/iulian/soundcloud-go/internal/library"
	"github.com/iulian/soundcloud-go/internal/mail"
	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/iulian/soundcloud-go/internal/playback"
	"github.com/iulian/soundcloud-go/internal/recommendation"
)

func TestAppAPIContract(t *testing.T) {
	fixture := newContractFixture()
	server := httptest.NewServer(fixture.handler())
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := server.Client()
	client.Jar = jar

	t.Run("health", func(t *testing.T) {
		response := request(t, client, http.MethodGet, server.URL+"/health", nil)
		assertStatus(t, response, http.StatusOK)
		assertMiddlewareHeaders(t, response)
		var body map[string]string
		decodeResponse(t, response, &body)
		if body["status"] != "ok" || body["service"] != "mixora-api" {
			t.Fatalf("health response = %#v", body)
		}
	})

	t.Run("protected route requires session", func(t *testing.T) {
		response := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/library", nil)
		assertStatus(t, response, http.StatusUnauthorized)
		var body errorResponse
		decodeResponse(t, response, &body)
		if body.Error.Code != "authentication_required" {
			t.Fatalf("error code = %q, want authentication_required", body.Error.Code)
		}
		if body.Error.RequestID == "" || body.Error.RequestID != response.Header.Get("X-Request-ID") {
			t.Fatalf("error request id = %q, header = %q", body.Error.RequestID, response.Header.Get("X-Request-ID"))
		}
	})

	t.Run("register", func(t *testing.T) {
		response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{
			"email": "listener@example.com", "password": "long-enough-password", "display_name": "Listener",
		})
		assertStatus(t, response, http.StatusCreated)
		var got auth.User
		decodeResponse(t, response, &got)
		if got.ID != fixture.user.ID {
			t.Fatalf("registered user id = %q, want %q", got.ID, fixture.user.ID)
		}
		if fixture.auth.registeredEmail != "listener@example.com" || fixture.auth.registeredName != "Listener" {
			t.Fatalf("registration input = %q/%q", fixture.auth.registeredEmail, fixture.auth.registeredName)
		}
		if fixture.outbox.message.Kind != "verify_email" || fixture.outbox.message.To != fixture.user.Email {
			t.Fatalf("verification message = %#v", fixture.outbox.message)
		}
		if !strings.Contains(fixture.outbox.message.TextBody, "verification-token") {
			t.Fatal("verification email does not contain the service token")
		}
	})

	t.Run("login and session", func(t *testing.T) {
		requestBody := map[string]string{"email": fixture.user.Email, "password": "long-enough-password"}
		req := newJSONRequest(t, http.MethodPost, server.URL+"/api/v1/auth/login", requestBody)
		req.Header.Set("X-Mixora-Device-ID", "contract-device")
		req.Header.Set("User-Agent", "mixora-contract-test")
		response, err := client.Do(req)
		if err != nil {
			t.Fatalf("login request error = %v", err)
		}
		assertStatus(t, response, http.StatusOK)
		var got auth.User
		decodeResponse(t, response, &got)
		if got.ID != fixture.user.ID {
			t.Fatalf("login user id = %q, want %q", got.ID, fixture.user.ID)
		}
		if fixture.auth.loginMetadata.DeviceID != "contract-device" || fixture.auth.loginMetadata.UserAgent != "mixora-contract-test" {
			t.Fatalf("login metadata = %#v", fixture.auth.loginMetadata)
		}

		cookies := response.Cookies()
		if len(cookies) != 1 {
			t.Fatalf("login cookies = %d, want 1", len(cookies))
		}
		cookie := cookies[0]
		if cookie.Name != sessionCookie || cookie.Value != fixture.sessionToken || cookie.Path != "/api/v1" || !cookie.HttpOnly || cookie.MaxAge <= 0 || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("session cookie = %#v", cookie)
		}

		response = request(t, client, http.MethodGet, server.URL+"/api/v1/auth/session", nil)
		assertStatus(t, response, http.StatusOK)
		decodeResponse(t, response, &got)
		if got.ID != fixture.user.ID {
			t.Fatalf("session user id = %q, want %q", got.ID, fixture.user.ID)
		}
	})

	t.Run("library get and put", func(t *testing.T) {
		response := request(t, client, http.MethodGet, server.URL+"/api/v1/library", nil)
		assertStatus(t, response, http.StatusOK)
		if got := response.Header.Get("ETag"); got != `"7"` {
			t.Fatalf("GET library ETag = %q, want %q", got, `"7"`)
		}
		var got map[string]any
		decodeResponse(t, response, &got)
		if len(got["likes"].([]any)) != 1 {
			t.Fatalf("GET library body = %#v", got)
		}

		payload := json.RawMessage(`{"likes":[{"id":"new"}],"history":[]}`)
		response = request(t, client, http.MethodPut, server.URL+"/api/v1/library", bytes.NewReader(payload))
		assertStatus(t, response, http.StatusOK)
		if got := response.Header.Get("ETag"); got != `"8"` {
			t.Fatalf("PUT library ETag = %q, want %q", got, `"8"`)
		}
		var saved map[string]any
		decodeResponse(t, response, &saved)
		if fixture.library.userID != fixture.user.ID || !jsonEqual(fixture.library.payload, payload) {
			t.Fatalf("saved library user/payload = %q/%s", fixture.library.userID, fixture.library.payload)
		}
	})

	t.Run("track preferences are server-owned desired state", func(t *testing.T) {
		input := map[string]any{
			"idempotency_key": "preference-1",
			"preference":      "liked",
			"track": map[string]any{
				"source": "soundcloud", "id": "soundcloud:tracks:42",
				"title": "Fixture track", "artist": "Fixture artist",
			},
		}
		response := requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/track-preferences", input)
		assertStatus(t, response, http.StatusOK)
		var saved library.TrackPreference
		decodeResponse(t, response, &saved)
		if saved.Preference != library.PreferenceLiked || saved.Track.Source != "soundcloud" || saved.Track.ID != "42" || saved.Revision != 1 {
			t.Fatalf("saved preference = %#v", saved)
		}

		response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/track-preferences", input)
		assertStatus(t, response, http.StatusOK)
		var replay library.TrackPreference
		decodeResponse(t, response, &replay)
		if replay.Revision != saved.Revision || replay.Preference != saved.Preference {
			t.Fatalf("replayed preference = %#v, want %#v", replay, saved)
		}

		response = request(t, client, http.MethodGet, server.URL+"/api/v1/me/track-preferences", nil)
		assertStatus(t, response, http.StatusOK)
		var listed struct {
			Preferences []library.TrackPreference `json:"preferences"`
		}
		decodeResponse(t, response, &listed)
		if len(listed.Preferences) != 1 || listed.Preferences[0].Track.ID != "42" {
			t.Fatalf("listed preferences = %#v", listed.Preferences)
		}

		conflict := map[string]any{
			"idempotency_key": "preference-1", "preference": "disliked",
			"track": input["track"],
		}
		response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/track-preferences", conflict)
		assertStatus(t, response, http.StatusConflict)
		var body errorResponse
		decodeResponse(t, response, &body)
		if body.Error.Code != "idempotency_conflict" {
			t.Fatalf("error code = %q, want idempotency_conflict", body.Error.Code)
		}
	})

	t.Run("history is normalized, ordered, and idempotent", func(t *testing.T) {
		input := map[string]any{
			"idempotency_key": "history-1",
			"occurred_at":     "2026-10-03T12:00:00Z",
			"track": map[string]any{
				"source": "soundcloud", "id": "soundcloud:tracks:99",
				"title": "History track", "artist": "History artist",
			},
		}
		response := requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/history", input)
		assertStatus(t, response, http.StatusOK)
		var saved library.HistoryEntry
		decodeResponse(t, response, &saved)
		if saved.Track.Source != "soundcloud" || saved.Track.ID != "99" || saved.PlayCount != 1 {
			t.Fatalf("saved history = %#v", saved)
		}

		response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/history", input)
		assertStatus(t, response, http.StatusOK)
		var replay library.HistoryEntry
		decodeResponse(t, response, &replay)
		if replay.PlayCount != 1 || !replay.LastListenedAt.Equal(saved.LastListenedAt) {
			t.Fatalf("history replay = %#v, want %#v", replay, saved)
		}

		response = request(t, client, http.MethodGet, server.URL+"/api/v1/history?limit=1", nil)
		assertStatus(t, response, http.StatusOK)
		var listed struct {
			History []library.HistoryEntry `json:"history"`
		}
		decodeResponse(t, response, &listed)
		if len(listed.History) != 1 || listed.History[0].Track.ID != "99" {
			t.Fatalf("history list = %#v", listed.History)
		}

		conflict := map[string]any{
			"idempotency_key": "history-1", "occurred_at": "2026-10-03T12:01:00Z",
			"track": input["track"],
		}
		response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/history", conflict)
		assertStatus(t, response, http.StatusConflict)
		var body errorResponse
		decodeResponse(t, response, &body)
		if body.Error.Code != "idempotency_conflict" {
			t.Fatalf("history conflict code = %q", body.Error.Code)
		}

		response = request(t, client, http.MethodGet, server.URL+"/api/v1/history?limit=0", nil)
		assertStatus(t, response, http.StatusBadRequest)
		closeResponse(t, response)

		response = request(t, client, http.MethodDelete, server.URL+"/api/v1/me/history", nil)
		assertStatus(t, response, http.StatusNoContent)
		closeResponse(t, response)

		response = request(t, client, http.MethodGet, server.URL+"/api/v1/history", nil)
		assertStatus(t, response, http.StatusOK)
		decodeResponse(t, response, &listed)
		if len(listed.History) != 0 {
			t.Fatalf("history after clear = %#v", listed.History)
		}
	})

	t.Run("account playlists retain ordered tracks and idempotent writes", func(t *testing.T) {
		playlistID := "123e4567-e89b-12d3-a456-426614174000"
		input := map[string]any{
			"idempotency_key": "playlist-write-1",
			"name":            "В дороге",
			"description":     "Музыка для поездки",
			"pinned":          true,
			"tracks": []any{
				map[string]any{"source": "soundcloud", "id": "soundcloud:tracks:42", "title": "First", "artist": "Artist"},
				map[string]any{"source": "music", "id": "second", "title": "Second", "artist": "Artist"},
			},
		}
		response := requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/playlists/"+playlistID, input)
		assertStatus(t, response, http.StatusOK)
		var saved library.Playlist
		decodeResponse(t, response, &saved)
		if saved.ID != playlistID || !saved.Pinned || len(saved.Tracks) != 2 || saved.Tracks[0].ID != "42" || saved.Revision != 1 {
			t.Fatalf("saved playlist = %#v", saved)
		}

		response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/playlists/"+playlistID, input)
		assertStatus(t, response, http.StatusOK)
		var replay library.Playlist
		decodeResponse(t, response, &replay)
		if replay.Revision != saved.Revision || !replay.UpdatedAt.Equal(saved.UpdatedAt) {
			t.Fatalf("playlist replay = %#v, want %#v", replay, saved)
		}

		response = request(t, client, http.MethodGet, server.URL+"/api/v1/me/playlists", nil)
		assertStatus(t, response, http.StatusOK)
		var listed struct {
			Playlists []library.Playlist `json:"playlists"`
		}
		decodeResponse(t, response, &listed)
		if len(listed.Playlists) != 1 || listed.Playlists[0].Tracks[0].ID != "42" {
			t.Fatalf("playlists = %#v", listed.Playlists)
		}

		changed := map[string]any{
			"idempotency_key": "playlist-write-2", "name": input["name"],
			"description": input["description"], "pinned": true,
			"tracks": []any{input["tracks"].([]any)[1], input["tracks"].([]any)[0]},
		}
		response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/playlists/"+playlistID, changed)
		assertStatus(t, response, http.StatusOK)
		var reordered library.Playlist
		decodeResponse(t, response, &reordered)
		if reordered.Revision != 2 || reordered.Tracks[0].ID != "second" {
			t.Fatalf("reordered playlist = %#v", reordered)
		}

		conflicting := map[string]any{
			"idempotency_key": "playlist-write-1", "name": changed["name"],
			"description": changed["description"], "pinned": true,
			"tracks": changed["tracks"],
		}
		response = requestJSON(t, client, http.MethodPut, server.URL+"/api/v1/me/playlists/"+playlistID, conflicting)
		assertStatus(t, response, http.StatusConflict)
		var conflict errorResponse
		decodeResponse(t, response, &conflict)
		if conflict.Error.Code != "idempotency_conflict" {
			t.Fatalf("playlist conflict = %#v", conflict)
		}

		deleteInput := map[string]any{"idempotency_key": "playlist-delete-1"}
		response = requestJSON(t, client, http.MethodDelete, server.URL+"/api/v1/me/playlists/"+playlistID, deleteInput)
		assertStatus(t, response, http.StatusOK)
		var deleted library.PlaylistDeleteResult
		decodeResponse(t, response, &deleted)
		if !deleted.Deleted || deleted.ID != playlistID {
			t.Fatalf("deleted playlist = %#v", deleted)
		}
		response = requestJSON(t, client, http.MethodDelete, server.URL+"/api/v1/me/playlists/"+playlistID, deleteInput)
		assertStatus(t, response, http.StatusOK)
		closeResponse(t, response)
	})

	t.Run("events accepts array and envelope", func(t *testing.T) {
		play := map[string]any{
			"idempotency_key": "event-1", "type": "play", "track_source": "fixture", "track_id": "track-1",
		}
		response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/events", []any{play})
		assertStatus(t, response, http.StatusNoContent)
		closeResponse(t, response)

		search := map[string]any{"idempotency_key": "event-2", "type": "search"}
		response = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/events", map[string]any{"events": []any{search}})
		assertStatus(t, response, http.StatusNoContent)
		closeResponse(t, response)
		if fixture.events.userID != fixture.user.ID || len(fixture.events.batches) != 2 || fixture.events.batches[0][0].TrackID != "track-1" || fixture.events.batches[1][0].Type != "search" {
			t.Fatalf("captured events = %#v", fixture.events)
		}
	})

	t.Run("events rejects a track outside the claimed wave session", func(t *testing.T) {
		previousOwns := fixture.impressions.owns
		fixture.impressions.owns = false
		t.Cleanup(func() { fixture.impressions.owns = previousOwns })
		response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/events", []any{map[string]any{
			"idempotency_key": "event-outside-wave", "type": "seek",
			"track_source": "soundcloud", "track_id": "42",
			"session_id": "123e4567-e89b-12d3-a456-426614174000",
		}})
		assertStatus(t, response, http.StatusBadRequest)
		var body errorResponse
		decodeResponse(t, response, &body)
		if body.Error.Code != "unknown_wave_track" {
			t.Fatalf("error code = %q, want unknown_wave_track", body.Error.Code)
		}
	})

	t.Run("events rejects client-created impressions in a wave session", func(t *testing.T) {
		response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/events", []any{map[string]any{
			"idempotency_key": "event-wave-impression", "type": "impression",
			"track_source": "soundcloud", "track_id": "42",
			"session_id": "123e4567-e89b-12d3-a456-426614174000",
		}})
		assertStatus(t, response, http.StatusBadRequest)
		var body errorResponse
		decodeResponse(t, response, &body)
		if body.Error.Code != "invalid_wave_event" {
			t.Fatalf("error code = %q, want invalid_wave_event", body.Error.Code)
		}
	})

	t.Run("wave fails when impressions cannot be persisted", func(t *testing.T) {
		fixture.impressions.err = errors.New("database unavailable")
		t.Cleanup(func() { fixture.impressions.err = nil })
		response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/wave", map[string]any{
			"preferences": map[string]string{"activity": "work"},
		})
		assertStatus(t, response, http.StatusServiceUnavailable)
		var body errorResponse
		decodeResponse(t, response, &body)
		if body.Error.Code != "wave_persistence_failed" {
			t.Fatalf("error code = %q, want wave_persistence_failed", body.Error.Code)
		}
	})

	t.Run("wave", func(t *testing.T) {
		response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/wave", map[string]any{
			"preferences": map[string]string{"activity": "work", "mood": "calm", "diversity": "popular", "language": "any"},
			"round":       2,
		})
		assertStatus(t, response, http.StatusOK)
		var body struct {
			Tracks       []music.Track `json:"tracks"`
			SessionID    string        `json:"session_id"`
			ModelVersion string        `json:"model_version"`
			Reason       string        `json:"reason"`
		}
		decodeResponse(t, response, &body)
		if len(body.Tracks) != 1 || body.Tracks[0].ID != "recommended-1" || body.ModelVersion != "contract-v1" || body.Reason != "fixture ranking" {
			t.Fatalf("wave response = %#v", body)
		}
		if !validUUID(body.SessionID) {
			t.Fatalf("wave session id = %q, want UUID", body.SessionID)
		}
		if fixture.recommendations.request.UserID != fixture.user.ID || fixture.recommendations.request.Round != 2 || fixture.recommendations.request.Preferences.Activity != "work" {
			t.Fatalf("wave request = %#v", fixture.recommendations.request)
		}
		if fixture.impressions.sessionID != body.SessionID || fixture.impressions.userID != fixture.user.ID {
			t.Fatalf("saved impressions = %#v", fixture.impressions)
		}
	})

	t.Run("wave feedback", func(t *testing.T) {
		response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/wave/123e4567-e89b-12d3-a456-426614174000/feedback", map[string]any{
			"idempotency_key": "wave-feedback-1", "type": "skip", "track_source": "soundcloud", "track_id": "42",
		})
		assertStatus(t, response, http.StatusNoContent)
		closeResponse(t, response)
		batch := fixture.events.batches[len(fixture.events.batches)-1]
		if batch[0].SessionID != "123e4567-e89b-12d3-a456-426614174000" || batch[0].Type != "skip" {
			t.Fatalf("wave feedback = %#v", batch)
		}
	})

	t.Run("wave feedback requires an idempotency key", func(t *testing.T) {
		response := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/wave/123e4567-e89b-12d3-a456-426614174000/feedback", map[string]any{
			"type": "skip", "track_source": "soundcloud", "track_id": "42",
		})
		assertStatus(t, response, http.StatusBadRequest)
		var body errorResponse
		decodeResponse(t, response, &body)
		if body.Error.Code != "invalid_feedback" {
			t.Fatalf("error code = %q, want invalid_feedback", body.Error.Code)
		}
	})

	t.Run("music routes still fall through", func(t *testing.T) {
		response := request(t, client, http.MethodGet, server.URL+"/api/v1/search?q=fixture", nil)
		assertStatus(t, response, http.StatusTeapot)
		if response.Header.Get("X-Contract-Music-Engine") != "reached" {
			t.Fatal("request did not reach the mounted music engine")
		}
		closeResponse(t, response)
	})

	t.Run("logout clears session", func(t *testing.T) {
		response := request(t, client, http.MethodPost, server.URL+"/api/v1/auth/logout", nil)
		assertStatus(t, response, http.StatusNoContent)
		cookies := response.Cookies()
		if len(cookies) != 1 || cookies[0].Name != sessionCookie || cookies[0].Value != "" || cookies[0].MaxAge >= 0 {
			t.Fatalf("logout cookies = %#v", cookies)
		}
		closeResponse(t, response)
		if fixture.auth.loggedOutToken != fixture.sessionToken {
			t.Fatalf("logged out token = %q, want %q", fixture.auth.loggedOutToken, fixture.sessionToken)
		}
	})
}

func TestPlaybackWebSocketContract(t *testing.T) {
	fixture := newContractFixture()
	server := httptest.NewServer(fixture.handler())
	t.Cleanup(server.Close)

	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/playback/ws"
	dial := func(t *testing.T) *websocket.Conn {
		t.Helper()
		header := http.Header{}
		header.Set("Origin", server.URL)
		header.Set("Cookie", sessionCookie+"="+fixture.sessionToken)
		connection, response, err := websocket.DefaultDialer.Dial(websocketURL, header)
		if err != nil {
			if response != nil {
				defer response.Body.Close()
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("websocket dial error = %v, status = %d, body = %s", err, response.StatusCode, body)
			}
			t.Fatalf("websocket dial error = %v", err)
		}
		return connection
	}

	first := dial(t)
	t.Cleanup(func() { _ = first.Close() })
	second := dial(t)
	t.Cleanup(func() { _ = second.Close() })

	state := []byte(`{"type":"state","playing":true,"position":12.5,"track":{"id":"track-1"},"queue":[]}`)
	if err := first.WriteMessage(websocket.TextMessage, state); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	if err := second.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	messageType, got, err := second.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if messageType != websocket.TextMessage || !bytes.Equal(got, state) {
		t.Fatalf("relayed websocket message = type %d, %s", messageType, got)
	}
	if stored, ok := fixture.playback.LastState(fixture.user.ID); !ok || !bytes.Equal(stored, state) {
		t.Fatalf("hub last state = %s, %v", stored, ok)
	}

	if err := first.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
		t.Fatalf("write invalid state error = %v", err)
	}
	if err := first.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, _, err = first.ReadMessage()
	var closeError *websocket.CloseError
	if !errors.As(err, &closeError) || closeError.Code != websocket.CloseUnsupportedData {
		t.Fatalf("invalid state close error = %v, want code %d", err, websocket.CloseUnsupportedData)
	}
}

type contractFixture struct {
	user            auth.User
	session         auth.Session
	sessionToken    string
	auth            *fakeAuthBackend
	library         *fakeLibraryBackend
	events          *fakeEventBackend
	outbox          *fakeMailBackend
	playback        *playback.Hub
	recommendations *fakeRecommendationBackend
	impressions     *fakeImpressionBackend
}

func newContractFixture() *contractFixture {
	now := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	user := auth.User{
		ID: "user-contract", Email: "listener@example.com", DisplayName: "Listener",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	session := auth.Session{
		ID: "session-contract", UserID: user.ID, DeviceID: "contract-device",
		ExpiresAt: now.Add(24 * time.Hour), LastSeenAt: now, CreatedAt: now,
	}
	fixture := &contractFixture{
		user: user, session: session, sessionToken: "session-token",
		library: &fakeLibraryBackend{snapshot: library.Snapshot{
			Payload: json.RawMessage(`{"likes":[{"id":"saved"}],"history":[]}`), Version: 7,
		}},
		events:   &fakeEventBackend{},
		outbox:   &fakeMailBackend{},
		playback: playback.NewHub(playback.DefaultMaxMessageBytes),
		recommendations: &fakeRecommendationBackend{result: recommendation.Result{
			Tracks:       []music.Track{{ID: "recommended-1", Source: "fixture", Title: "Contract Track", Artist: "Fixture Artist", Access: "playable"}},
			ModelVersion: "contract-v1", Reason: "fixture ranking",
		}},
		impressions: &fakeImpressionBackend{owns: true},
	}
	fixture.auth = &fakeAuthBackend{
		user: user, session: session, sessionToken: fixture.sessionToken,
		registration: auth.Registration{User: user, EmailVerificationToken: "verification-token"},
	}
	return fixture
}

func (f *contractFixture) handler() http.Handler {
	server := &Server{
		auth: f.auth, libraries: f.library, events: f.events, mailOutbox: f.outbox,
		playback: f.playback, recommendations: f.recommendations,
		impressions: f.impressions,
		publicURL:   "https://mixora.test", sessionTTL: 24 * time.Hour,
	}
	musicEngine := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Contract-Music-Engine", "reached")
		w.WriteHeader(http.StatusTeapot)
	})
	return server.handler(musicEngine)
}

type fakeAuthBackend struct {
	user            auth.User
	session         auth.Session
	sessionToken    string
	registration    auth.Registration
	registeredEmail string
	registeredName  string
	loginMetadata   auth.SessionMetadata
	loggedOutToken  string
}

func (f *fakeAuthBackend) Register(_ context.Context, email, _ string, displayName string) (auth.Registration, error) {
	f.registeredEmail = email
	f.registeredName = displayName
	return f.registration, nil
}

func (f *fakeAuthBackend) Login(_ context.Context, _, _ string, metadata auth.SessionMetadata) (auth.Login, error) {
	f.loginMetadata = metadata
	return auth.Login{User: f.user, Session: f.session, SessionToken: f.sessionToken}, nil
}

func (f *fakeAuthBackend) Authenticate(_ context.Context, token string) (auth.User, auth.Session, error) {
	if token != f.sessionToken {
		return auth.User{}, auth.Session{}, auth.ErrInvalidToken
	}
	return f.user, f.session, nil
}

func (f *fakeAuthBackend) Logout(_ context.Context, token string) error {
	f.loggedOutToken = token
	return nil
}

func (f *fakeAuthBackend) VerifyEmail(context.Context, string) (auth.User, error) {
	return f.user, nil
}

func (*fakeAuthBackend) BeginPasswordReset(context.Context, string) (*auth.PasswordReset, error) {
	return nil, nil
}

func (f *fakeAuthBackend) ResetPassword(context.Context, string, string) (auth.User, error) {
	return f.user, nil
}

type fakeLibraryBackend struct {
	snapshot           library.Snapshot
	userID             string
	payload            json.RawMessage
	preferenceUserID   string
	preferences        map[string]library.TrackPreference
	preferenceReceipts map[string]fakePreferenceReceipt
	history            map[string]library.HistoryEntry
	historyReceipts    map[string]fakeHistoryReceipt
	playlists          map[string]library.Playlist
	playlistReceipts   map[string]fakePlaylistReceipt
}

type fakePreferenceReceipt struct {
	fingerprint string
	preference  library.TrackPreference
}

type fakeHistoryReceipt struct {
	fingerprint string
	entry       library.HistoryEntry
}

type fakePlaylistReceipt struct {
	fingerprint string
	playlist    *library.Playlist
	deleted     *library.PlaylistDeleteResult
}

func (f *fakeLibraryBackend) Get(context.Context, string) (library.Snapshot, error) {
	return f.snapshot, nil
}

func (f *fakeLibraryBackend) Put(_ context.Context, userID string, payload json.RawMessage) (library.Snapshot, error) {
	f.userID = userID
	f.payload = append(json.RawMessage(nil), payload...)
	f.snapshot = library.Snapshot{Payload: append(json.RawMessage(nil), payload...), Version: f.snapshot.Version + 1}
	return f.snapshot, nil
}

func (f *fakeLibraryBackend) ListTrackPreferences(_ context.Context, userID string) ([]library.TrackPreference, error) {
	f.preferenceUserID = userID
	result := make([]library.TrackPreference, 0, len(f.preferences))
	for _, preference := range f.preferences {
		result = append(result, preference)
	}
	return result, nil
}

func (f *fakeLibraryBackend) SetTrackPreference(_ context.Context, userID string, input library.PreferenceInput) (library.TrackPreference, error) {
	normalized, err := library.NormalizePreferenceInput(input)
	if err != nil {
		return library.TrackPreference{}, err
	}
	if f.preferenceReceipts == nil {
		f.preferenceReceipts = make(map[string]fakePreferenceReceipt)
	}
	fingerprint := string(library.PreferenceRequestFingerprint(normalized))
	if receipt, found := f.preferenceReceipts[normalized.IdempotencyKey]; found {
		if receipt.fingerprint != fingerprint {
			return library.TrackPreference{}, library.ErrIdempotencyKeyConflict
		}
		return receipt.preference, nil
	}
	if f.preferences == nil {
		f.preferences = make(map[string]library.TrackPreference)
	}
	snapshot := library.TrackSnapshot{
		ID: normalized.Track.ID, Source: normalized.Track.Source, Title: normalized.Track.Title,
		Artist: normalized.Track.Artist, ArtistID: normalized.Track.ArtistID, Artwork: normalized.Track.Artwork,
		Duration: normalized.Track.Duration, Explicit: normalized.Track.Explicit, Access: normalized.Track.Access,
		Permalink: normalized.Track.Permalink,
	}
	key := snapshot.Source + ":" + snapshot.ID
	current, found := f.preferences[key]
	revision := int64(1)
	if found {
		revision = current.Revision
		if current.Preference != normalized.Preference || current.Track != snapshot {
			revision++
		}
	}
	result := library.TrackPreference{
		Track: snapshot, Preference: normalized.Preference, Revision: revision, UpdatedAt: time.Now().UTC(),
	}
	if found && current.Preference == result.Preference && current.Track == result.Track {
		result.UpdatedAt = current.UpdatedAt
	}
	f.preferences[key] = result
	f.preferenceReceipts[normalized.IdempotencyKey] = fakePreferenceReceipt{fingerprint: fingerprint, preference: result}
	f.preferenceUserID = userID
	return result, nil
}

func (f *fakeLibraryBackend) ListHistory(_ context.Context, _ string, limit int) ([]library.HistoryEntry, error) {
	entries := make([]library.HistoryEntry, 0, len(f.history))
	for _, entry := range f.history {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].LastListenedAt.Equal(entries[j].LastListenedAt) {
			return entries[i].Track.Source+":"+entries[i].Track.ID < entries[j].Track.Source+":"+entries[j].Track.ID
		}
		return entries[i].LastListenedAt.After(entries[j].LastListenedAt)
	})
	if limit < len(entries) {
		entries = entries[:limit]
	}
	return entries, nil
}

func (f *fakeLibraryBackend) RecordHistory(_ context.Context, _ string, input library.HistoryInput) (library.HistoryEntry, error) {
	normalized, err := library.NormalizeHistoryInput(input)
	if err != nil {
		return library.HistoryEntry{}, err
	}
	if f.historyReceipts == nil {
		f.historyReceipts = make(map[string]fakeHistoryReceipt)
	}
	fingerprint := string(library.HistoryRequestFingerprint(normalized))
	if receipt, found := f.historyReceipts[normalized.IdempotencyKey]; found {
		if receipt.fingerprint != fingerprint {
			return library.HistoryEntry{}, library.ErrHistoryIdempotencyKeyConflict
		}
		return receipt.entry, nil
	}
	if f.history == nil {
		f.history = make(map[string]library.HistoryEntry)
	}
	snapshot := library.TrackSnapshot{
		ID: normalized.Track.ID, Source: normalized.Track.Source, Title: normalized.Track.Title,
		Artist: normalized.Track.Artist, ArtistID: normalized.Track.ArtistID, Artwork: normalized.Track.Artwork,
		Duration: normalized.Track.Duration, Explicit: normalized.Track.Explicit, Access: normalized.Track.Access,
		Permalink: normalized.Track.Permalink,
	}
	key := snapshot.Source + ":" + snapshot.ID
	entry, found := f.history[key]
	if !found {
		entry = library.HistoryEntry{Track: snapshot, FirstListenedAt: normalized.OccurredAt, LastListenedAt: normalized.OccurredAt, PlayCount: 1}
	} else {
		entry.PlayCount++
		if normalized.OccurredAt.Before(entry.FirstListenedAt) {
			entry.FirstListenedAt = normalized.OccurredAt
		}
		if !normalized.OccurredAt.Before(entry.LastListenedAt) {
			entry.LastListenedAt = normalized.OccurredAt
			entry.Track = snapshot
		}
	}
	f.history[key] = entry
	f.historyReceipts[normalized.IdempotencyKey] = fakeHistoryReceipt{fingerprint: fingerprint, entry: entry}
	return entry, nil
}

func (f *fakeLibraryBackend) ClearHistory(_ context.Context, _ string) error {
	f.history = make(map[string]library.HistoryEntry)
	f.historyReceipts = make(map[string]fakeHistoryReceipt)
	return nil
}

func (f *fakeLibraryBackend) ListPlaylists(_ context.Context, _ string) ([]library.Playlist, error) {
	result := make([]library.Playlist, 0, len(f.playlists))
	for _, playlist := range f.playlists {
		result = append(result, copyFakePlaylist(playlist))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func (f *fakeLibraryBackend) ReplacePlaylist(_ context.Context, _ string, playlistID string, input library.PlaylistInput) (library.Playlist, error) {
	playlistID, err := library.NormalizePlaylistID(playlistID)
	if err != nil {
		return library.Playlist{}, err
	}
	normalized, err := library.NormalizePlaylistInput(input)
	if err != nil {
		return library.Playlist{}, err
	}
	if f.playlistReceipts == nil {
		f.playlistReceipts = make(map[string]fakePlaylistReceipt)
	}
	fingerprint := string(library.PlaylistRequestFingerprint(playlistID, normalized))
	if receipt, found := f.playlistReceipts[normalized.IdempotencyKey]; found {
		if receipt.fingerprint != fingerprint || receipt.playlist == nil {
			return library.Playlist{}, library.ErrPlaylistIdempotencyConflict
		}
		return copyFakePlaylist(*receipt.playlist), nil
	}
	if f.playlists == nil {
		f.playlists = make(map[string]library.Playlist)
	}
	tracks := make([]library.TrackSnapshot, 0, len(normalized.Tracks))
	for _, track := range normalized.Tracks {
		tracks = append(tracks, library.TrackSnapshot{
			ID: track.ID, Source: track.Source, Title: track.Title, Artist: track.Artist,
			ArtistID: track.ArtistID, Artwork: track.Artwork, Duration: track.Duration,
			Explicit: track.Explicit, Access: track.Access, Permalink: track.Permalink,
		})
	}
	now := time.Now().UTC()
	previous, found := f.playlists[playlistID]
	result := library.Playlist{
		ID: playlistID, Name: normalized.Name, Description: normalized.Description,
		Tracks: tracks, Pinned: normalized.Pinned, Liked: normalized.Liked,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if found {
		result.Revision = previous.Revision
		result.CreatedAt = previous.CreatedAt
		if reflect.DeepEqual(previous.Tracks, tracks) && previous.Name == result.Name && previous.Description == result.Description && previous.Pinned == result.Pinned && previous.Liked == result.Liked {
			result.UpdatedAt = previous.UpdatedAt
		} else {
			result.Revision++
		}
	}
	f.playlists[playlistID] = copyFakePlaylist(result)
	stored := copyFakePlaylist(result)
	f.playlistReceipts[normalized.IdempotencyKey] = fakePlaylistReceipt{fingerprint: fingerprint, playlist: &stored}
	return result, nil
}

func (f *fakeLibraryBackend) DeletePlaylist(_ context.Context, _ string, playlistID string, input library.PlaylistDeleteInput) (library.PlaylistDeleteResult, error) {
	playlistID, err := library.NormalizePlaylistID(playlistID)
	if err != nil {
		return library.PlaylistDeleteResult{}, err
	}
	normalized, err := library.NormalizePlaylistDeleteInput(input)
	if err != nil {
		return library.PlaylistDeleteResult{}, err
	}
	if f.playlistReceipts == nil {
		f.playlistReceipts = make(map[string]fakePlaylistReceipt)
	}
	fingerprint := string(library.PlaylistDeleteRequestFingerprint(playlistID))
	if receipt, found := f.playlistReceipts[normalized.IdempotencyKey]; found {
		if receipt.fingerprint != fingerprint || receipt.deleted == nil {
			return library.PlaylistDeleteResult{}, library.ErrPlaylistIdempotencyConflict
		}
		return *receipt.deleted, nil
	}
	delete(f.playlists, playlistID)
	result := library.PlaylistDeleteResult{ID: playlistID, Deleted: true}
	f.playlistReceipts[normalized.IdempotencyKey] = fakePlaylistReceipt{fingerprint: fingerprint, deleted: &result}
	return result, nil
}

func copyFakePlaylist(playlist library.Playlist) library.Playlist {
	playlist.Tracks = append([]library.TrackSnapshot(nil), playlist.Tracks...)
	return playlist
}

type fakeEventBackend struct {
	userID  string
	batches [][]events.Event
}

func (f *fakeEventBackend) Add(_ context.Context, userID string, input []events.Event) error {
	if len(input) == 0 || len(input) > 100 {
		return errors.New("events batch must contain 1 to 100 items")
	}
	for _, event := range input {
		if err := events.Validate(event); err != nil {
			return err
		}
	}
	f.userID = userID
	f.batches = append(f.batches, append([]events.Event(nil), input...))
	return nil
}

type fakeMailBackend struct {
	message mail.Message
}

func (f *fakeMailBackend) Enqueue(_ context.Context, message mail.Message) (int64, error) {
	f.message = message
	return 1, nil
}

type fakeRecommendationBackend struct {
	request recommendation.Request
	result  recommendation.Result
}

func (f *fakeRecommendationBackend) Recommend(_ context.Context, request recommendation.Request) (recommendation.Result, error) {
	f.request = request
	return f.result, nil
}

type fakeImpressionBackend struct {
	userID    string
	sessionID string
	request   recommendation.Request
	result    recommendation.Result
	owns      bool
	err       error
}

func (f *fakeImpressionBackend) Save(_ context.Context, userID, sessionID string, request recommendation.Request, result recommendation.Result) error {
	if f.err != nil {
		return f.err
	}
	f.userID = userID
	f.sessionID = sessionID
	f.request = request
	f.result = result
	return nil
}

func (f *fakeImpressionBackend) Owns(_ context.Context, _, _, _, _ string) (bool, error) {
	return f.owns, nil
}

func requestJSON(t *testing.T, client *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	request := newJSONRequest(t, method, url, body)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, url, err)
	}
	return response
}

func newJSONRequest(t *testing.T, method, url string, body any) *http.Request {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	request, err := http.NewRequest(method, url, bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	return request
}

func request(t *testing.T, client *http.Client, method, url string, body io.Reader) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, url, err)
	}
	return response
}

func assertStatus(t *testing.T, response *http.Response, want int) {
	t.Helper()
	if response.StatusCode != want {
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, want, body)
	}
}

func assertMiddlewareHeaders(t *testing.T, response *http.Response) {
	t.Helper()
	if response.Header.Get("X-Request-ID") == "" || response.Header.Get("X-Content-Type-Options") != "nosniff" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("middleware headers = %#v", response.Header)
	}
}

func decodeResponse(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", contentType)
	}
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
}

func closeResponse(t *testing.T, response *http.Response) {
	t.Helper()
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response body error = %v", err)
	}
}

func jsonEqual(left, right []byte) bool {
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && valuesEqual(leftValue, rightValue)
}

func valuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}
