package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/iulian/soundcloud-go/internal/music"
	soundcloudmodels "github.com/iulian/soundcloud-go/pkg/soundcloud/models"
	spotifymodels "github.com/iulian/soundcloud-go/pkg/spotify/models"
	ytdlpmodels "github.com/iulian/soundcloud-go/pkg/ytdlp/models"
)

type recordingTrackObserver struct {
	calls [][]music.Track
}

func (o *recordingTrackObserver) Save(_ context.Context, tracks []music.Track) error {
	o.calls = append(o.calls, append([]music.Track(nil), tracks...))
	return nil
}

func TestProviderCatalogObserverConvertsSpotifyAndYTDLPResults(t *testing.T) {
	observer := &recordingTrackObserver{}
	handler := &Handler{}
	handler.SetTrackObserver(observer)

	handler.observeSpotifyTracks(context.Background(), []spotifymodels.Track{{
		ID: "spotify-1", Title: "Spotify track", IsPlayable: true,
		Artists: []spotifymodels.Artist{{ID: "artist-1", Name: "Artist"}},
	}})
	handler.observeYTDLPItems(context.Background(), []ytdlpmodels.MediaItem{{
		ID: "youtube-1", Title: "YouTube track", Artist: "Uploader", Extractor: "youtube:search",
		WebpageURL: "https://www.youtube.com/watch?v=youtube-1",
	}})

	if len(observer.calls) != 2 {
		t.Fatalf("observer calls = %d, want 2", len(observer.calls))
	}
	if got := observer.calls[0]; len(got) != 1 || got[0].Source != "spotify" || got[0].ID != "spotify-1" {
		t.Fatalf("Spotify observation = %#v", got)
	}
	if got := observer.calls[1]; len(got) != 1 || got[0].Source != "youtube" || got[0].ID != "youtube-1" {
		t.Fatalf("yt-dlp observation = %#v", got)
	}
}

func TestCatalogObserverSkipsUnidentifiableTracksAndDeduplicates(t *testing.T) {
	observer := &recordingTrackObserver{}
	handler := &Handler{}
	handler.SetTrackObserver(observer)
	handler.observeTracks(context.Background(), []music.Track{
		{Source: "spotify", ID: "same"},
		{Source: "spotify", ID: "same"},
		{Source: "youtube"},
	})
	if len(observer.calls) != 1 || len(observer.calls[0]) != 1 {
		t.Fatalf("observations = %#v", observer.calls)
	}
}

func TestSoundCloudCatalogObserverCoversTrackBearingResponses(t *testing.T) {
	observer := &recordingTrackObserver{}
	handler := &Handler{}
	handler.SetTrackObserver(observer)

	handler.observeSoundCloudTrack(context.Background(), soundcloudmodels.Track{ID: 1, Title: "Direct", Streamable: true})
	handler.observeSoundCloudTracks(context.Background(), []soundcloudmodels.Track{{ID: 2, Title: "Related", Streamable: true}})
	handler.observeSoundCloudPlaylist(context.Background(), soundcloudmodels.Playlist{Tracks: []soundcloudmodels.Track{{ID: 3, Title: "Playlist", Streamable: true}}})
	handler.observeSoundCloudChart(context.Background(), &soundcloudmodels.ChartResponse{Collection: []soundcloudmodels.ChartItem{{Track: soundcloudmodels.Track{ID: 4, Title: "Chart", Streamable: true}}}})
	handler.observeSoundCloudSearchItems(context.Background(), []soundcloudmodels.SearchItem{{Kind: "track", ID: 5, Title: "Search", Streamable: true}})
	raw, err := json.Marshal(soundcloudmodels.Track{ID: 6, Kind: "track", Title: "Resolve", Streamable: true})
	if err != nil {
		t.Fatal(err)
	}
	handler.observeSoundCloudResolve(context.Background(), raw)

	if len(observer.calls) != 6 {
		t.Fatalf("observer calls = %d, want 6", len(observer.calls))
	}
	for index, wantID := range []string{"1", "2", "3", "4", "5", "6"} {
		got := observer.calls[index]
		if len(got) != 1 || got[0].Source != "soundcloud" || got[0].ID != wantID {
			t.Fatalf("observation[%d] = %#v, want SoundCloud %s", index, got, wantID)
		}
	}
}
