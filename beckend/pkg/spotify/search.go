package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/iulian/soundcloud-go/pkg/spotify/models"
)

var (
	trackLinkRegex    = regexp.MustCompile(`open\.spotify\.com/track/([a-zA-Z0-9]+)`)
	albumLinkRegex    = regexp.MustCompile(`open\.spotify\.com/album/([a-zA-Z0-9]+)`)
	artistLinkRegex   = regexp.MustCompile(`open\.spotify\.com/artist/([a-zA-Z0-9]+)`)
	playlistLinkRegex = regexp.MustCompile(`open\.spotify\.com/playlist/([a-zA-Z0-9]+)`)
)

// Search searches Spotify for tracks, albums, artists, or playlists.
// Uses Spotify Web API if credentials/token are present, or zero-config web resolution otherwise.
func (c *Client) Search(ctx context.Context, query string, searchType ResourceType, limit int) (*models.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}
	if limit <= 0 {
		limit = 10
	}
	if searchType == "" {
		searchType = TypeTrack
	}

	result := &models.SearchResult{
		Query: query,
	}

	// 1. Try official Web API if credentials or token available
	if c.accessToken != "" || (c.clientID != "" && c.clientSecret != "") {
		if err := c.searchWithAPI(ctx, query, searchType, limit, result); err == nil {
			return result, nil
		}
	}

	// 2. Zero-config fallback via public search
	return c.searchPublic(ctx, query, searchType, limit)
}

func (c *Client) searchWithAPI(ctx context.Context, query string, sType ResourceType, limit int, result *models.SearchResult) error {
	token, err := c.getOrRefreshToken(ctx)
	if err != nil {
		return err
	}

	apiType := string(sType)
	if apiType == "" {
		apiType = "track"
	}

	reqURL := fmt.Sprintf("https://api.spotify.com/v1/search?q=%s&type=%s&limit=%d", url.QueryEscape(query), apiType, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spotify search API failed (%d)", resp.StatusCode)
	}

	var apiResp struct {
		Tracks struct {
			Items []struct {
				ID         string `json:"id"`
				URI        string `json:"uri"`
				Name       string `json:"name"`
				DurationMs int64  `json:"duration_ms"`
				Explicit   bool   `json:"explicit"`
				PreviewURL string `json:"preview_url"`
				Artists    []struct {
					ID   string `json:"id"`
					URI  string `json:"uri"`
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Images []struct {
						URL    string `json:"url"`
						Height int    `json:"height"`
						Width  int    `json:"width"`
					} `json:"images"`
				} `json:"album"`
			} `json:"items"`
		} `json:"tracks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}

	for _, item := range apiResp.Tracks.Items {
		track := models.Track{
			ID:          item.ID,
			URI:         item.URI,
			Title:       item.Name,
			DurationMs:  item.DurationMs,
			Explicit:    item.Explicit,
			PreviewURL:  item.PreviewURL,
			ExternalURL: fmt.Sprintf("https://open.spotify.com/track/%s", item.ID),
			IsPlayable:  true,
		}
		for _, a := range item.Artists {
			track.Artists = append(track.Artists, models.Artist{
				ID:          a.ID,
				URI:         a.URI,
				Name:        a.Name,
				ExternalURL: fmt.Sprintf("https://open.spotify.com/artist/%s", a.ID),
			})
		}
		var imgs []models.Image
		for _, img := range item.Album.Images {
			imgs = append(imgs, models.Image{
				URL:    img.URL,
				Height: img.Height,
				Width:  img.Width,
			})
		}
		track.Album = &models.Album{
			ID:          item.Album.ID,
			Name:        item.Album.Name,
			Images:      imgs,
			ExternalURL: fmt.Sprintf("https://open.spotify.com/album/%s", item.Album.ID),
		}
		result.Tracks = append(result.Tracks, track)
	}

	return nil
}

func (c *Client) searchPublic(ctx context.Context, query string, sType ResourceType, limit int) (*models.SearchResult, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=site:open.spotify.com/%s+%s", sType, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var idRegex *regexp.Regexp
	switch sType {
	case TypeAlbum:
		idRegex = albumLinkRegex
	case TypeArtist:
		idRegex = artistLinkRegex
	case TypePlaylist:
		idRegex = playlistLinkRegex
	default:
		idRegex = trackLinkRegex
	}

	matches := idRegex.FindAllSubmatch(body, -1)
	seen := make(map[string]bool)
	var ids []string
	for _, m := range matches {
		if len(m) >= 2 {
			id := string(m[1])
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
				if len(ids) >= limit {
					break
				}
			}
		}
	}

	result := &models.SearchResult{
		Query: query,
	}

	if len(ids) == 0 {
		return result, nil
	}

	// Fetch metadata concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, id := range ids {
		wg.Add(1)
		go func(targetID string) {
			defer wg.Done()
			switch sType {
			case TypeAlbum:
				if album, err := c.GetAlbum(ctx, targetID); err == nil && album != nil {
					mu.Lock()
					result.Albums = append(result.Albums, *album)
					mu.Unlock()
				}
			case TypeArtist:
				if artist, err := c.GetArtist(ctx, targetID); err == nil && artist != nil {
					mu.Lock()
					result.Artists = append(result.Artists, *artist)
					mu.Unlock()
				}
			case TypePlaylist:
				if playlist, err := c.GetPlaylist(ctx, targetID); err == nil && playlist != nil {
					mu.Lock()
					result.Playlists = append(result.Playlists, *playlist)
					mu.Unlock()
				}
			default:
				if track, err := c.GetTrack(ctx, targetID); err == nil && track != nil {
					mu.Lock()
					result.Tracks = append(result.Tracks, *track)
					mu.Unlock()
				}
			}
		}(id)
	}

	wg.Wait()
	return result, nil
}

func (c *Client) getOrRefreshToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.accessToken != "" {
		token := c.accessToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	if c.clientID == "" || c.clientSecret == "" {
		return "", ErrUnauthorized
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double check after lock
	if c.accessToken != "" {
		return c.accessToken, nil
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.clientID, c.clientSecret)))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed (%d)", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	c.accessToken = tokenResp.AccessToken
	return c.accessToken, nil
}
