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

	var apiResp spotifySearchAPIResponse

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}

	// Spotify puts results under a top-level key matching the requested type.
	// Decode and normalize only that type so a valid artist/album/playlist
	// response does not look like an empty track search.
	switch sType {
	case TypeArtist:
		for _, item := range apiResp.Artists.Items {
			result.Artists = append(result.Artists, normalizeSpotifyAPIArtist(item))
		}
	case TypeAlbum:
		for _, item := range apiResp.Albums.Items {
			result.Albums = append(result.Albums, normalizeSpotifyAPIAlbum(item))
		}
	case TypePlaylist:
		for _, item := range apiResp.Playlists.Items {
			result.Playlists = append(result.Playlists, normalizeSpotifyAPIPlaylist(item))
		}
	default:
		for _, item := range apiResp.Tracks.Items {
			result.Tracks = append(result.Tracks, normalizeSpotifyAPITrack(item))
		}
	}

	return nil
}

// spotifySearchAPIResponse models the four result containers returned by the
// official /v1/search endpoint. The API only fills the container requested in
// the type parameter, but defining every container here keeps the normalizers
// explicit and prevents type-specific fields from being silently discarded.
type spotifySearchAPIResponse struct {
	Tracks struct {
		Items []spotifyAPITrack `json:"items"`
	} `json:"tracks"`
	Artists struct {
		Items []spotifyAPIArtist `json:"items"`
	} `json:"artists"`
	Albums struct {
		Items []spotifyAPIAlbum `json:"items"`
	} `json:"albums"`
	Playlists struct {
		Items []spotifyAPIPlaylist `json:"items"`
	} `json:"playlists"`
}

type spotifyAPIExternalURLs struct {
	Spotify string `json:"spotify"`
}

type spotifyAPIImage struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type spotifyAPIArtist struct {
	ID           string                 `json:"id"`
	URI          string                 `json:"uri"`
	Name         string                 `json:"name"`
	Genres       []string               `json:"genres"`
	Images       []spotifyAPIImage      `json:"images"`
	Popularity   int                    `json:"popularity"`
	Followers    spotifyAPIFollowers    `json:"followers"`
	ExternalURLs spotifyAPIExternalURLs `json:"external_urls"`
}

type spotifyAPIFollowers struct {
	Total int `json:"total"`
}

type spotifyAPIAlbum struct {
	ID           string                 `json:"id"`
	URI          string                 `json:"uri"`
	Name         string                 `json:"name"`
	AlbumType    string                 `json:"album_type"`
	ReleaseDate  string                 `json:"release_date"`
	TotalTracks  int                    `json:"total_tracks"`
	Images       []spotifyAPIImage      `json:"images"`
	Artists      []spotifyAPIArtist     `json:"artists"`
	ExternalURLs spotifyAPIExternalURLs `json:"external_urls"`
}

type spotifyAPITrack struct {
	ID           string                 `json:"id"`
	URI          string                 `json:"uri"`
	Name         string                 `json:"name"`
	DurationMs   int64                  `json:"duration_ms"`
	Explicit     bool                   `json:"explicit"`
	Popularity   int                    `json:"popularity"`
	PreviewURL   string                 `json:"preview_url"`
	IsPlayable   *bool                  `json:"is_playable"`
	TrackNumber  int                    `json:"track_number"`
	DiscNumber   int                    `json:"disc_number"`
	Artists      []spotifyAPIArtist     `json:"artists"`
	Album        spotifyAPIAlbum        `json:"album"`
	ExternalURLs spotifyAPIExternalURLs `json:"external_urls"`
}

type spotifyAPIPlaylist struct {
	ID           string                  `json:"id"`
	URI          string                  `json:"uri"`
	Name         string                  `json:"name"`
	Description  string                  `json:"description"`
	Owner        spotifyAPIOwner         `json:"owner"`
	Images       []spotifyAPIImage       `json:"images"`
	Tracks       spotifyAPITracksSummary `json:"tracks"`
	ExternalURLs spotifyAPIExternalURLs  `json:"external_urls"`
}

type spotifyAPIOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type spotifyAPITracksSummary struct {
	Total int `json:"total"`
}

func normalizeSpotifyAPIImages(images []spotifyAPIImage) []models.Image {
	result := make([]models.Image, 0, len(images))
	for _, image := range images {
		result = append(result, models.Image{
			URL:    image.URL,
			Height: image.Height,
			Width:  image.Width,
		})
	}
	return result
}

func spotifyAPIURI(resourceType ResourceType, id, uri string) string {
	if uri != "" {
		return uri
	}
	if id == "" {
		return ""
	}
	return fmt.Sprintf("spotify:%s:%s", resourceType, id)
}

