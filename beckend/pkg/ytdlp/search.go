package ytdlp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/ytdlp/models"
)

// SearchYouTube performs a fast search on YouTube for audio/video tracks without any API keys.
func (c *Client) SearchYouTube(ctx context.Context, query string, limit int) (*models.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}
	if limit <= 0 {
		limit = 5
	} else if limit > 25 {
		limit = 25
	}

	searchSpec := fmt.Sprintf("ytsearch%d:%s", limit, query)
	out, err := c.runCommand(ctx, "-J", "--flat-playlist", searchSpec)
	if err != nil {
		return nil, err
	}

	var raw rawPlaylist
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing YouTube search JSON: %w", err)
	}

	result := &models.SearchResult{
		Query:   query,
		Service: "youtube",
	}

	for _, entry := range raw.Entries {
		item := mapRawToMediaItem(&entry)
		if item.WebpageURL == "" && item.ID != "" {
			item.WebpageURL = fmt.Sprintf("https://www.youtube.com/watch?v=%s", item.ID)
		}
		result.Items = append(result.Items, *item)
	}

	return result, nil
}
