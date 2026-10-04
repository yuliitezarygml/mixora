package music

import (
	"net/url"
	"regexp"
	"strings"

	spotifymodels "github.com/iulian/soundcloud-go/pkg/spotify/models"
	ytdlpmodels "github.com/iulian/soundcloud-go/pkg/ytdlp/models"
)

var providerSourceID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,39}$`)

// FromSpotify reduces a Spotify result to the same provider-neutral track
// shape used by the recommendation catalog. Playback and richer Spotify fields
// stay with the music engine; only stable identity and renderable metadata are
// retained here.
func FromSpotify(track spotifymodels.Track) Track {
	artistNames := make([]string, 0, len(track.Artists))
	artistID := ""
	artistURL := ""
	for _, artist := range track.Artists {
		if name := strings.TrimSpace(artist.Name); name != "" {
			artistNames = append(artistNames, name)
		}
		if artistID == "" {
			artistID = spotifyResourceID(artist.ID, artist.URI, "artist")
		}
		if artistURL == "" {
			artistURL = strings.TrimSpace(artist.ExternalURL)
		}
	}

	artist := strings.Join(artistNames, ", ")
	if artist == "" {
		artist = "Исполнитель"
	}
	artwork := ""
	album := ""
	if track.Album != nil {
		album = strings.TrimSpace(track.Album.Name)
		for _, image := range track.Album.Images {
			if image.URL = strings.TrimSpace(image.URL); image.URL != "" {
				artwork = image.URL
				break
			}
		}
	}
	duration := float64(track.DurationMs) / 1000
	if duration < 0 {
		duration = 0
	}
	// Spotify Connect controls a separate device; it is not an audio URL that
	// the Mixora browser player can assign to an <audio> element. Only an
	// explicit preview URL is browser-playable, so the recommendation catalog
	// must not offer a Connect-only track as a playable Wave candidate.
	access := "blocked"
	if strings.TrimSpace(track.PreviewURL) != "" {
		access = "preview"
	}

	return CanonicalTrack(Track{
		ID:        spotifyResourceID(track.ID, track.URI, "track"),
		Source:    "spotify",
		Title:     strings.TrimSpace(track.Title),
		Artist:    artist,
		ArtistID:  artistID,
		Artwork:   artwork,
		Duration:  duration,
		Explicit:  track.Explicit,
		Access:    access,
		Permalink: strings.TrimSpace(track.ExternalURL),
		ArtistURL: artistURL,
		Album:     album,
	})
}

func spotifyResourceID(id, uri, kind string) string {
	if id = strings.TrimSpace(id); id != "" {
		return strings.TrimPrefix(id, "spotify:"+kind+":")
	}
	return strings.TrimPrefix(strings.TrimSpace(uri), "spotify:"+kind+":")
}

// FromYTDLP adapts a provider-verified yt-dlp item without retaining its
// expiring direct-audio URLs. The extractor (or, for generic extractors, the
// webpage host) becomes the stable source namespace, so identical ids from two
// sites cannot collide in the catalog.
func FromYTDLP(item ytdlpmodels.MediaItem) Track {
	artist := strings.TrimSpace(item.Artist)
	if artist == "" {
		artist = strings.TrimSpace(item.Uploader)
	}
	if artist == "" {
		artist = "Исполнитель"
	}
	duration := item.Duration
	if duration < 0 {
		duration = 0
	}

	return CanonicalTrack(Track{
		ID:          strings.TrimSpace(item.ID),
		Source:      ytdlpProviderSource(item.Extractor, item.WebpageURL),
		Title:       strings.TrimSpace(item.Title),
		Artist:      artist,
		Artwork:     strings.TrimSpace(item.Thumbnail),
		Duration:    duration,
		Access:      "playable",
		Permalink:   strings.TrimSpace(item.WebpageURL),
		Description: strings.TrimSpace(item.Description),
		Album:       strings.TrimSpace(item.Album),
	})
}

func ytdlpProviderSource(extractor, webpageURL string) string {
	value := strings.ToLower(strings.TrimSpace(extractor))
	if separator := strings.IndexByte(value, ':'); separator >= 0 {
		value = value[:separator]
	}
	if value != "" && value != "generic" && providerSourceID.MatchString(value) {
		return value
	}

	parsed, err := url.Parse(strings.TrimSpace(webpageURL))
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	switch {
	case host == "youtu.be" || host == "youtube.com" || strings.HasSuffix(host, ".youtube.com"):
		return "youtube"
	case host == "bandcamp.com" || strings.HasSuffix(host, ".bandcamp.com"):
		return "bandcamp"
	case host == "vk.com" || strings.HasSuffix(host, ".vk.com") || host == "vkvideo.ru" || strings.HasSuffix(host, ".vkvideo.ru"):
		return "vk"
	case providerSourceID.MatchString(host):
		return host
	default:
		return ""
	}
}
