package soundcloud

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

// GetTrending returns trending ("New & Hot") tracks for a specified genre.
// If genre is empty, "all-music" is used.
// Supported genres: "all-music", "danceedm", "hiphoprap", "rock", "pop", "electronic", "deephouse", "techno", "trap", "rbsoul", etc.
func (c *Client) GetTrending(ctx context.Context, genre string, limit int) (*models.ChartResponse, error) {
	if genre == "" {
		genre = "all-music"
	}
	if !strings.HasPrefix(genre, "soundcloud:genres:") {
		genre = "soundcloud:genres:" + genre
	}
	if limit <= 0 {
		limit = 20
	} else if limit > 50 {
		limit = 50
	}

	params := url.Values{
		"kind":  {"trending"},
		"genre": {genre},
		"limit": {strconv.Itoa(limit)},
	}

	data, err := c.apiGet(ctx, "/charts", params)
	if err != nil {
		return nil, err
	}

	return decodeJSON[*models.ChartResponse](data)
}
