package spotify

import (
	"context"
	"fmt"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/spotify/models"
)

// GetTrack retrieves a track by Spotify ID or URI.
func (c *Client) GetTrack(ctx context.Context, idOrURI string) (*models.Track, error) {
	parsed, err := ParseURL(idOrURI)
	id := idOrURI
	if err == nil {
		id = parsed.ID
	} else {
		id = strings.TrimPrefix(id, "spotify:track:")
	}

	entity, err := c.fetchEmbedData(ctx, TypeTrack, id)
	if err != nil {
		return nil, err
	}

	return parseTrackEntity(entity), nil
}

// GetAlbum retrieves an album by Spotify ID or URI, including its tracklist.
func (c *Client) GetAlbum(ctx context.Context, idOrURI string) (*models.Album, error) {
	parsed, err := ParseURL(idOrURI)
	id := idOrURI
	if err == nil {
		id = parsed.ID
	} else {
		id = strings.TrimPrefix(id, "spotify:album:")
	}

	entity, err := c.fetchEmbedData(ctx, TypeAlbum, id)
	if err != nil {
		return nil, err
	}

	album := &models.Album{
		ID:          id,
		URI:         fmt.Sprintf("spotify:album:%s", id),
		Name:        getString(entity, "title", "name"),
		AlbumType:   "album",
		ExternalURL: fmt.Sprintf("https://open.spotify.com/album/%s", id),
	}

	if subtitle := getString(entity, "subtitle"); subtitle != "" {
		album.Artists = []models.Artist{{Name: subtitle}}
	}

	album.Images = parseImages(entity)
	album.Tracks = parseTrackList(entity, album.Name)
	album.TotalTracks = len(album.Tracks)

	return album, nil
}

// GetArtist retrieves artist details and top tracks.
func (c *Client) GetArtist(ctx context.Context, idOrURI string) (*models.Artist, error) {
	parsed, err := ParseURL(idOrURI)
	id := idOrURI
	if err == nil {
		id = parsed.ID
	} else {
		id = strings.TrimPrefix(id, "spotify:artist:")
	}

	entity, err := c.fetchEmbedData(ctx, TypeArtist, id)
	if err != nil {
		return nil, err
	}

	artist := &models.Artist{
		ID:          id,
		URI:         fmt.Sprintf("spotify:artist:%s", id),
		Name:        getString(entity, "name", "title"),
		ExternalURL: fmt.Sprintf("https://open.spotify.com/artist/%s", id),
		Images:      parseImages(entity),
		TopTracks:   parseTrackList(entity, ""),
	}

	return artist, nil
}

// GetPlaylist retrieves a playlist by ID or URI, including its tracks.
func (c *Client) GetPlaylist(ctx context.Context, idOrURI string) (*models.Playlist, error) {
	parsed, err := ParseURL(idOrURI)
	id := idOrURI
	if err == nil {
		id = parsed.ID
	} else {
		id = strings.TrimPrefix(id, "spotify:playlist:")
	}

	entity, err := c.fetchEmbedData(ctx, TypePlaylist, id)
	if err != nil {
		return nil, err
	}

	playlist := &models.Playlist{
		ID:          id,
		URI:         fmt.Sprintf("spotify:playlist:%s", id),
		Name:        getString(entity, "name", "title"),
		Owner:       getString(entity, "subtitle"),
		Description: getString(entity, "description"),
		Images:      parseImages(entity),
		Tracks:      parseTrackList(entity, ""),
		ExternalURL: fmt.Sprintf("https://open.spotify.com/playlist/%s", id),
	}
	playlist.TotalTracks = len(playlist.Tracks)

	return playlist, nil
}

