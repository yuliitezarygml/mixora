package soundcloud

import (
	"context"
	"fmt"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

// GetPlaylist retrieves a playlist or album by its ID.
func (c *Client) GetPlaylist(ctx context.Context, playlistID int64) (*models.Playlist, error) {
	path := fmt.Sprintf("/playlists/%d", playlistID)
	data, err := c.apiGet(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.Playlist](data)
}
