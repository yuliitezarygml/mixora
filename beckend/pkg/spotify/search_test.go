package spotify_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/iulian/soundcloud-go/pkg/spotify"
	"github.com/iulian/soundcloud-go/pkg/spotify/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSearchOfficialAPIParsesRequestedResourceType(t *testing.T) {
	tests := []struct {
		name     string
		kind     spotify.ResourceType
		response string
		assert   func(*testing.T, *models.SearchResult)
	}{
		{
			name: "track",
			kind: spotify.TypeTrack,
			response: `{
				"tracks": {"items": [{
					"id": "track-1", "uri": "spotify:track:track-1", "name": "Track One",
					"duration_ms": 212000, "explicit": true, "popularity": 91,
					"preview_url": "https://cdn.example.test/preview.mp3", "is_playable": false,
					"track_number": 2, "disc_number": 1,
					"external_urls": {"spotify": "https://open.spotify.com/track/track-1"},
					"artists": [{"id": "artist-1", "uri": "spotify:artist:artist-1", "name": "Artist One"}],
					"album": {
						"id": "album-1", "uri": "spotify:album:album-1", "name": "Album One",
						"album_type": "album", "release_date": "2025-01-01", "total_tracks": 10,
						"images": [{"url": "https://img.example.test/album.jpg", "height": 640, "width": 640}],
						"artists": [{"id": "artist-1", "name": "Artist One"}]
					}
				}]}
			}`,
			assert: func(t *testing.T, result *models.SearchResult) {
				t.Helper()
				if len(result.Tracks) != 1 || len(result.Artists) != 0 || len(result.Albums) != 0 || len(result.Playlists) != 0 {
					t.Fatalf("expected exactly one track result, got %+v", result)
				}
				track := result.Tracks[0]
				if track.Title != "Track One" || track.DurationMs != 212000 || track.IsPlayable || track.Popularity != 91 || track.TrackNumber != 2 || track.DiscNumber != 1 {
					t.Fatalf("track fields were not normalized: %+v", track)
				}
				if len(track.Artists) != 1 || track.Artists[0].ExternalURL != "https://open.spotify.com/artist/artist-1" {
					t.Fatalf("track artists were not normalized: %+v", track.Artists)
				}
				if track.Album == nil || track.Album.Name != "Album One" || track.Album.ReleaseDate != "2025-01-01" || len(track.Album.Images) != 1 {
					t.Fatalf("track album was not normalized: %+v", track.Album)
				}
			},
		},
		{
			name: "artist",
			kind: spotify.TypeArtist,
			response: `{
				"artists": {"items": [{
					"id": "artist-1", "uri": "spotify:artist:artist-1", "name": "Artist One",
					"genres": ["electronic", "pop"], "popularity": 87, "followers": {"total": 12345},
					"images": [{"url": "https://img.example.test/artist.jpg", "height": 320, "width": 320}],
					"external_urls": {"spotify": "https://open.spotify.com/artist/artist-1"}
				}]}
			}`,
			assert: func(t *testing.T, result *models.SearchResult) {
				t.Helper()
				if len(result.Artists) != 1 || len(result.Tracks) != 0 || len(result.Albums) != 0 || len(result.Playlists) != 0 {
					t.Fatalf("expected exactly one artist result, got %+v", result)
				}
				artist := result.Artists[0]
				if artist.Name != "Artist One" || artist.Followers != 12345 || artist.Popularity != 87 || len(artist.Genres) != 2 || len(artist.Images) != 1 {
					t.Fatalf("artist fields were not normalized: %+v", artist)
				}
			},
		},
		{
			name: "album",
			kind: spotify.TypeAlbum,
			response: `{
				"albums": {"items": [{
					"id": "album-1", "uri": "spotify:album:album-1", "name": "Album One",
					"album_type": "single", "release_date": "2024-10-11", "total_tracks": 4,
					"images": [{"url": "https://img.example.test/album.jpg", "height": 300, "width": 300}],
					"artists": [{"id": "artist-1", "name": "Artist One"}],
					"external_urls": {"spotify": "https://open.spotify.com/album/album-1"}
				}]}
			}`,
			assert: func(t *testing.T, result *models.SearchResult) {
				t.Helper()
				if len(result.Albums) != 1 || len(result.Tracks) != 0 || len(result.Artists) != 0 || len(result.Playlists) != 0 {
					t.Fatalf("expected exactly one album result, got %+v", result)
				}
				album := result.Albums[0]
				if album.Name != "Album One" || album.AlbumType != "single" || album.ReleaseDate != "2024-10-11" || album.TotalTracks != 4 {
					t.Fatalf("album fields were not normalized: %+v", album)
				}
				if len(album.Artists) != 1 || album.Artists[0].URI != "spotify:artist:artist-1" || len(album.Images) != 1 {
					t.Fatalf("album relationships were not normalized: %+v", album)
				}
			},
		},
		{
			name: "playlist",
			kind: spotify.TypePlaylist,
			response: `{
				"playlists": {"items": [{
					"id": "playlist-1", "uri": "spotify:playlist:playlist-1", "name": "Playlist One",
					"description": "A test playlist", "owner": {"id": "owner-id", "display_name": "Mixora"},
					"images": [{"url": "https://img.example.test/playlist.jpg", "height": 480, "width": 480}],
					"tracks": {"total": 42},
					"external_urls": {"spotify": "https://open.spotify.com/playlist/playlist-1"}
				}]}
			}`,
			assert: func(t *testing.T, result *models.SearchResult) {
				t.Helper()
				if len(result.Playlists) != 1 || len(result.Tracks) != 0 || len(result.Artists) != 0 || len(result.Albums) != 0 {
					t.Fatalf("expected exactly one playlist result, got %+v", result)
				}
				playlist := result.Playlists[0]
				if playlist.Name != "Playlist One" || playlist.Description != "A test playlist" || playlist.Owner != "Mixora" || playlist.TotalTracks != 42 || len(playlist.Images) != 1 {
					t.Fatalf("playlist fields were not normalized: %+v", playlist)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := spotify.New(
				context.Background(),
				spotify.WithAccessToken("test-access-token"),
				spotify.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host != "api.spotify.com" || req.URL.Path != "/v1/search" {
						t.Fatalf("unexpected request target: %s", req.URL)
					}
					if got := req.URL.Query().Get("type"); got != string(test.kind) {
						t.Fatalf("expected search type %q, got %q", test.kind, got)
					}
					if got := req.URL.Query().Get("q"); got != "Mixora query" {
						t.Fatalf("expected search query to be preserved, got %q", got)
					}
					if got := req.Header.Get("Authorization"); got != "Bearer test-access-token" {
						t.Fatalf("unexpected authorization header %q", got)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(test.response)),
						Request:    req,
					}, nil
				})}),
			)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}

			result, err := client.Search(context.Background(), "Mixora query", test.kind, 3)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			test.assert(t, result)
		})
	}
}
