package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/pkg/spotify/models"
)

// ConnectClient is the HTTP client for interacting with the Spotify Connect daemon (go-librespot / librespot).
type ConnectClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewConnectClient creates a new Spotify Connect daemon client.
// Default baseURL is http://127.0.0.1:24879 if empty.
func NewConnectClient(baseURL string, hc *http.Client) *ConnectClient {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:24879"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	return &ConnectClient{
		baseURL:    baseURL,
		httpClient: hc,
	}
}

// BaseURL returns the configured base URL of the Connect daemon.
func (c *ConnectClient) BaseURL() string {
	return c.baseURL
}

// IsAvailable checks if the Spotify Connect daemon is responding.
func (c *ConnectClient) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/status", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// Status retrieves the current player and device status.
func (c *ConnectClient) Status(ctx context.Context) (*models.ConnectStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("creating status request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status failed (%d): %s", resp.StatusCode, string(body))
	}

	var status models.ConnectStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decoding status response: %w", err)
	}
	return &status, nil
}

// Load loads a Spotify URI (track, album, playlist, artist, episode) and optionally begins playback.
func (c *ConnectClient) Load(ctx context.Context, uri string, play bool) error {
	params := url.Values{}
	params.Set("uri", uri)
	params.Set("play", strconv.FormatBool(play))

	endpoint := fmt.Sprintf("%s/player/load?%s", c.baseURL, params.Encode())
	return c.postAction(ctx, endpoint)
}

// Play resumes playback on the Spotify Connect device.
func (c *ConnectClient) Play(ctx context.Context) error {
	return c.postAction(ctx, c.baseURL+"/player/play")
}

// Pause pauses playback on the Spotify Connect device.
func (c *ConnectClient) Pause(ctx context.Context) error {
	return c.postAction(ctx, c.baseURL+"/player/pause")
}

// PlayPause toggles play/pause state.
func (c *ConnectClient) PlayPause(ctx context.Context) error {
	return c.postAction(ctx, c.baseURL+"/player/play-pause")
}

// Next skips to the next track.
func (c *ConnectClient) Next(ctx context.Context) error {
	return c.postAction(ctx, c.baseURL+"/player/next")
}

// Prev skips to the previous track.
func (c *ConnectClient) Prev(ctx context.Context) error {
	return c.postAction(ctx, c.baseURL+"/player/prev")
}

// Seek seeks to position in milliseconds.
func (c *ConnectClient) Seek(ctx context.Context, positionMs int64) error {
	endpoint := fmt.Sprintf("%s/player/seek?position=%d", c.baseURL, positionMs)
	return c.postAction(ctx, endpoint)
}

// SetVolume sets the volume level (percentage 0-100).
// Automatically scales 0-100 to the Connect standard 0-65535 range.
func (c *ConnectClient) SetVolume(ctx context.Context, volumePercent int) error {
	if volumePercent < 0 {
		volumePercent = 0
	}
	if volumePercent > 100 {
		volumePercent = 100
	}
	val := int(float64(volumePercent) / 100.0 * 65535.0)
	endpoint := fmt.Sprintf("%s/player/volume?volume=%d", c.baseURL, val)
	return c.postAction(ctx, endpoint)
}

func (c *ConnectClient) postAction(ctx context.Context, fullURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, nil)
	if err != nil {
		return fmt.Errorf("creating action request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConnectUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("connect action failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}
