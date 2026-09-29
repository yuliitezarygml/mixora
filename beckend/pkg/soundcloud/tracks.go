package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

// GetTrack retrieves a single track by its ID.
func (c *Client) GetTrack(ctx context.Context, trackID int64) (*models.Track, error) {
	path := fmt.Sprintf("/tracks/%d", trackID)
	data, err := c.apiGet(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.Track](data)
}

// GetTracks retrieves a batch of tracks by their IDs.
func (c *Client) GetTracks(ctx context.Context, ids []int64) ([]models.Track, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = strconv.FormatInt(id, 10)
	}

	params := url.Values{
		"ids": {strings.Join(strIDs, ",")},
	}

	data, err := c.apiGet(ctx, "/tracks", params)
	if err != nil {
		return nil, err
	}
	return decodeJSON[[]models.Track](data)
}

// GetTrackComments retrieves paginated comments on a track.
func (c *Client) GetTrackComments(ctx context.Context, trackID int64, opts SearchOptions) (*models.PaginatedResponse[models.Comment], error) {
	path := fmt.Sprintf("/tracks/%d/comments", trackID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Comment]](data)
}

// GetTrackLikers retrieves users who liked a track.
func (c *Client) GetTrackLikers(ctx context.Context, trackID int64, opts SearchOptions) (*models.PaginatedResponse[models.User], error) {
	path := fmt.Sprintf("/tracks/%d/likers", trackID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.User]](data)
}

// GetRelatedTracks retrieves related tracks for recommendations.
func (c *Client) GetRelatedTracks(ctx context.Context, trackID int64, opts SearchOptions) (*models.PaginatedResponse[models.Track], error) {
	path := fmt.Sprintf("/tracks/%d/related", trackID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Track]](data)
}

// GetTrackWaveform fetches the sound wave amplitude samples for drawing the audio player visualizer.
func (c *Client) GetTrackWaveform(ctx context.Context, track *models.Track) (*models.Waveform, error) {
	if track.WaveformURL == "" {
		return nil, fmt.Errorf("soundcloud: track has no waveform_url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, track.WaveformURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("soundcloud: waveform HTTP %d", resp.StatusCode)
	}

	var wf models.Waveform
	if err := json.NewDecoder(resp.Body).Decode(&wf); err != nil {
		return nil, fmt.Errorf("soundcloud: failed to parse waveform json: %w", err)
	}

	return &wf, nil
}
