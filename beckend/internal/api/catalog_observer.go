package api

import (
	"context"
	"strings"

	"github.com/iulian/soundcloud-go/internal/music"
	spotifymodels "github.com/iulian/soundcloud-go/pkg/spotify/models"
	ytdlpmodels "github.com/iulian/soundcloud-go/pkg/ytdlp/models"
)

// observeTracks is deliberately best-effort: catalog indexing enriches future
// recommendations but never changes an already successful music-engine read.
func (h *Handler) observeTracks(ctx context.Context, tracks []music.Track) {
	if h == nil || h.tracks == nil || len(tracks) == 0 {
		return
	}
	type key struct{ source, id string }
	seen := make(map[key]struct{}, len(tracks))
	observed := make([]music.Track, 0, len(tracks))
	for _, track := range tracks {
		track = music.CanonicalTrack(track)
		track.Source = strings.TrimSpace(track.Source)
		track.ID = strings.TrimSpace(track.ID)
		if track.Source == "" || track.ID == "" {
			continue
		}
		identity := key{source: track.Source, id: track.ID}
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		observed = append(observed, track)
	}
	if len(observed) > 0 {
		_ = h.tracks.Save(ctx, observed)
	}
}

func (h *Handler) observeSpotifyTrack(ctx context.Context, track spotifymodels.Track) {
	h.observeTracks(ctx, []music.Track{music.FromSpotify(track)})
}

func (h *Handler) observeSpotifyTracks(ctx context.Context, tracks []spotifymodels.Track) {
	observed := make([]music.Track, 0, len(tracks))
	for _, track := range tracks {
		observed = append(observed, music.FromSpotify(track))
	}
	h.observeTracks(ctx, observed)
}

func (h *Handler) observeYTDLPItem(ctx context.Context, item ytdlpmodels.MediaItem) {
	h.observeTracks(ctx, []music.Track{music.FromYTDLP(item)})
}

func (h *Handler) observeYTDLPItems(ctx context.Context, items []ytdlpmodels.MediaItem) {
	observed := make([]music.Track, 0, len(items))
	for _, item := range items {
		observed = append(observed, music.FromYTDLP(item))
	}
	h.observeTracks(ctx, observed)
}
