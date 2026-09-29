package models

import "time"

// SearchResult wraps a collection of polymorphic search results.
type SearchResult struct {
	Collection   []SearchItem `json:"collection"`
	NextHref     string       `json:"next_href,omitempty"`
	TotalResults int          `json:"total_results,omitempty"`
}

// SearchItem represents an item returned in a general search (track, user, or playlist).
type SearchItem struct {
	Kind         string `json:"kind"` // "track", "user", "playlist"
	ID           int64  `json:"id"`
	Permalink    string `json:"permalink"`
	PermalinkURL string `json:"permalink_url"`
	URI          string `json:"uri"`
	Title        string `json:"title,omitempty"`    // for tracks and playlists
	Username     string `json:"username,omitempty"` // for users
	ArtworkURL   string `json:"artwork_url,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	Description  string `json:"description,omitempty"`
	Duration     int    `json:"duration,omitempty"`
	User         *User  `json:"user,omitempty"`

	// Track-specific fields
	Media         *Media `json:"media,omitempty"`
	Streamable    bool   `json:"streamable,omitempty"`
	PlaybackCount int    `json:"playback_count,omitempty"`
	LikesCount    int    `json:"likes_count,omitempty"`

	// Playlist-specific fields
	TrackCount int  `json:"track_count,omitempty"`
	IsAlbum    bool `json:"is_album,omitempty"`
}

// Like represents a like item for tracks or playlists.
type Like struct {
	CreatedAt time.Time `json:"created_at"`
	Kind      string    `json:"kind"`
	Track     *Track    `json:"track,omitempty"`
	Playlist  *Playlist `json:"playlist,omitempty"`
}
