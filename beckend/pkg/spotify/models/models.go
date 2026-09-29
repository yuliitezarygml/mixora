package models

// Image represents an album or artist image.
type Image struct {
	URL    string `json:"url"`
	Height int    `json:"height,omitempty"`
	Width  int    `json:"width,omitempty"`
}

// Artist represents a Spotify artist summary or detail.
type Artist struct {
	ID           string   `json:"id"`
	URI          string   `json:"uri"`
	Name         string   `json:"name"`
	Genres       []string `json:"genres,omitempty"`
	Images       []Image  `json:"images,omitempty"`
	Popularity   int      `json:"popularity,omitempty"`
	ExternalURL  string   `json:"external_url,omitempty"`
	Followers    int      `json:"followers,omitempty"`
	TopTracks    []Track  `json:"top_tracks,omitempty"`
}

// Album represents a Spotify album summary or detail.
type Album struct {
	ID          string   `json:"id"`
	URI         string   `json:"uri"`
	Name        string   `json:"name"`
	AlbumType   string   `json:"album_type,omitempty"`
	ReleaseDate string   `json:"release_date,omitempty"`
	TotalTracks int      `json:"total_tracks,omitempty"`
	Images      []Image  `json:"images,omitempty"`
	Artists     []Artist `json:"artists,omitempty"`
	Tracks      []Track  `json:"tracks,omitempty"`
	ExternalURL string   `json:"external_url,omitempty"`
}

// Track represents a Spotify track with metadata and playback links.
type Track struct {
	ID          string   `json:"id"`
	URI         string   `json:"uri"`
	Title       string   `json:"title"`
	Artists     []Artist `json:"artists"`
	Album       *Album   `json:"album,omitempty"`
	DurationMs  int64    `json:"duration_ms"`
	Explicit    bool     `json:"explicit"`
	Popularity  int      `json:"popularity,omitempty"`
	PreviewURL  string   `json:"preview_url,omitempty"`
	ExternalURL string   `json:"external_url,omitempty"`
	IsPlayable  bool     `json:"is_playable"`
	TrackNumber int      `json:"track_number,omitempty"`
	DiscNumber  int      `json:"disc_number,omitempty"`
}

// Playlist represents a Spotify playlist.
type Playlist struct {
	ID          string   `json:"id"`
	URI         string   `json:"uri"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Images      []Image  `json:"images,omitempty"`
	TotalTracks int      `json:"total_tracks,omitempty"`
	Tracks      []Track  `json:"tracks,omitempty"`
	ExternalURL string   `json:"external_url,omitempty"`
}

// SearchResult represents search results across tracks, albums, artists, playlists.
type SearchResult struct {
	Query     string     `json:"query"`
	Tracks    []Track    `json:"tracks,omitempty"`
	Artists   []Artist   `json:"artists,omitempty"`
	Albums    []Album    `json:"albums,omitempty"`
	Playlists []Playlist `json:"playlists,omitempty"`
}

// Lyrics represents synchronized and plain lyrics.
type Lyrics struct {
	TrackName string      `json:"track_name"`
	Artist    string      `json:"artist"`
	Source    string      `json:"source"`
	Synced    bool        `json:"synced"`
	Lines     []LyricLine `json:"lines,omitempty"`
	Plain     string      `json:"plain,omitempty"`
}

// LyricLine represents a single lyric line with timing.
type LyricLine struct {
	TimeMs int64  `json:"time_ms"`
	Text   string `json:"text"`
}

// ConnectStatus represents the current playback status of the Spotify Connect device (librespot).
type ConnectStatus struct {
	Username           string        `json:"username"`
	DeviceID           string        `json:"device_id"`
	DeviceType         string        `json:"device_type"`
	DeviceName         string        `json:"device_name"`
	PlayOrigin         string        `json:"play_origin,omitempty"`
	PlayOriginDeviceID string        `json:"play_origin_device_id,omitempty"`
	ContextURI         string        `json:"context_uri,omitempty"`
	ContextName        string        `json:"context_name,omitempty"`
	Stopped            bool          `json:"stopped"`
	Paused             bool          `json:"paused"`
	Buffering          bool          `json:"buffering"`
	Volume             int           `json:"volume"`
	VolumeSteps        int           `json:"volume_steps"`
	RepeatContext      bool          `json:"repeat_context"`
	RepeatTrack        bool          `json:"repeat_track"`
	ShuffleContext     bool          `json:"shuffle_context"`
	Track              *ConnectTrack `json:"track,omitempty"`
}

// ConnectTrack represents track metadata from the Spotify Connect daemon.
type ConnectTrack struct {
	URI           string   `json:"uri"`
	Name          string   `json:"name"`
	ArtistNames   []string `json:"artist_names"`
	ArtistURIs    []string `json:"artist_uris,omitempty"`
	AlbumName     string   `json:"album_name"`
	AlbumURI      string   `json:"album_uri,omitempty"`
	AlbumCoverURL string   `json:"album_cover_url,omitempty"`
	Position      int64    `json:"position"`
	Duration      int64    `json:"duration"`
	ReleaseDate   string   `json:"release_date,omitempty"`
	TrackNumber   int      `json:"track_number,omitempty"`
	DiscNumber    int      `json:"disc_number,omitempty"`
	Format        string   `json:"format,omitempty"`
	Codec         string   `json:"codec,omitempty"`
	Bitrate       *int     `json:"bitrate,omitempty"`
	SampleRate    *int     `json:"sample_rate,omitempty"`
	BitDepth      *int     `json:"bit_depth,omitempty"`
}

// ConnectEvent represents an event received from the Spotify Connect WebSocket stream.
type ConnectEvent struct {
	Type     string        `json:"type"` // "active", "inactive", "metadata", "playing", "paused", "volume", etc.
	Track    *ConnectTrack `json:"track,omitempty"`
	Position int64         `json:"position,omitempty"`
	Volume   int           `json:"volume,omitempty"`
	Raw      []byte        `json:"raw,omitempty"`
}
