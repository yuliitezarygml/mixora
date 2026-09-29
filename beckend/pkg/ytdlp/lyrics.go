package ytdlp

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
	"time"

	"github.com/iulian/soundcloud-go/pkg/ytdlp/models"
)

const lrclibBaseURL = "https://lrclib.net/api"

var (
	lrcTimestampRegex = regexp.MustCompile(`^\[(\d{2}):(\d{2})\.(\d{2,3})\](.*)$`)
	cleanTitleRegex   = regexp.MustCompile(`(?i)\s*[\(\[](official\s*(music\s*)?video|audio|lyrics?|hd|hq|slowed|remix|mood\s*video|lyric\s*video|visualizer)[\)\]]`)
)

type lrclibResult struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
	Instrumental bool    `json:"instrumental"`
	Duration     float64 `json:"duration"`
}

// FetchLyrics attempts to fetch time-synced or plain lyrics for a track name and artist.
func FetchLyrics(ctx context.Context, trackTitle, artist string, durationSec int64) *models.Lyrics {
	cleanTitle := cleanTitleRegex.ReplaceAllString(trackTitle, "")
	cleanTitle = strings.TrimSpace(cleanTitle)

	// Split "Artist - Title" if present
	if strings.Contains(cleanTitle, " - ") {
		parts := strings.SplitN(cleanTitle, " - ", 2)
		if artist == "" || artist == parts[0] {
			artist = strings.TrimSpace(parts[0])
			cleanTitle = strings.TrimSpace(parts[1])
		}
	}

	// 1. Exact match via get
	if l := fetchLRCLIBGet(ctx, cleanTitle, artist, durationSec); l != nil {
		return l
	}

	// 2. Search fallback
	if l := searchLRCLIBSearch(ctx, cleanTitle, artist); l != nil {
		return l
	}

	return nil
}

func fetchLRCLIBGet(ctx context.Context, title, artist string, durationSec int64) *models.Lyrics {
	params := url.Values{}
	params.Set("track_name", title)
	if artist != "" {
		params.Set("artist_name", artist)
	}
	if durationSec > 0 {
		params.Set("duration", strconv.FormatInt(durationSec, 10))
	}

	reqURL := fmt.Sprintf("%s/get?%s", lrclibBaseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Universal-Music-Extractor/2.0")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	var res lrclibResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil
	}

	return formatLyrics(&res)
}

func searchLRCLIBSearch(ctx context.Context, title, artist string) *models.Lyrics {
	params := url.Values{}
	params.Set("track_name", title)
	if artist != "" {
		params.Set("artist_name", artist)
	}

	reqURL := fmt.Sprintf("%s/search?%s", lrclibBaseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Universal-Music-Extractor/2.0")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var results []lrclibResult
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 {
		return nil
	}

	return formatLyrics(&results[0])
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
