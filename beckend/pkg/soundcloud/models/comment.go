package models

import "time"

// Comment represents a timed comment left on a track.
type Comment struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Timestamp int       `json:"timestamp"` // playback position in milliseconds
	TrackID   int64     `json:"track_id"`
	UserID    int64     `json:"user_id"`
	User      *User     `json:"user,omitempty"`
}
