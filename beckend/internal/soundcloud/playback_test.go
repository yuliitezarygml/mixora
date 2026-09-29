package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlaybackResolvesWithoutExposingToken(t *testing.T) {
	var base string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth server-secret" {
			t.Error("missing server authorization")
		}
		if r.URL.Path == "/tracks/123/streams" {
			json.NewEncoder(w).Encode(map[string]string{"hls_mp3_128_url": base + "/tracks/123/streams/abc"})
			return
		}
		w.Header().Set("Location", "https://cf-hls-media.sndcdn.com/playlist.m3u8?signature=test")
		w.WriteHeader(302)
	}))
	defer upstream.Close()
	base = upstream.URL
	client := New(staticToken("server-secret"))
	client.baseURL = base
	p, err := client.Playback(context.Background(), "123")
	if err != nil || p.Format != "hls" || p.URL != "https://cf-hls-media.sndcdn.com/playlist.m3u8?signature=test" {
		t.Fatalf("unexpected playback: %v %v", p, err)
	}
}

func TestPlaybackSupportsCurrentAACStreams(t *testing.T) {
	for _, field := range []string{"hls_aac_160_url", "hls_aac_96_url"} {
		for _, direct := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/direct=%t", field, direct), func(t *testing.T) {
				const cdn = "https://playback.media-streaming.soundcloud.cloud/track/aac/playlist.m3u8?signature=test"
				var base string
				requests := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					if r.Header.Get("Authorization") != "OAuth server-secret" {
						t.Error("missing API authorization")
					}
					if r.URL.Path == "/tracks/123/streams" {
						target := base + "/tracks/123/streams/aac"
						if direct {
							target = cdn
						}
						json.NewEncoder(w).Encode(map[string]string{field: target})
						return
					}
					w.Header().Set("Location", cdn)
					w.WriteHeader(http.StatusFound)
				}))
				defer upstream.Close()
				base = upstream.URL
				client := New(staticToken("server-secret"))
				client.baseURL = base
				p, err := client.Playback(context.Background(), "123")
				if err != nil || p.URL != cdn || p.Format != "hls" {
					t.Fatalf("AAC playback failed: %v %v", p, err)
				}
				wantRequests := 2
				if direct {
					wantRequests = 1
				}
				if requests != wantRequests {
					t.Errorf("requests = %d, want %d", requests, wantRequests)
				}
			})
		}
	}
}
func TestPlaybackRejectsUntrustedURLs(t *testing.T) {
	for _, mode := range []string{"api", "cdn", "missing"} {
		t.Run(mode, func(t *testing.T) {
			var base string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/tracks/123/streams" {
					target := base + "/tracks/123/streams/abc"
					if mode == "api" {
						target = "https://attacker.example/secret"
					}
					if mode == "missing" {
						target = ""
					}
					json.NewEncoder(w).Encode(map[string]string{"hls_mp3_128_url": target})
					return
				}
				w.Header().Set("Location", "https://sndcdn.com.attacker.example/playlist.m3u8")
				w.WriteHeader(302)
			}))
			defer upstream.Close()
			base = upstream.URL
			c := New(staticToken("server-secret"))
			c.baseURL = base
			if _, err := c.Playback(context.Background(), "123"); err == nil {
				t.Fatal("untrusted playback accepted")
			}
		})
	}
}

func TestPlaybackFollowsOnlyTrustedAPIHops(t *testing.T) {
	var base string
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "OAuth server-secret" {
			t.Error("missing API authorization")
		}
		switch r.URL.Path {
		case "/tracks/123/streams":
			json.NewEncoder(w).Encode(map[string]string{"hls_aac_160_url": base + "/tracks/123/streams/aac"})
		case "/tracks/123/streams/aac":
			w.Header().Set("Location", "/tracks/123/streams/current")
			w.WriteHeader(http.StatusTemporaryRedirect)
		case "/tracks/123/streams/current":
			w.Header().Set("Location", "https://cf-hls-media.sndcdn.com:443/playlist.m3u8?signature=test")
			w.WriteHeader(http.StatusFound)
		default:
			t.Error("unexpected request")
		}
	}))
	defer upstream.Close()
	base = upstream.URL
	client := New(staticToken("server-secret"))
	client.baseURL = base
	p, err := client.Playback(context.Background(), "123")
	if err != nil || p.Format != "hls" || requests != 3 {
		t.Fatalf("unexpected resolution: %v %v (%d requests)", p, err, requests)
	}
}

func TestPlaybackBoundsRedirectsAndRejectsUnsafeLocations(t *testing.T) {
	for _, location := range []string{
		"/tracks/123/streams/loop", // bounded even on the trusted API
		"/unrelated-api-path",
		"http://cf-hls-media.sndcdn.com/audio",
		"https://cf-hls-media.sndcdn.com:444/audio",
		"https://user:password@cf-hls-media.sndcdn.com/audio",
		"https://sndcdn.com.attacker.example/audio",
		"https://playback.media-streaming.soundcloud.cloud.attacker.example/audio",
		"https://attacker.soundcloud.cloud/audio",
		"",
	} {
		t.Run(location, func(t *testing.T) {
			var base string
			requests := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path == "/tracks/123/streams" {
					json.NewEncoder(w).Encode(map[string]string{"hls_aac_160_url": base + "/tracks/123/streams/loop"})
					return
				}
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusFound)
			}))
			defer upstream.Close()
			base = upstream.URL
			client := New(staticToken("server-secret"))
			client.baseURL = base
			if _, err := client.Playback(context.Background(), "123"); err == nil {
				t.Fatal("unsafe or unresolved redirect accepted")
			}
			if requests > 5 {
				t.Fatalf("redirect limit exceeded: %d", requests)
			}
		})
	}
}
