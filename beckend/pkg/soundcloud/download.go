package soundcloud

import (
	"context"
	"fmt"
)

// DownloadResponse represents the payload from the original download endpoint.
type DownloadResponse struct {
	RedirectURI string `json:"redirectUri"`
}

// GetOriginalDownloadURL retrieves the direct download link for the original, uncompressed
// audio file (WAV, FLAC, AIFF, or 320kbps MP3) uploaded by the creator.
// Note: This endpoint only succeeds if track.Downloadable == true.
func (c *Client) GetOriginalDownloadURL(ctx context.Context, trackID int64) (string, error) {
	path := fmt.Sprintf("/tracks/%d/download", trackID)
	data, err := c.apiGet(ctx, path, nil)
	if err != nil {
		return "", err
	}

	res, err := decodeJSON[DownloadResponse](data)
	if err != nil {
		return "", err
	}

	if res.RedirectURI == "" {
		return "", fmt.Errorf("soundcloud: track %d is not available for direct download", trackID)
	}

	return res.RedirectURI, nil
}
