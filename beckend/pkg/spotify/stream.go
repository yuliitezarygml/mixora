package spotify

import (
	"context"
	"fmt"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/spotify/models"
)

// AudioQualityInfo describes an available audio format/quality tier in Spotify.
type AudioQualityInfo struct {
	Name      string `json:"name"`
	Codec     string `json:"codec"`
	Bitrate   string `json:"bitrate"`
	Available bool   `json:"available"`
	Type      string `json:"type"` // "direct_preview" or "spotify_connect"
	URL       string `json:"url,omitempty"`
}

// StreamInfo contains information about streaming options for a Spotify track.
type StreamInfo struct {
	TrackID            string             `json:"track_id"`
	URI                string             `json:"uri"`
	Title              string             `json:"title"`
	Artists            string             `json:"artists"`
	DurationMs         int64              `json:"duration_ms"`
	PreviewURL         string             `json:"preview_url,omitempty"`
	ConnectAvailable   bool               `json:"connect_available"`
	ConnectDevice      string             `json:"connect_device,omitempty"`
	AvailableQualities []AudioQualityInfo `json:"available_qualities"`
	ConnectStatus      *models.ConnectStatus `json:"connect_status,omitempty"`
}

// GetStream retrieves streaming options for a track (direct MP3 preview URL, Connect availability and audio qualities).
func (c *Client) GetStream(ctx context.Context, idOrURI string) (*StreamInfo, error) {
	track, err := c.GetTrack(ctx, idOrURI)
	if err != nil {
		return nil, fmt.Errorf("getting track for stream: %w", err)
	}

	artistNames := make([]string, len(track.Artists))
	for i, a := range track.Artists {
		artistNames[i] = a.Name
	}

	info := &StreamInfo{
		TrackID:    track.ID,
		URI:        track.URI,
		Title:      track.Title,
		Artists:    fmt.Sprintf("%v", artistNames),
		DurationMs: track.DurationMs,
		PreviewURL: track.PreviewURL,
	}

	// Check if Spotify Connect receiver is currently responding
	if c.connectClient != nil && c.connectClient.IsAvailable(ctx) {
		info.ConnectAvailable = true
		if status, err := c.connectClient.Status(ctx); err == nil {
			info.ConnectDevice = status.DeviceName
			info.ConnectStatus = status
		}
	}

	info.AvailableQualities = []AudioQualityInfo{
		{
			Name:      "MP3 96-160 kbps (Direct Preview)",
			Codec:     "mp3",
			Bitrate:   "160 kbps",
			Available: track.PreviewURL != "",
			Type:      "direct_preview",
			URL:       track.PreviewURL,
		},
		{
			Name:      "OGG Vorbis Very High (320 kbps)",
			Codec:     "vorbis",
			Bitrate:   "320 kbps",
			Available: info.ConnectAvailable,
			Type:      "spotify_connect",
		},
		{
			Name:      "OGG Vorbis High (160 kbps)",
			Codec:     "vorbis",
			Bitrate:   "160 kbps",
			Available: info.ConnectAvailable,
			Type:      "spotify_connect",
		},
		{
			Name:      "OGG Vorbis Normal (96 kbps)",
			Codec:     "vorbis",
			Bitrate:   "96 kbps",
			Available: info.ConnectAvailable,
			Type:      "spotify_connect",
		},
		{
			Name:      "FLAC HiFi (Lossless Audio)",
			Codec:     "flac",
			Bitrate:   "Lossless",
			Available: info.ConnectAvailable,
			Type:      "spotify_connect",
		},
	}

	return info, nil
}

// PlayOnConnect loads and starts playing the track on the active Spotify Connect daemon.
func (c *Client) PlayOnConnect(ctx context.Context, idOrURI string) error {
	parsed, err := ParseURL(idOrURI)
	uri := idOrURI
	if err == nil {
		uri = parsed.URI
	} else if !strings.HasPrefix(uri, "spotify:") {
		uri = "spotify:track:" + uri
	}

	if c.connectClient == nil || !c.connectClient.IsAvailable(ctx) {
		return ErrConnectUnavailable
	}

	return c.connectClient.Load(ctx, uri, true)
}
