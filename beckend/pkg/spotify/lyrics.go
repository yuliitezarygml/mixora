package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/spotify/models"
)

const lrclibBaseURL = "https://lrclib.net/api"

var lrcTimestampRegex = regexp.MustCompile(`^\[(\d{2}):(\d{2})\.(\d{2,3})\](.*)$`)

type lrclibResult struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
	Instrumental bool    `json:"instrumental"`
	Duration     float64 `json:"duration"`
}

// GetTrackLyrics fetches synchronized and plain lyrics for a Spotify track.
func (c *Client) GetTrackLyrics(ctx context.Context, idOrURI string) (*models.Lyrics, error) {
	track, err := c.GetTrack(ctx, idOrURI)
	if err != nil {
		return nil, err
	}

	artistName := ""
	if len(track.Artists) > 0 {
		artistName = track.Artists[0].Name
	}
	durationSec := track.DurationMs / 1000

	// 1. Exact match via LRCLIB get
	lyrics, err := c.fetchLRCLIB(ctx, track.Title, artistName, durationSec)
	if err == nil && lyrics != nil {
		return lyrics, nil
	}

	// 2. Search fallback
	lyrics, err = c.searchLRCLIB(ctx, track.Title, artistName)
	if err == nil && lyrics != nil {
		return lyrics, nil
	}

	return nil, ErrNoLyrics
}

func (c *Client) fetchLRCLIB(ctx context.Context, trackName, artistName string, durationSec int64) (*models.Lyrics, error) {
	params := url.Values{}
	params.Set("track_name", trackName)
	if artistName != "" {
		params.Set("artist_name", artistName)
	}
	if durationSec > 0 {
		params.Set("duration", strconv.FormatInt(durationSec, 10))
	}

	reqURL := fmt.Sprintf("%s/get?%s", lrclibBaseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SoundCloud-Spotify-Go-Client/2.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrNoLyrics
	}

	var res lrclibResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return formatLyrics(&res), nil
}

func (c *Client) searchLRCLIB(ctx context.Context, trackName, artistName string) (*models.Lyrics, error) {
	params := url.Values{}
	params.Set("track_name", trackName)
	if artistName != "" {
		params.Set("artist_name", artistName)
	}

	reqURL := fmt.Sprintf("%s/search?%s", lrclibBaseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SoundCloud-Spotify-Go-Client/2.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrNoLyrics
	}

	var results []lrclibResult
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 {
		return nil, ErrNoLyrics
	}

	return formatLyrics(&results[0]), nil
}

func formatLyrics(res *lrclibResult) *models.Lyrics {
	if res.Instrumental {
		return &models.Lyrics{
			TrackName: res.TrackName,
			Artist:    res.ArtistName,
			Source:    "LRCLIB (Instrumental)",
			Plain:     "[Instrumental]",
			Synced:    false,
		}
	}

	l := &models.Lyrics{
		TrackName: res.TrackName,
		Artist:    res.ArtistName,
		Source:    "LRCLIB",
		Plain:     res.PlainLyrics,
	}

	if res.SyncedLyrics != "" {
		l.Synced = true
		lines := strings.Split(res.SyncedLyrics, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			m := lrcTimestampRegex.FindStringSubmatch(line)
			if len(m) == 5 {
				min, _ := strconv.ParseInt(m[1], 10, 64)
				sec, _ := strconv.ParseInt(m[2], 10, 64)
				msStr := m[3]
				if len(msStr) == 2 {
					msStr += "0"
				}
				ms, _ := strconv.ParseInt(msStr, 10, 64)
				totalMs := min*60*1000 + sec*1000 + ms
				text := strings.TrimSpace(m[4])
				l.Lines = append(l.Lines, models.LyricLine{
					TimeMs: totalMs,
					Text:   text,
				})
			}
		}
	}

	return l
}
