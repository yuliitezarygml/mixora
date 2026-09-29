package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	spotifyURLRegex = regexp.MustCompile(`open\.spotify\.com/(track|album|playlist|artist)/([a-zA-Z0-9]+)`)
	spotifyURIRegex = regexp.MustCompile(`spotify:(track|album|playlist|artist):([a-zA-Z0-9]+)`)
	nextDataRegex   = regexp.MustCompile(`<script id="__NEXT_DATA__" type="application/json">([^<]+)</script>`)
)

// ResourceType represents the Spotify entity type.
type ResourceType string

const (
	TypeTrack    ResourceType = "track"
	TypeAlbum    ResourceType = "album"
	TypePlaylist ResourceType = "playlist"
	TypeArtist   ResourceType = "artist"
)

// ParsedResource contains the extracted type and ID of a Spotify URL/URI.
type ParsedResource struct {
	Type ResourceType
	ID   string
	URI  string
}

// ParseURL parses a Spotify URL or URI into its component type and ID.
func ParseURL(input string) (*ParsedResource, error) {
	input = strings.TrimSpace(input)
	if m := spotifyURIRegex.FindStringSubmatch(input); len(m) == 3 {
		return &ParsedResource{
			Type: ResourceType(m[1]),
			ID:   m[2],
			URI:  fmt.Sprintf("spotify:%s:%s", m[1], m[2]),
		}, nil
	}

	if m := spotifyURLRegex.FindStringSubmatch(input); len(m) == 3 {
		return &ParsedResource{
			Type: ResourceType(m[1]),
			ID:   m[2],
			URI:  fmt.Sprintf("spotify:%s:%s", m[1], m[2]),
		}, nil
	}

	return nil, ErrInvalidURL
}

// Resolve resolves any Spotify public URL or URI into its typed entity.
func (c *Client) Resolve(ctx context.Context, rawURL string) (any, error) {
	parsed, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}

	switch parsed.Type {
	case TypeTrack:
		return c.GetTrack(ctx, parsed.ID)
	case TypeAlbum:
		return c.GetAlbum(ctx, parsed.ID)
	case TypeArtist:
		return c.GetArtist(ctx, parsed.ID)
	case TypePlaylist:
		return c.GetPlaylist(ctx, parsed.ID)
	default:
		return nil, ErrInvalidURL
	}
}

// fetchEmbedData fetches the embed page for a Spotify resource and extracts its raw entity JSON data.
func (c *Client) fetchEmbedData(ctx context.Context, resType ResourceType, id string) (map[string]any, error) {
	embedURL := fmt.Sprintf("https://open.spotify.com/embed/%s/%s", resType, url.PathEscape(id))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching embed page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify embed error (%d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	matches := nextDataRegex.FindSubmatch(body)
	if len(matches) < 2 {
		return nil, fmt.Errorf("failed to extract NEXT_DATA from Spotify embed")
	}

	var root map[string]any
	if err := json.Unmarshal(matches[1], &root); err != nil {
		return nil, fmt.Errorf("parsing NEXT_DATA JSON: %w", err)
	}

	props, _ := root["props"].(map[string]any)
	pageProps, _ := props["pageProps"].(map[string]any)
	state, _ := pageProps["state"].(map[string]any)
	data, _ := state["data"].(map[string]any)
	entity, _ := data["entity"].(map[string]any)

	if entity == nil {
		return nil, ErrNotFound
	}

	return entity, nil
}