func parseTrackEntity(entity map[string]any) *models.Track {
	id := getString(entity, "id")
	track := &models.Track{
		ID:          id,
		URI:         getString(entity, "uri"),
		Title:       getString(entity, "name", "title"),
		DurationMs:  getInt64(entity, "duration"),
		Explicit:    getBool(entity, "isExplicit"),
		IsPlayable:  getBool(entity, "isPlayable"),
		ExternalURL: fmt.Sprintf("https://open.spotify.com/track/%s", id),
	}

	if track.URI == "" && id != "" {
		track.URI = fmt.Sprintf("spotify:track:%s", id)
	}

	// Audio preview
	if ap, ok := entity["audioPreview"].(map[string]any); ok {
		track.PreviewURL = getString(ap, "url")
	}

	// Artists
	if artistsRaw, ok := entity["artists"].([]any); ok {
		for _, a := range artistsRaw {
			if aMap, ok := a.(map[string]any); ok {
				artist := models.Artist{
					Name: getString(aMap, "name"),
					URI:  getString(aMap, "uri"),
				}
				if strings.HasPrefix(artist.URI, "spotify:artist:") {
					artist.ID = strings.TrimPrefix(artist.URI, "spotify:artist:")
					artist.ExternalURL = fmt.Sprintf("https://open.spotify.com/artist/%s", artist.ID)
				}
				track.Artists = append(track.Artists, artist)
			}
		}
	}

	// Images / Album
	images := parseImages(entity)
	albumName := ""
	if rel, ok := entity["relatedEntityUri"].(string); ok && strings.HasPrefix(rel, "spotify:album:") {
		albumID := strings.TrimPrefix(rel, "spotify:album:")
		track.Album = &models.Album{
			ID:          albumID,
			URI:         rel,
			Images:      images,
			ExternalURL: fmt.Sprintf("https://open.spotify.com/album/%s", albumID),
		}
	} else if len(images) > 0 {
		track.Album = &models.Album{
			Name:   albumName,
			Images: images,
		}
	}

	return track
}

func parseTrackList(entity map[string]any, defaultAlbum string) []models.Track {
	var tracks []models.Track
	trackListRaw, ok := entity["trackList"].([]any)
	if !ok {
		return tracks
	}

	for i, t := range trackListRaw {
		tMap, ok := t.(map[string]any)
		if !ok {
			continue
		}

		uri := getString(tMap, "uri")
		id := strings.TrimPrefix(uri, "spotify:track:")
		track := models.Track{
			ID:          id,
			URI:         uri,
			Title:       getString(tMap, "title", "name"),
			DurationMs:  getInt64(tMap, "duration"),
			Explicit:    getBool(tMap, "isExplicit"),
			IsPlayable:  getBool(tMap, "isPlayable"),
			TrackNumber: i + 1,
			ExternalURL: fmt.Sprintf("https://open.spotify.com/track/%s", id),
		}

		if sub := getString(tMap, "subtitle"); sub != "" {
			track.Artists = []models.Artist{{Name: sub}}
		}

		if ap, ok := tMap["audioPreview"].(map[string]any); ok {
			track.PreviewURL = getString(ap, "url")
		}

		if defaultAlbum != "" {
			track.Album = &models.Album{Name: defaultAlbum}
		}

		tracks = append(tracks, track)
	}

	return tracks
}

func parseImages(entity map[string]any) []models.Image {
	var images []models.Image

	// Check coverArt -> sources
	if coverArt, ok := entity["coverArt"].(map[string]any); ok {
		if sources, ok := coverArt["sources"].([]any); ok {
			for _, s := range sources {
				if sMap, ok := s.(map[string]any); ok {
					if u := getString(sMap, "url"); u != "" {
						images = append(images, models.Image{
							URL:    u,
							Height: int(getInt64(sMap, "height")),
							Width:  int(getInt64(sMap, "width")),
						})
					}
				}
			}
		}
	}

	// Check visualIdentity -> image
	if len(images) == 0 {
		if vi, ok := entity["visualIdentity"].(map[string]any); ok {
			if imgList, ok := vi["image"].([]any); ok {
				for _, img := range imgList {
					if iMap, ok := img.(map[string]any); ok {
						if u := getString(iMap, "url"); u != "" {
							images = append(images, models.Image{
								URL:    u,
								Height: int(getInt64(iMap, "height")),
								Width:  int(getInt64(iMap, "width")),
							})
						}
					}
				}
			}
		}
	}

	return images
}

func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func getInt64(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k].(float64); ok {
			return int64(v)
		}
	}
	return 0
}

func getBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k].(bool); ok {
			return v
		}
	}
	return false
}
