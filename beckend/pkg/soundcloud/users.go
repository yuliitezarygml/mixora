package soundcloud

import (
	"context"
	"fmt"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

// GetUser retrieves user profile information by user ID.
func (c *Client) GetUser(ctx context.Context, userID int64) (*models.User, error) {
	path := fmt.Sprintf("/users/%d", userID)
	data, err := c.apiGet(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.User](data)
}

// GetUserTracks retrieves tracks uploaded by a specific user.
func (c *Client) GetUserTracks(ctx context.Context, userID int64, opts SearchOptions) (*models.PaginatedResponse[models.Track], error) {
	path := fmt.Sprintf("/users/%d/tracks", userID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Track]](data)
}

// GetUserPlaylists retrieves playlists created by a specific user.
func (c *Client) GetUserPlaylists(ctx context.Context, userID int64, opts SearchOptions) (*models.PaginatedResponse[models.Playlist], error) {
	path := fmt.Sprintf("/users/%d/playlists", userID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Playlist]](data)
}

// GetUserAlbums retrieves albums created by a specific user.
func (c *Client) GetUserAlbums(ctx context.Context, userID int64, opts SearchOptions) (*models.PaginatedResponse[models.Playlist], error) {
	path := fmt.Sprintf("/users/%d/albums", userID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Playlist]](data)
}

// GetUserLikes retrieves tracks liked by a specific user.
func (c *Client) GetUserLikes(ctx context.Context, userID int64, opts SearchOptions) (*models.PaginatedResponse[models.Like], error) {
	path := fmt.Sprintf("/users/%d/likes", userID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Like]](data)
}

// GetUserFollowers retrieves followers of a user.
func (c *Client) GetUserFollowers(ctx context.Context, userID int64, opts SearchOptions) (*models.PaginatedResponse[models.User], error) {
	path := fmt.Sprintf("/users/%d/followers", userID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.User]](data)
}

// GetUserFollowings retrieves users followed by a user.
func (c *Client) GetUserFollowings(ctx context.Context, userID int64, opts SearchOptions) (*models.PaginatedResponse[models.User], error) {
	path := fmt.Sprintf("/users/%d/followings", userID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.User]](data)
}

// GetUserTopTracks retrieves top/most played tracks for a user.
func (c *Client) GetUserTopTracks(ctx context.Context, userID int64, opts SearchOptions) (*models.PaginatedResponse[models.Track], error) {
	path := fmt.Sprintf("/users/%d/toptracks", userID)
	data, err := c.apiGet(ctx, path, opts.values())
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Track]](data)
}

// GetUserWebProfiles retrieves external links associated with a user profile.
func (c *Client) GetUserWebProfiles(ctx context.Context, userID int64) ([]models.WebProfile, error) {
	path := fmt.Sprintf("/users/%d/web-profiles", userID)
	data, err := c.apiGet(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[[]models.WebProfile](data)
}
