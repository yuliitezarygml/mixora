package ytdlp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/ytdlp/models"
)

type rawItem struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Uploader    string  `json:"uploader"`
	Album       string  `json:"album"`
	Duration    float64 `json:"duration"`
	Thumbnail   string  `json:"thumbnail"`
	WebpageURL  string  `json:"webpage_url"`
	Extractor   string  `json:"extractor"`
	URL         string  `json:"url"`
	Ext         string  `json:"ext"`
	ACodec      string  `json:"acodec"`
	ABR         float64 `json:"abr"`
	ASR         int     `json:"asr"`
	Description string  `json:"description"`
	Formats     []struct {
		FormatID   string  `json:"format_id"`
		URL        string  `json:"url"`
		Ext        string  `json:"ext"`
		ACodec     string  `json:"acodec"`
		VCodec     string  `json:"vcodec"`
		ABR        float64 `json:"abr"`
		ASR        int     `json:"asr"`
		Filesize   int64   `json:"filesize"`
		FormatNote string  `json:"format_note"`
	} `json:"formats"`
}

type rawPlaylist struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Uploader   string    `json:"uploader"`
	Extractor  string    `json:"extractor"`
	WebpageURL string    `json:"webpage_url"`
	Entries    []rawItem `json:"entries"`
}

// Extract extracts full media metadata, all available audio qualities, and synchronized lyrics.
func (c *Client) Extract(ctx context.Context, mediaURL string) (*models.MediaItem, error) {
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" {
		return nil, ErrUnsupportedURL
	}

	out, err := c.runCommand(ctx, "-J", "--no-playlist", "-f", "ba/b", mediaURL)
	if err != nil {
		return nil, err
	}

	var raw rawItem
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing yt-dlp JSON: %w", err)
	}

	item := mapRawToMediaItem(&raw)

	// Fetch lyrics automatically
	if item.Title != "" {
		durSec := int64(item.Duration)
		item.Lyrics = FetchLyrics(ctx, item.Title, item.Artist, durSec)
	}

	return item, nil
}

// ExtractAudioURL extracts direct playable CDN audio URL for the given resource.
func (c *Client) ExtractAudioURL(ctx context.Context, mediaURL string) (string, error) {
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" {
		return "", ErrUnsupportedURL
	}

	out, err := c.runCommand(ctx, "-f", "ba/b", "-g", mediaURL)
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", fmt.Errorf("no direct audio URL returned")
	}

	return strings.TrimSpace(lines[0]), nil
}

// ExtractPlaylist extracts playlist or album tracks (e.g. Bandcamp album, YouTube playlist).
func (c *Client) ExtractPlaylist(ctx context.Context, playlistURL string) (*models.PlaylistResult, error) {
	playlistURL = strings.TrimSpace(playlistURL)
	if playlistURL == "" {
		return nil, ErrUnsupportedURL
	}

	out, err := c.runCommand(ctx, "-J", "--flat-playlist", playlistURL)
	if err != nil {
		return nil, err
	}

	var raw rawPlaylist
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing yt-dlp playlist JSON: %w", err)
	}

	res := &models.PlaylistResult{
		ID:          raw.ID,
		Title:       raw.Title,
		Uploader:    raw.Uploader,
		Extractor:   raw.Extractor,
		WebpageURL:  raw.WebpageURL,
		TotalTracks: len(raw.Entries),
	}

	for _, entry := range raw.Entries {
		res.Entries = append(res.Entries, *mapRawToMediaItem(&entry))
	}

	return res, nil
}

func mapRawToMediaItem(r *rawItem) *models.MediaItem {
	item := &models.MediaItem{
		ID:          r.ID,
		Title:       r.Title,
		Artist:      r.Artist,
		Uploader:    r.Uploader,
		Album:       r.Album,
		Duration:    r.Duration,
		Thumbnail:   r.Thumbnail,
		WebpageURL:  r.WebpageURL,
		Extractor:   r.Extractor,
		AudioURL:    r.URL,
		AudioFormat: r.Ext,
		Bitrate:     r.ABR,
		Description: r.Description,
	}

	if item.Artist == "" {
		item.Artist = r.Uploader
	}

	var qualities []models.AudioQuality

	for _, f := range r.Formats {
		item.Formats = append(item.Formats, models.Format{
			FormatID:   f.FormatID,
			URL:        f.URL,
			Ext:        f.Ext,
			ACodec:     f.ACodec,
			VCodec:     f.VCodec,
			ABR:        f.ABR,
			ASR:        f.ASR,
			Filesize:   f.Filesize,
			FormatNote: f.FormatNote,
		})

		// Check if it's an audio stream
		isAudioOnly := f.VCodec == "none" && f.ACodec != "none"
		if isAudioOnly && f.URL != "" {
			codec, name := identifyAudioQuality(f.ACodec, f.Ext, f.ABR, f.ASR)
			qualities = append(qualities, models.AudioQuality{
				FormatID:   f.FormatID,
				Name:       name,
				Codec:      codec,
				Bitrate:    f.ABR,
				SampleRate: f.ASR,
				Extension:  f.Ext,
				URL:        f.URL,
				Filesize:   f.Filesize,
			})

			// Set best default audio URL if not present
			if item.AudioURL == "" || item.AudioURL == item.WebpageURL {
				item.AudioURL = f.URL
				item.AudioFormat = f.Ext
				item.Bitrate = f.ABR
			}
		}
	}

	// Sort qualities by bitrate descending (highest quality first)
	sort.Slice(qualities, func(i, j int) bool {
		return qualities[i].Bitrate > qualities[j].Bitrate
	})
	item.AvailableQualities = qualities

	// If we found qualities and best is at index 0, point default AudioURL to highest quality
	if len(qualities) > 0 && qualities[0].URL != "" {
		item.AudioURL = qualities[0].URL
		item.AudioFormat = qualities[0].Extension
		item.Bitrate = qualities[0].Bitrate
	}

	return item
}

func identifyAudioQuality(acodec, ext string, abr float64, asr int) (string, string) {
	codec := strings.ToLower(acodec)
	ext = strings.ToLower(ext)

	switch {
	case strings.Contains(codec, "flac") || ext == "flac":
		return "flac", "FLAC (Lossless Master)"
	case strings.Contains(codec, "wav") || ext == "wav":
		return "wav", "WAV (Uncompressed PCM)"
	case strings.Contains(codec, "opus") || ext == "opus" || (ext == "webm" && strings.Contains(codec, "opus")):
		if abr >= 120 {
			return "opus", fmt.Sprintf("Opus %.0f kbps (Studio High Quality)", abr)
		}
		return "opus", fmt.Sprintf("Opus %.0f kbps", abr)
	case strings.Contains(codec, "mp4a") || strings.Contains(codec, "aac") || ext == "m4a" || ext == "aac":
		if abr >= 120 {
			return "aac", fmt.Sprintf("AAC %.0f kbps (High Quality)", abr)
		}
		return "aac", fmt.Sprintf("AAC %.0f kbps", abr)
	case strings.Contains(codec, "mp3") || ext == "mp3":
		if abr >= 300 {
			return "mp3", fmt.Sprintf("MP3 %.0f kbps (320k HQ)", abr)
		}
		return "mp3", fmt.Sprintf("MP3 %.0f kbps", abr)
	default:
		if abr > 0 {
			return ext, fmt.Sprintf("%s %.0f kbps", strings.ToUpper(ext), abr)
		}
		return ext, strings.ToUpper(ext)
	}
}
