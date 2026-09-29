package models

import (
	"strings"
	"time"
)

// Format describes audio protocol and mime type.
type Format struct {
	Protocol string `json:"protocol"` // "hls" or "progressive"
	MimeType string `json:"mime_type"`
}

// Transcoding contains audio stream access information.
type Transcoding struct {
	URL      string `json:"url"`
	Preset   string `json:"preset"`
	Duration int    `json:"duration"`
	Snipped  bool   `json:"snipped"`
	Format   Format `json:"format"`
	Quality  string `json:"quality"`
}

// Media contains all available transcodings for a track.
type Media struct {
	Transcodings []Transcoding `json:"transcodings"`
}

// PublisherMetadata contains copyright and publishing info.
type PublisherMetadata struct {
	ID             int64  `json:"id"`
	URN            string `json:"urn"`
	Artist         string `json:"artist,omitempty"`
	AlbumTitle     string `json:"album_title,omitempty"`
	ContainsMusic  bool   `json:"contains_music"`
	ISRC           string `json:"isrc,omitempty"`
	Publisher      string `json:"publisher,omitempty"`
	WriterComposer string `json:"writer_composer,omitempty"`
}

// Track represents a SoundCloud track entity.
type Track struct {
	ID                int64              `json:"id"`
	Kind              string             `json:"kind"`
	CreatedAt         time.Time          `json:"created_at"`
	Duration          int                `json:"duration"` // in milliseconds
	Permalink         string             `json:"permalink"`
	PermalinkURL      string             `json:"permalink_url"`
	Title             string             `json:"title"`
	Description       string             `json:"description,omitempty"`
	Genre             string             `json:"genre,omitempty"`
	TagList           string             `json:"tag_list,omitempty"`
	LabelName         string             `json:"label_name,omitempty"`
	URI               string             `json:"uri"`
	CommentCount      int                `json:"comment_count"`
	LikesCount        int                `json:"likes_count"`
	PlaybackCount     int                `json:"playback_count"`
	RepostCount       int                `json:"reposts_count"`
	DownloadCount     int                `json:"download_count"`
	Downloadable      bool               `json:"downloadable"`
	Streamable        bool               `json:"streamable"`
	ArtworkURL        string             `json:"artwork_url,omitempty"`
	WaveformURL       string             `json:"waveform_url,omitempty"`
	State             string             `json:"state"`
	Sharing           string             `json:"sharing"`
	User              *User              `json:"user,omitempty"`
	UserID            int64              `json:"user_id"`
	Media             Media              `json:"media"`
	PublisherMetadata *PublisherMetadata `json:"publisher_metadata,omitempty"`
	FullDurationMS    int                `json:"full_duration"`
	HasDownloadsLeft  bool               `json:"has_downloads_left"`
	Policy            string             `json:"policy,omitempty"`
	MonetizationModel string             `json:"monetization_model,omitempty"`
}

// ArtworkURLSize returns the artwork URL resized to the requested size.
// Supported sizes: "t500x500", "t300x300", "crop", "original", "large", "badge", "small", "tiny", "mini".
func (t *Track) ArtworkURLSize(size string) string {
	if t.ArtworkURL == "" {
		return ""
	}
	return replaceImageSize(t.ArtworkURL, size)
}

func replaceImageSize(imgURL, size string) string {
	const defaultPattern = "-large."
	idx := strings.LastIndex(imgURL, defaultPattern)
	if idx == -1 {
		return imgURL
	}
	return imgURL[:idx] + "-" + size + imgURL[idx+len(defaultPattern)-1:]
}
