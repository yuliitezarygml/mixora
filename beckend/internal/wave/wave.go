package wave

import (
	"encoding/json"
	"math"
	"strings"
	"unicode/utf8"
)

type Track struct {
	ID            string  `json:"id"`
	Source        string  `json:"source"`
	Title         string  `json:"title"`
	Artist        string  `json:"artist"`
	ArtistID      string  `json:"artistId"`
	Artwork       string  `json:"artwork"`
	Duration      float64 `json:"duration"`
	Explicit      bool    `json:"explicit"`
	Access        string  `json:"access"`
	Permalink     string  `json:"permalink"`
	Genre         string  `json:"genre"`
	PlaybackCount int     `json:"playbackCount"`
}

type Preferences struct {
	Activity  string `json:"activity"`
	Diversity string `json:"diversity"`
	Mood      string `json:"mood"`
	Language  string `json:"language"`
}

type Context struct {
	Artist string `json:"artist"`
	Genre  string `json:"genre"`
	Title  string `json:"title"`
}

type Library struct {
	Likes, History, Dislikes []Track
}

type Options struct {
	Explicit bool
	Exclude  []Track
	Limit    int
	Random   func() float64
}

var moodQueries = map[string]string{
	"calm": "ambient chill", "energetic": "electronic dance", "happy": "indie pop", "sad": "melancholic acoustic", "any": "indie electronic",
}
var activityQueries = map[string]string{
	"wake": "morning music", "road": "driving music", "work": "focus music", "workout": "workout music", "sleep": "sleep music",
}

func Defaults(p Preferences) Preferences {
	switch p.Diversity {
	case "favorite", "familiar":
		p.Diversity = "favorite"
	case "unknown", "discover":
		p.Diversity = "unknown"
	case "popular":
		p.Diversity = "popular"
	default:
		p.Diversity = "any"
	}
	if _, ok := activityQueries[p.Activity]; !ok {
		p.Activity = "any"
	}
	if _, ok := moodQueries[p.Mood]; !ok {
		p.Mood = "any"
	}
	if p.Language != "any" && p.Language != "russian" && p.Language != "foreign" && p.Language != "instrumental" {
		p.Language = "any"
	}
	return p
}

func Query(p Preferences, likes []Track, ctx Context, round int) string {
	p = Defaults(p)
	if p.Activity == "any" && p.Mood == "any" {
		if artist := clip(ctx.Artist); artist != "" {
			return artist
		}
		if genre := clip(ctx.Genre); genre != "" {
			return genre
		}
	}
	parts := []string{}
	if query, ok := activityQueries[p.Activity]; ok {
		parts = append(parts, query)
	}
	if p.Mood != "any" {
		parts = append(parts, moodQueries[p.Mood])
	}
	if p.Language == "instrumental" {
		parts = append(parts, "instrumental")
	}
	if len(parts) == 0 {
		if round < 0 {
			round = 0
		}
		if len(likes) > 0 && p.Diversity != "unknown" {
			if artist := clip(likes[round%len(likes)].Artist); artist != "" {
				return artist
			}
		}
		parts = append(parts, moodQueries["any"])
	}
	query := strings.Join(parts, " ")
	if p.Language == "russian" {
		return "русская музыка " + query
	}
	return query
}

