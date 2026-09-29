package models

import "time"

// Playlist represents a playlist, album, or compilation.
type Playlist struct {
	ID             int64      `json:"id"`
	Kind           string     `json:"kind"`
	CreatedAt      time.Time  `json:"created_at"`
	Duration       int        `json:"duration"`
	Permalink      string     `json:"permalink"`
	PermalinkURL   string     `json:"permalink_url"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	Genre          string     `json:"genre,omitempty"`
	TagList        string     `json:"tag_list,omitempty"`
	URI            string     `json:"uri"`
	ArtworkURL     string     `json:"artwork_url,omitempty"`
	LikesCount     int        `json:"likes_count"`
	RepostCount    int        `json:"reposts_count"`
	IsAlbum        bool       `json:"is_album"`
	SetType        string     `json:"set_type"`
	TrackCount     int        `json:"track_count"`
	User           *User      `json:"user,omitempty"`
	UserID         int64      `json:"user_id"`
	Tracks         []Track    `json:"tracks,omitempty"`
	ManagedByFeeds bool       `json:"managed_by_feeds"`
	Sharing        string     `json:"sharing"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
}
