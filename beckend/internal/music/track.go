package music

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

var trailingNumericID = regexp.MustCompile(`(?:^|:|/)([0-9]+)$`)

// Track is the provider-neutral shape consumed by the Mixora client.
type Track struct {
	ID            string  `json:"id"`
	Source        string  `json:"source"`
	Title         string  `json:"title"`
	Artist        string  `json:"artist"`
	ArtistID      string  `json:"artistId,omitempty"`
	Artwork       string  `json:"artwork,omitempty"`
	Duration      float64 `json:"duration"`
	Explicit      bool    `json:"explicit"`
	Access        string  `json:"access"`
	Permalink     string  `json:"permalink,omitempty"`
	ArtistURL     string  `json:"artistUrl,omitempty"`
	Genre         string  `json:"genre,omitempty"`
	Description   string  `json:"description,omitempty"`
	PlaybackCount int     `json:"playbackCount,omitempty"`
	CreatedAt     string  `json:"createdAt,omitempty"`
	Album         string  `json:"album,omitempty"`
	TagList       string  `json:"tagList,omitempty"`
}

func (t Track) Key() string {
	return t.Source + ":" + t.ID
}

// CanonicalTrack normalizes legacy public representations at the app boundary.
// The client still treats the resulting reference as opaque.
func CanonicalTrack(track Track) Track {
	track.Source = strings.ToLower(strings.TrimSpace(track.Source))
	track.ID = CanonicalTrackID(track.Source, track.ID)
	track.ArtistID = CanonicalTrackID(track.Source, track.ArtistID)
	return track
}

func CanonicalTrackID(source, value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(strings.TrimSpace(source), "soundcloud") {
		if match := trailingNumericID.FindStringSubmatch(strings.TrimRight(value, "/")); len(match) == 2 {
			return match[1]
		}
	}
	return value
}

func FromSoundCloud(track models.Track) Track {
	artist := "Исполнитель"
	artistID := ""
	artistURL := ""
	artwork := track.ArtworkURL
	if track.User != nil {
		artist = strings.TrimSpace(track.User.Username)
		artistID = strconv.FormatInt(track.User.ID, 10)
		artistURL = track.User.PermalinkURL
		if artwork == "" {
			artwork = track.User.AvatarURL
		}
	}
	album := ""
	if track.PublisherMetadata != nil {
		if v := strings.TrimSpace(track.PublisherMetadata.Artist); v != "" {
			artist = v
		}
		album = track.PublisherMetadata.AlbumTitle
	}
	access := "playable"
	if !track.Streamable || track.Policy == "BLOCK" {
		access = "blocked"
	}
	return Track{
		ID:            strconv.FormatInt(track.ID, 10),
		Source:        "soundcloud",
		Title:         track.Title,
		Artist:        artist,
		ArtistID:      artistID,
		Artwork:       artwork,
		Duration:      float64(track.Duration) / 1000,
		Access:        access,
		Permalink:     track.PermalinkURL,
		ArtistURL:     artistURL,
		Genre:         track.Genre,
		Description:   track.Description,
		PlaybackCount: track.PlaybackCount,
		CreatedAt:     track.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Album:         album,
		TagList:       track.TagList,
	}
}