func Build(candidates []Track, library Library, preferences Preferences, opts Options) []Track {
	preferences = Defaults(preferences)
	if opts.Limit <= 0 {
		opts.Limit = 30
	}
	if opts.Random == nil {
		opts.Random = func() float64 { return 0 }
	}
	denied := map[string]struct{}{}
	for _, track := range append(append([]Track{}, library.Dislikes...), opts.Exclude...) {
		denied[key(track)] = struct{}{}
	}
	familiar := map[string]struct{}{}
	for _, track := range append(append([]Track{}, library.Likes...), library.History...) {
		familiar[key(track)] = struct{}{}
	}
	favorites := map[string]struct{}{}
	for _, track := range library.Likes {
		id := track.ArtistID
		if id == "" {
			id = track.Artist
		}
		favorites[id] = struct{}{}
	}
	known := []Track{}
	if preferences.Diversity == "favorite" {
		known = append(append([]Track{}, library.Likes...), library.History...)
	}
	hasFamiliar := len(familiar) > 0
	eligible := []Track{}
	for _, track := range unique(append(known, candidates...)) {
		if track.Access == "blocked" {
			continue
		}
		if _, skip := denied[key(track)]; skip {
			continue
		}
		if !opts.Explicit && track.Explicit {
			continue
		}
		_, seen := familiar[key(track)]
		if preferences.Diversity == "favorite" && hasFamiliar && !seen {
			continue
		}
		if preferences.Diversity == "unknown" && seen {
			continue
		}
		cyrillic := hasCyrillic(track.Title + " " + track.Artist)
		if preferences.Language == "russian" && !cyrillic {
			continue
		}
		if preferences.Language == "foreign" && cyrillic {
			continue
		}
		eligible = append(eligible, track)
	}
	if preferences.Language == "instrumental" {
		quiet := []Track{}
		for _, track := range eligible {
			if instrumental(track.Title + " " + track.Genre) {
				quiet = append(quiet, track)
			}
		}
		if len(quiet) > 0 {
			eligible = quiet
		}
	}
	type scored struct {
		track Track
		score float64
	}
	ranked := make([]scored, len(eligible))
	for i, track := range eligible {
		id := track.ArtistID
		if id == "" {
			id = track.Artist
		}
		score := opts.Random()
		if _, ok := favorites[id]; ok {
			score += 0.3
		}
		if preferences.Diversity == "popular" {
			score += math.Log10(float64(track.PlaybackCount) + 1)
		}
		ranked[i] = scored{track, score}
	}
	for i := 1; i < len(ranked); i++ {
		item := ranked[i]
		j := i
		for j > 0 && ranked[j-1].score < item.score {
			ranked[j] = ranked[j-1]
			j--
		}
		ranked[j] = item
	}
	ordered := make([]Track, len(ranked))
	for i, item := range ranked {
		ordered[i] = item.track
	}
	result := []Track{}
	for len(ordered) > 0 && len(result) < opts.Limit {
		index := 0
		if len(result) > 0 {
			for i, track := range ordered {
				if track.Artist != result[len(result)-1].Artist {
					index = i
					break
				}
			}
		}
		result = append(result, ordered[index])
		ordered = append(ordered[:index], ordered[index+1:]...)
	}
	return result
}

func FromSoundCloud(raw json.RawMessage) ([]Track, error) {
	var wrapped struct {
		Collection []json.RawMessage `json:"collection"`
	}
	items := []json.RawMessage{}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Collection != nil {
		items = wrapped.Collection
	} else if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	tracks := []Track{}
	for _, item := range items {
		var source struct {
			URN, Title, Genre, Access string
			ID                        json.RawMessage
			MetadataArtist            string `json:"metadata_artist"`
			ArtworkURL                string `json:"artwork_url"`
			PermalinkURL              string `json:"permalink_url"`
			Duration                  int
			Explicit                  bool
			PlaybackCount             int `json:"playback_count"`
			User                      struct {
				URN, Username string
				ID            json.RawMessage
				AvatarURL     string `json:"avatar_url"`
			}
			Publisher struct {
				Artist string `json:"artist"`
			} `json:"publisher_metadata"`
		}
		if json.Unmarshal(item, &source) != nil || strings.TrimSpace(source.Title) == "" {
			continue
		}
		id := firstText(source.URN, rawText(source.ID))
		artistID := firstText(source.User.URN, rawText(source.User.ID))
		artist := firstText(source.MetadataArtist, source.Publisher.Artist, source.User.Username)
		if artist == "" {
			artist = "Исполнитель"
		}
		access := source.Access
		if access == "" {
			access = "playable"
		}
		artwork := source.ArtworkURL
		if artwork == "" {
			artwork = source.User.AvatarURL
		}
		tracks = append(tracks, Track{
			ID: id, Source: "soundcloud", Title: clip(source.Title), Artist: artist, ArtistID: artistID,
			Artwork: artwork, Duration: float64(source.Duration) / 1000, Explicit: source.Explicit,
			Access: access, Permalink: source.PermalinkURL, Genre: source.Genre, PlaybackCount: source.PlaybackCount,
		})
	}
	return tracks, nil
}

func key(track Track) string { return track.Source + ":" + track.ID }

func unique(tracks []Track) []Track {
	seen := map[string]struct{}{}
	out := make([]Track, 0, len(tracks))
	for _, track := range tracks {
		if _, ok := seen[key(track)]; ok {
			continue
		}
		seen[key(track)] = struct{}{}
		out = append(out, track)
	}
	return out
}

func instrumental(value string) bool {
	value = strings.ToLower(value)
	for _, part := range []string{"instrumental", "ambient", "piano", "classical", "оркестр", "без слов"} {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}

func hasCyrillic(value string) bool {
	for _, r := range strings.ToLower(value) {
		if r == 'ё' || (r >= 'а' && r <= 'я') {
			return true
		}
	}
	return false
}

func clip(value string) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= 200 {
		return value
	}
	runes := []rune(value)
	return string(runes[:200])
}

func firstText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func rawText(raw json.RawMessage) string {
	text := strings.Trim(string(raw), `"`)
	if text == "null" || text == "" {
		return ""
	}
	return text
}