func spotifyAPIExternalURL(resourceType ResourceType, id, externalURL string) string {
	if externalURL != "" {
		return externalURL
	}
	if id == "" {
		return ""
	}
	return fmt.Sprintf("https://open.spotify.com/%s/%s", resourceType, id)
}

func normalizeSpotifyAPIArtist(item spotifyAPIArtist) models.Artist {
	return models.Artist{
		ID:          item.ID,
		URI:         spotifyAPIURI(TypeArtist, item.ID, item.URI),
		Name:        item.Name,
		Genres:      item.Genres,
		Images:      normalizeSpotifyAPIImages(item.Images),
		Popularity:  item.Popularity,
		Followers:   item.Followers.Total,
		ExternalURL: spotifyAPIExternalURL(TypeArtist, item.ID, item.ExternalURLs.Spotify),
	}
}

func normalizeSpotifyAPIAlbum(item spotifyAPIAlbum) models.Album {
	album := models.Album{
		ID:          item.ID,
		URI:         spotifyAPIURI(TypeAlbum, item.ID, item.URI),
		Name:        item.Name,
		AlbumType:   item.AlbumType,
		ReleaseDate: item.ReleaseDate,
		TotalTracks: item.TotalTracks,
		Images:      normalizeSpotifyAPIImages(item.Images),
		ExternalURL: spotifyAPIExternalURL(TypeAlbum, item.ID, item.ExternalURLs.Spotify),
	}
	for _, artist := range item.Artists {
		album.Artists = append(album.Artists, normalizeSpotifyAPIArtist(artist))
	}
	return album
}

func normalizeSpotifyAPITrack(item spotifyAPITrack) models.Track {
	// Some Spotify markets omit is_playable. Keep the package's historical
	// default (playable unless the API explicitly says otherwise) in that case.
	isPlayable := true
	if item.IsPlayable != nil {
		isPlayable = *item.IsPlayable
	}

	track := models.Track{
		ID:          item.ID,
		URI:         spotifyAPIURI(TypeTrack, item.ID, item.URI),
		Title:       item.Name,
		DurationMs:  item.DurationMs,
		Explicit:    item.Explicit,
		Popularity:  item.Popularity,
		PreviewURL:  item.PreviewURL,
		IsPlayable:  isPlayable,
		TrackNumber: item.TrackNumber,
		DiscNumber:  item.DiscNumber,
		ExternalURL: spotifyAPIExternalURL(TypeTrack, item.ID, item.ExternalURLs.Spotify),
	}
	for _, artist := range item.Artists {
		track.Artists = append(track.Artists, normalizeSpotifyAPIArtist(artist))
	}
	if item.Album.ID != "" || item.Album.Name != "" || len(item.Album.Images) > 0 {
		album := normalizeSpotifyAPIAlbum(item.Album)
		track.Album = &album
	}
	return track
}

func normalizeSpotifyAPIPlaylist(item spotifyAPIPlaylist) models.Playlist {
	owner := item.Owner.DisplayName
	if owner == "" {
		owner = item.Owner.ID
	}
	return models.Playlist{
		ID:          item.ID,
		URI:         spotifyAPIURI(TypePlaylist, item.ID, item.URI),
		Name:        item.Name,
		Description: item.Description,
		Owner:       owner,
		Images:      normalizeSpotifyAPIImages(item.Images),
		TotalTracks: item.Tracks.Total,
		ExternalURL: spotifyAPIExternalURL(TypePlaylist, item.ID, item.ExternalURLs.Spotify),
	}
}

func (c *Client) searchPublic(ctx context.Context, query string, sType ResourceType, limit int) (*models.SearchResult, error) {
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

	// Spotify has no official unauthenticated search API. The regular
	// DuckDuckGo HTML endpoint is frequently terminated by bot protection in
	// containers, while its lightweight endpoint stays server-rendered and
	// exposes the same public Spotify links. Keep the old endpoint as a
	// fallback: this is a zero-config metadata discovery path, not a source of
	// audio or account credentials.
	searchURLs := []string{
		fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=site:open.spotify.com/%s+%s", sType, url.QueryEscape(query)),
		fmt.Sprintf("https://html.duckduckgo.com/html/?q=site:open.spotify.com/%s+%s", sType, url.QueryEscape(query)),
	}
	var body []byte
	var lastErr error
	for _, searchURL := range searchURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", c.userAgent)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		candidate, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			lastErr = fmt.Errorf("public Spotify search returned HTTP %d", resp.StatusCode)
			continue
		}
		body = candidate
		if len(idRegex.FindAllSubmatch(body, -1)) > 0 {
			break
		}
	}
	if len(body) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("public Spotify search returned no response")
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
