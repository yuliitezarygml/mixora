package soundcloud

import (
	"context"
	"net/url"
	"strconv"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

// SearchOptions holds parameters for searching and paginating resources.
type SearchOptions struct {
	Limit  int // Maximum number of items (default 20, max 50)
	Offset int // Pagination offset
}

func (o SearchOptions) values() url.Values {
	p := url.Values{}
	limit := o.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 50 {
		limit = 50
	}

	p.Set("limit", strconv.Itoa(limit))
	if o.Offset > 0 {
		p.Set("offset", strconv.Itoa(o.Offset))
	}
	return p
}

// Search executes a general multi-type search query (tracks, playlists, users).
func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) (*models.SearchResult, error) {
	p := opts.values()
	p.Set("q", query)
	data, err := c.apiGet(ctx, "/search", p)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.SearchResult](data)
}

// SearchTracks searches specifically for tracks matching the query.
func (c *Client) SearchTracks(ctx context.Context, query string, opts SearchOptions) (*models.PaginatedResponse[models.Track], error) {
	p := opts.values()
	p.Set("q", query)
	data, err := c.apiGet(ctx, "/search/tracks", p)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Track]](data)
}

// SearchUsers searches specifically for users matching the query.
func (c *Client) SearchUsers(ctx context.Context, query string, opts SearchOptions) (*models.PaginatedResponse[models.User], error) {
	p := opts.values()
	p.Set("q", query)
	data, err := c.apiGet(ctx, "/search/users", p)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.User]](data)
}

// SearchPlaylists searches specifically for playlists matching the query.
func (c *Client) SearchPlaylists(ctx context.Context, query string, opts SearchOptions) (*models.PaginatedResponse[models.Playlist], error) {
	p := opts.values()
	p.Set("q", query)
	data, err := c.apiGet(ctx, "/search/playlists", p)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Playlist]](data)
}

// SearchAlbums searches specifically for albums matching the query.
func (c *Client) SearchAlbums(ctx context.Context, query string, opts SearchOptions) (*models.PaginatedResponse[models.Playlist], error) {
	p := opts.values()
	p.Set("q", query)
	data, err := c.apiGet(ctx, "/search/albums", p)
	if err != nil {
		return nil, err
	}
	return decodeJSON[*models.PaginatedResponse[models.Playlist]](data)
}
