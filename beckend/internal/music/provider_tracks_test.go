package music

import (
	"testing"

	spotifymodels "github.com/iulian/soundcloud-go/pkg/spotify/models"
	ytdlpmodels "github.com/iulian/soundcloud-go/pkg/ytdlp/models"
)

func TestFromSpotifyProducesProviderNeutralTrack(t *testing.T) {
	track := FromSpotify(spotifymodels.Track{
		URI:         "spotify:track:spotify-track",
		Title:       " Song ",
		DurationMs:  245000,
		Explicit:    true,
		IsPlayable:  true,
		ExternalURL: "https://open.spotify.com/track/spotify-track",
		Artists: []spotifymodels.Artist{
			{ID: "artist-1", Name: " First Artist ", ExternalURL: "https://open.spotify.com/artist/artist-1"},
			{ID: "artist-2", Name: "Second Artist"},
		},
		Album: &spotifymodels.Album{
			Name:   " Album ",
			Images: []spotifymodels.Image{{URL: ""}, {URL: "https://images.example/cover.jpg"}},
		},
	})

	if track.ID != "spotify-track" || track.Source != "spotify" {
		t.Fatalf("identity = %#v", track)
	}
	if track.Artist != "First Artist, Second Artist" || track.ArtistID != "artist-1" || track.ArtistURL != "https://open.spotify.com/artist/artist-1" {
		t.Fatalf("artist metadata = %#v", track)
	}
	if track.Artwork != "https://images.example/cover.jpg" || track.Album != "Album" || track.Duration != 245 || !track.Explicit || track.Access != "blocked" {
		t.Fatalf("renderable metadata = %#v", track)
	}
}

func TestFromSpotifyUsesPreviewAccessWhenPublicResultHasPreview(t *testing.T) {
	track := FromSpotify(spotifymodels.Track{
		ID:         "spotify-track",
		Title:      "Song",
		IsPlayable: false,
		PreviewURL: "https://p.scdn.co/mp3-preview/example",
	})
	if track.Access != "preview" {
		t.Fatalf("access = %q, want preview", track.Access)
	}
}

func TestFromYTDLPUsesStableExtractorOrHostSourceWithoutAudioURL(t *testing.T) {
	track := FromYTDLP(ytdlpmodels.MediaItem{
		ID:          "video-42",
		Title:       " Video ",
		Uploader:    "Uploader",
		Duration:    91.5,
		Thumbnail:   "https://images.example/thumbnail.jpg",
		WebpageURL:  "https://www.youtube.com/watch?v=video-42",
		Extractor:   "youtube:search",
		AudioURL:    "https://cdn.example/temporary-audio-url",
		Description: " Description ",
		Album:       " Album ",
	})
	if track.Source != "youtube" || track.ID != "video-42" || track.Artist != "Uploader" || track.Access != "playable" {
		t.Fatalf("identity = %#v", track)
	}
	if track.Permalink != "https://www.youtube.com/watch?v=video-42" || track.Description != "Description" || track.Album != "Album" {
		t.Fatalf("metadata = %#v", track)
	}
	if track.Permalink == "https://cdn.example/temporary-audio-url" {
		t.Fatalf("direct audio URL leaked into catalog track: %#v", track)
	}

	if source := ytdlpProviderSource("generic", "https://artist.bandcamp.com/track/example"); source != "bandcamp" {
		t.Fatalf("generic Bandcamp source = %q, want bandcamp", source)
	}
	if source := ytdlpProviderSource("", "https://vk.com/audio-1"); source != "vk" {
		t.Fatalf("host-derived VK source = %q, want vk", source)
	}
}
