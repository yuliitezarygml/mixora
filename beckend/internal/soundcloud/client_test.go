package soundcloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type staticToken string

func (t staticToken) AccessToken(context.Context) (string, error) { return string(t), nil }
func TestSearchAndStreams(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth test-token" {
			t.Error("missing auth")
		}
		switch r.URL.Path {
		case "/tracks":
			if r.URL.Query().Get("q") != "rock & jazz" || r.URL.Query().Get("linked_partitioning") != "true" {
				t.Error("invalid query")
			}
			w.Write([]byte(`{"collection":[{"urn":"soundcloud:tracks:123","access":"playable"}],"next_href":null}`))
		case "/tracks/soundcloud:tracks:123/streams":
			w.Write([]byte(`{"hls_aac_160_url":"https://api.soundcloud.com/tracks/123/streams/abc"}`))
		default:
			w.WriteHeader(429)
		}
	}))
	defer upstream.Close()
	client := New(staticToken("test-token"))
	client.baseURL = upstream.URL
	data, err := client.Search(context.Background(), "rock & jazz", 20, 0)
	if err != nil || !json.Valid(data) {
		t.Fatalf("search: %s %v", data, err)
	}
	if _, err = client.Streams(context.Background(), "soundcloud:tracks:123"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Streams(context.Background(), "../../me"); !errors.Is(err, ErrID) {
		t.Fatal("invalid path accepted")
	}
	_, err = client.Streams(context.Background(), "999")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Fatal("upstream status lost")
	}
	if _, err = New(nil).Search(context.Background(), "x", 20, 0); !errors.Is(err, ErrDisabled) {
		t.Fatal("missing credentials not handled")
	}
}
func TestTokenExchange(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		r.ParseForm()
		if calls == 1 {
			id, secret, ok := r.BasicAuth()
			if !ok || id != "client" || secret != "secret" || r.Form.Get("grant_type") != "client_credentials" {
				t.Error("client credentials must use Basic auth")
			}
		}
		if calls == 2 {
			if r.Form.Get("refresh_token") != "refresh-1" || r.Form.Get("client_secret") != "secret" || r.Form.Get("grant_type") != "refresh_token" {
				t.Error("incorrect refresh")
			}
		}
		w.Write([]byte(`{"access_token":"access","refresh_token":"refresh-1","expires_in":3600}`))
	}))
	defer server.Close()
	first, err := exchange(context.Background(), server.Client(), server.URL, "client", "secret", Token{})
	if err != nil {
		t.Fatal(err)
	}
	if first.ExpiresAt.Before(time.Now().Add(59 * time.Minute)) {
		t.Fatal("expiry not recorded")
	}
	if _, err = exchange(context.Background(), server.Client(), server.URL, "client", "secret", first); err != nil {
		t.Fatal(err)
	}
}
func TestEncryptedTokens(t *testing.T) {
	store, err := NewTokens(nil, "client", "secret", strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := store.seal([]byte("private-refresh-token"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := store.open(encrypted)
	if err != nil || string(plain) != "private-refresh-token" {
		t.Fatal("encryption roundtrip")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err = store.open(encrypted); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err = NewTokens(nil, "client", "secret", "short"); err == nil {
		t.Fatal("weak key accepted")
	}
}
