package models

// Lyrics represents the lyrics of a track.
type Lyrics struct {
	TrackID      int64  `json:"track_id,omitempty"`
	TrackTitle   string `json:"track_title"`
	ArtistName   string `json:"artist_name"`
	PlainLyrics  string `json:"plain_lyrics,omitempty"`
	SyncedLyrics string `json:"synced_lyrics,omitempty"` // LRC format with timestamps [mm:ss.xx]
	Instrumental bool   `json:"instrumental"`
	Source       string `json:"source"` // "lrclib" or "description"
}
