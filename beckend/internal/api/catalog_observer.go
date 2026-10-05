package api

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/iulian/soundcloud-go/internal/music"
	soundcloudmodels "github.com/iulian/soundcloud-go/pkg/soundcloud/models"
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

func (h *Handler) observeSoundCloudTrack(ctx context.Context, track soundcloudmodels.Track) {
	h.observeTracks(ctx, []music.Track{music.FromSoundCloud(track)})
}

func (h *Handler) observeSoundCloudTracks(ctx context.Context, tracks []soundcloudmodels.Track) {
	observed := make([]music.Track, 0, len(tracks))
	for _, track := range tracks {
		observed = append(observed, music.FromSoundCloud(track))
	}
	h.observeTracks(ctx, observed)
}

func (h *Handler) observeSoundCloudPlaylist(ctx context.Context, playlist soundcloudmodels.Playlist) {
	h.observeSoundCloudTracks(ctx, playlist.Tracks)
}

func (h *Handler) observeSoundCloudChart(ctx context.Context, chart *soundcloudmodels.ChartResponse) {
	if chart == nil {
		return
	}
	tracks := make([]soundcloudmodels.Track, 0, len(chart.Collection))
	for _, item := range chart.Collection {
		tracks = append(tracks, item.Track)
	}
	h.observeSoundCloudTracks(ctx, tracks)
}

func (h *Handler) observeSoundCloudSearchItems(ctx context.Context, items []soundcloudmodels.SearchItem) {
	tracks := make([]soundcloudmodels.Track, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Kind) != "track" {
			continue
		}
		track := soundcloudmodels.Track{
			ID:           item.ID,
			Kind:         item.Kind,
			Permalink:    item.Permalink,
			PermalinkURL: item.PermalinkURL,
			Title:        item.Title,
			Description:  item.Description,
			Duration:     item.Duration,
			ArtworkURL:   item.ArtworkURL,
			User:         item.User,
			Streamable:   item.Streamable,
			Media:        soundcloudmodels.Media{},
		}
		if item.Media != nil {
			track.Media = *item.Media
		}
		tracks = append(tracks, track)
	}
	h.observeSoundCloudTracks(ctx, tracks)
}

// observeSoundCloudResolve keeps the public raw resolve response intact while
// cataloging any provider-verified track(s) it contains. A malformed optional
// observer decode is ignored, just like an unavailable catalog write.
func (h *Handler) observeSoundCloudResolve(ctx context.Context, raw json.RawMessage) {
	var envelope struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return
	}
	switch strings.TrimSpace(envelope.Kind) {
	case "track":
		var track soundcloudmodels.Track
		if json.Unmarshal(raw, &track) == nil {
			h.observeSoundCloudTrack(ctx, track)
		}
	case "playlist", "system-playlist":
		var playlist soundcloudmodels.Playlist
		if json.Unmarshal(raw, &playlist) == nil {
			h.observeSoundCloudPlaylist(ctx, playlist)
		}
	}
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
