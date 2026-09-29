package soundcloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogUsesOnlySupportedResources(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "OAuth test" {
			t.Error("missing authorization")
		}
		w.Write([]byte(`{"collection":[]}`))
	}))
	defer upstream.Close()
	c := New(staticToken("test"))
	c.baseURL = upstream.URL
	for _, args := range [][3]string{{"users", "soundcloud:users:123", "tracks"}, {"users", "123", "playlists"}, {"playlists", "456", ""}, {"tracks", "789", ""}} {
		if _, err := c.Resource(context.Background(), args[0], args[1], args[2]); err != nil {
			t.Fatal(err)
		}
	}
	if len(paths) != 4 {
		t.Fatal(paths)
	}
	for _, args := range [][3]string{{"oauth", "123", ""}, {"users", "../me", "tracks"}, {"tracks", "123", "secrets"}, {"users", "https://other.test", ""}} {
		if _, err := c.Resource(context.Background(), args[0], args[1], args[2]); err == nil {
			t.Fatal("invalid resource accepted")
		}
	}
	if len(paths) != 4 {
		t.Fatal("invalid resource reached upstream")
	}
}
