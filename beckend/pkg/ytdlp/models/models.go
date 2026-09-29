package models

// AudioQuality represents an audio format with quality tier and direct stream link.
type AudioQuality struct {
	FormatID   string  `json:"format_id"`
	Name       string  `json:"name"`        // e.g. "Opus 160 kbps (Studio Quality)", "AAC 128 kbps (HQ Audio)", "MP3 128 kbps"
	Codec      string  `json:"codec"`       // "opus", "aac", "mp3", "flac", "wav"
	Bitrate    float64 `json:"bitrate"`     // in kbps
	SampleRate int     `json:"sample_rate"` // in Hz (e.g. 48000, 44100)
	Extension  string  `json:"extension"`   // "webm", "m4a", "mp3", "flac", "wav"
	URL        string  `json:"url"`         // direct audio stream URL
	Filesize   int64   `json:"filesize,omitempty"`
}

// LyricLine represents a single lyric line with timing.
type LyricLine struct {
	TimeMs int64  `json:"time_ms"`
	Text   string `json:"text"`
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

// Format represents an available raw format stream from yt-dlp.
type Format struct {
	FormatID   string  `json:"format_id"`
	URL        string  `json:"url"`
	Ext        string  `json:"ext"`
	ACodec     string  `json:"acodec,omitempty"`
	VCodec     string  `json:"vcodec,omitempty"`
	ABR        float64 `json:"abr,omitempty"`
	ASR        int     `json:"asr,omitempty"`
	Filesize   int64   `json:"filesize,omitempty"`
	FormatNote string  `json:"format_note,omitempty"`
}

// MediaItem represents an extracted media track or episode with qualities and lyrics.
type MediaItem struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Artist             string         `json:"artist,omitempty"`
	Uploader           string         `json:"uploader,omitempty"`
	Album              string         `json:"album,omitempty"`
	Duration           float64        `json:"duration"` // in seconds
	Thumbnail          string         `json:"thumbnail,omitempty"`
	WebpageURL         string         `json:"webpage_url"`
	Extractor          string         `json:"extractor"` // "youtube", "vk", "bandcamp", etc.
	AudioURL           string         `json:"audio_url,omitempty"`
	AudioFormat        string         `json:"audio_format,omitempty"` // "opus", "mp3", "m4a", etc.
	Bitrate            float64        `json:"bitrate,omitempty"`
	Description        string         `json:"description,omitempty"`
	AvailableQualities []AudioQuality `json:"available_qualities,omitempty"`
	Lyrics             *Lyrics        `json:"lyrics,omitempty"`
	Formats            []Format       `json:"formats,omitempty"`
}

// SearchResult represents results from a search query (e.g. YouTube Music, YouTube).
type SearchResult struct {
	Query   string      `json:"query"`
	Service string      `json:"service"`
	Items   []MediaItem `json:"items"`
}

// PlaylistResult represents an extracted playlist or album (e.g. Bandcamp album, YouTube playlist).
type PlaylistResult struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Uploader    string      `json:"uploader,omitempty"`
	Extractor   string      `json:"extractor"`
	WebpageURL  string      `json:"webpage_url"`
	Thumbnail   string      `json:"thumbnail,omitempty"`
	TotalTracks int         `json:"total_tracks"`
	Entries     []MediaItem `json:"entries"`
}
