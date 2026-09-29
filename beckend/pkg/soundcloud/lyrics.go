package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

const lrclibBaseURL = "https://lrclib.net/api"

var (
	junkTitleRegex = regexp.MustCompile(`(?i)\s*[\(\[](official\s*(music\s*)?video|audio|lyrics?|hd|hq|slowed(\s*\+\s*reverb)?|remix|edit)[\)\]]`)
)

type lrclibResponse struct {
	ID           int64   `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
	Instrumental bool    `json:"instrumental"`
	Duration     float64 `json:"duration"`
}

// GetTrackLyrics fetches lyrics for a track.
// It first attempts to retrieve time-synced / plain lyrics from the public lyrics database (LRCLIB),
// and falls back to checking the track's SoundCloud description.
func (c *Client) GetTrackLyrics(ctx context.Context, track *models.Track) (*models.Lyrics, error) {
	artist, title := parseArtistAndTitle(track)

	// 1. Try LRCLIB exact get
	lyrics, err := c.fetchLRCLIB(ctx, artist, title, track.Duration/1000)
	if err == nil && lyrics != nil {
		lyrics.TrackID = track.ID
		return lyrics, nil
	}

	// 2. Try LRCLIB search fallback with artist + title
	lyrics, err = c.searchLRCLIB(ctx, title, artist)
	if err == nil && lyrics != nil {
		lyrics.TrackID = track.ID
		return lyrics, nil
	}

	// 3. Try LRCLIB search fallback with track title alone
	if title != track.Title {
		lyrics, err = c.searchLRCLIB(ctx, track.Title, "")
		if err == nil && lyrics != nil {
			lyrics.TrackID = track.ID
			return lyrics, nil
		}
	}

	// 4. Fallback: check if the artist provided lyrics in the track description
	desc := strings.TrimSpace(track.Description)
	if isLikelyLyrics(desc) {
		return &models.Lyrics{
			TrackID:     track.ID,
			TrackTitle:  track.Title,
			ArtistName:  artist,
			PlainLyrics: desc,
			Source:      "description",
		}, nil
	}

	return nil, fmt.Errorf("soundcloud: lyrics not found for track %q by %q", title, artist)
}

func (c *Client) fetchLRCLIB(ctx context.Context, artist, title string, durationSec int) (*models.Lyrics, error) {
	params := url.Values{
		"track_name":  {title},
		"artist_name": {artist},
	}
	if durationSec > 0 {
		params.Set("duration", fmt.Sprintf("%d", durationSec))
	}

	endpoint := lrclibBaseURL + "/get?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SoundCloud-Go-Client/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lrclib status: %d", resp.StatusCode)
	}

	var res lrclibResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if res.PlainLyrics == "" && res.SyncedLyrics == "" && !res.Instrumental {
		return nil, fmt.Errorf("empty lyrics")
	}

	return &models.Lyrics{
		TrackTitle:   res.TrackName,
		ArtistName:   res.ArtistName,
		PlainLyrics:  res.PlainLyrics,
		SyncedLyrics: res.SyncedLyrics,
		Instrumental: res.Instrumental,
		Source:       "lrclib",
	}, nil
}

func (c *Client) searchLRCLIB(ctx context.Context, title, artist string) (*models.Lyrics, error) {
	params := url.Values{
		"q": {artist + " " + title},
	}

	endpoint := lrclibBaseURL + "/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SoundCloud-Go-Client/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lrclib search status: %d: %s", resp.StatusCode, string(body))
	}

	var list []lrclibResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	if len(list) == 0 {
		return nil, fmt.Errorf("no search results")
	}

	first := list[0]
	return &models.Lyrics{
		TrackTitle:   first.TrackName,
		ArtistName:   first.ArtistName,
		PlainLyrics:  first.PlainLyrics,
		SyncedLyrics: first.SyncedLyrics,
		Instrumental: first.Instrumental,
		Source:       "lrclib",
	}, nil
}

// parseArtistAndTitle extracts clean artist and title from SoundCloud track title and metadata.
func parseArtistAndTitle(track *models.Track) (artist string, title string) {
	if track.PublisherMetadata != nil && track.PublisherMetadata.Artist != "" {
		artist = track.PublisherMetadata.Artist
	}
	if artist == "" && track.User != nil {
		artist = track.User.Username
	}

	rawTitle := track.Title

	// Handle "Artist - Track" in title
	if strings.Contains(rawTitle, " - ") {
		parts := strings.SplitN(rawTitle, " - ", 2)
		if len(parts) == 2 {
			artist = strings.TrimSpace(parts[0])
			rawTitle = parts[1]
		}
	}

	// Strip suffixes like (Official Music Video), [slowed], etc.
	cleanedTitle := junkTitleRegex.ReplaceAllString(rawTitle, "")
	cleanedTitle = strings.TrimFunc(cleanedTitle, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && (r < 'а' || r > 'я') && (r < 'А' || r > 'Я')
	})
	cleanedTitle = strings.TrimSpace(cleanedTitle)

	if cleanedTitle == "" {
		cleanedTitle = track.Title
	}

	return strings.TrimSpace(artist), cleanedTitle
}

func isLikelyLyrics(text string) bool {
	if len(text) < 100 {
		return false
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "lyrics") ||
		strings.Contains(lower, "текст песни") ||
		strings.Contains(lower, "verse 1") ||
		strings.Contains(lower, "chorus") ||
		strings.Contains(lower, "куплет") ||
		strings.Contains(lower, "припев")
}
