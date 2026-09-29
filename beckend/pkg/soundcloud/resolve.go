package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

// Resolve queries the /resolve endpoint to inspect any SoundCloud permalink URL.
// Returns the raw JSON payload with the "kind" field.
func (c *Client) Resolve(ctx context.Context, scURL string) (json.RawMessage, error) {
	params := url.Values{"url": {scURL}}
	data, err := c.apiGet(ctx, "/resolve", params)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// ResolveTrack resolves a track permalink URL to a Track model.
func (c *Client) ResolveTrack(ctx context.Context, scURL string) (*models.Track, error) {
	raw, err := c.Resolve(ctx, scURL)
	if err != nil {
		return nil, err
	}

	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}

	if meta.Kind != "track" {
		return nil, fmt.Errorf("%w: expected track, got %s", ErrUnexpectedKind, meta.Kind)
	}

	return decodeJSON[*models.Track](raw)
}

// ResolveUser resolves a user permalink URL to a User model.
func (c *Client) ResolveUser(ctx context.Context, scURL string) (*models.User, error) {
	raw, err := c.Resolve(ctx, scURL)
	if err != nil {
		return nil, err
	}

	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}

	if meta.Kind != "user" {
		return nil, fmt.Errorf("%w: expected user, got %s", ErrUnexpectedKind, meta.Kind)
	}

	return decodeJSON[*models.User](raw)
}

// ResolvePlaylist resolves a playlist/set/album permalink URL to a Playlist model.
func (c *Client) ResolvePlaylist(ctx context.Context, scURL string) (*models.Playlist, error) {
	raw, err := c.Resolve(ctx, scURL)
	if err != nil {
		return nil, err
	}

	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}

	if meta.Kind != "playlist" && meta.Kind != "system-playlist" {
		return nil, fmt.Errorf("%w: expected playlist, got %s", ErrUnexpectedKind, meta.Kind)
	}

	return decodeJSON[*models.Playlist](raw)
}
