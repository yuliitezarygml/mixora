package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

// GetStreamURL calls the transcoding endpoint to retrieve the actual direct playable media URL.
// The transcodingURL comes from track.Media.Transcodings[i].URL.
func (c *Client) GetStreamURL(ctx context.Context, transcodingURL string) (string, error) {
	c.mu.RLock()
	clientID := c.clientID
	authToken := c.authToken
	c.mu.RUnlock()

	params := url.Values{"client_id": {clientID}}
	if authToken != "" {
		params.Set("track_authorization", authToken)
	}

	targetURL := transcodingURL
	if strings.Contains(targetURL, "?") {
		targetURL += "&" + params.Encode()
	} else {
		targetURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "OAuth "+authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("soundcloud: stream error HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res models.StreamURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("soundcloud: failed to parse stream url response: %w", err)
	}

	return res.URL, nil
}

// GetProgressiveStreamURL finds the progressive mp3 transcoding and resolves its direct MP3 link.
func (c *Client) GetProgressiveStreamURL(ctx context.Context, track *models.Track) (string, error) {
	for _, t := range track.Media.Transcodings {
		if t.Format.Protocol == "progressive" {
			return c.GetStreamURL(ctx, t.URL)
		}
	}
	return "", ErrNoProgressive
}

// GetHLSStreamURL finds the HLS transcoding and resolves its m3u8 playlist stream link.
func (c *Client) GetHLSStreamURL(ctx context.Context, track *models.Track) (string, error) {
	for _, t := range track.Media.Transcodings {
		if t.Format.Protocol == "hls" {
			return c.GetStreamURL(ctx, t.URL)
		}
	}
	return "", ErrNoHLS
}

// GetBestStreamURL tries to obtain a progressive MP3 stream first; if not available, falls back to HLS.
func (c *Client) GetBestStreamURL(ctx context.Context, track *models.Track) (string, string, error) {
	if streamURL, err := c.GetProgressiveStreamURL(ctx, track); err == nil {
		return streamURL, "progressive", nil
	}
	if streamURL, err := c.GetHLSStreamURL(ctx, track); err == nil {
		return streamURL, "hls", nil
	}
	return "", "", fmt.Errorf("%w or %w", ErrNoProgressive, ErrNoHLS)
}

// AudioFormatInfo describes a resolved SoundCloud audio format/transcoding.
type AudioFormatInfo struct {
	Preset   string `json:"preset"`    // e.g. "aac_160k", "mp3_1_0"
	Protocol string `json:"protocol"`  // "progressive" or "hls"
	MimeType string `json:"mime_type"` // "audio/mp4", "audio/mpeg"
	Quality  string `json:"quality"`   // "sq", "hq", "lq"
	Codec    string `json:"codec"`     // "aac", "mp3", "opus"
	Bitrate  string `json:"bitrate"`   // "160 kbps", "128 kbps", "96 kbps"
	URL      string `json:"url,omitempty"`
}

// GetAllStreamFormats returns all available transcodings with their direct streaming URLs.
func (c *Client) GetAllStreamFormats(ctx context.Context, track *models.Track) []AudioFormatInfo {
	var formats []AudioFormatInfo
	for _, t := range track.Media.Transcodings {
		codec := "mp3"
		bitrate := "128 kbps"
		switch t.Preset {
		case "aac_160k":
			codec = "aac"
			bitrate = "160 kbps (HQ)"
		case "aac_96k":
			codec = "aac"
			bitrate = "96 kbps"
		case "opus_0_0":
			codec = "opus"
			bitrate = "64-160 kbps"
		case "mp3_1_0":
			codec = "mp3"
			bitrate = "128 kbps"
		case "abr_sq":
			codec = "mp3"
			bitrate = "128 kbps"
		}

		info := AudioFormatInfo{
			Preset:   t.Preset,
			Protocol: t.Format.Protocol,
			MimeType: t.Format.MimeType,
			Quality:  t.Quality,
			Codec:    codec,
			Bitrate:  bitrate,
		}

		// Try resolving the direct stream link
		if streamURL, err := c.GetStreamURL(ctx, t.URL); err == nil {
			info.URL = streamURL
		}

		formats = append(formats, info)
	}
	return formats
}
