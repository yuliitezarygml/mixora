package recommendation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/iulian/soundcloud-go/pkg/soundcloud"
)

var ErrUnavailable = errors.New("recommendation source is unavailable")

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

type Request struct {
	UserID      string        `json:"-"`
	Preferences Preferences   `json:"preferences"`
	Context     Context       `json:"context"`
	Round       int           `json:"round"`
	Explicit    bool          `json:"explicit"`
	Exclude     []music.Track `json:"exclude"`
	Likes       []music.Track `json:"likes"`
	History     []music.Track `json:"history"`
	Dislikes    []music.Track `json:"dislikes"`
	Seeds       []music.Track `json:"seeds"`
}

type Result struct {
	Tracks       []music.Track `json:"tracks"`
	ModelVersion string        `json:"model_version"`
	Reason       string        `json:"reason"`
}

type Service struct {
	soundcloud    *soundcloud.Client
	collaborative Collaborative
	catalog       Catalog
}

type Collaborative interface {
	UpsertUser(context.Context, string) error
	UpsertItems(context.Context, []music.Track) error
	Recommend(context.Context, string, int) ([]string, error)
}

type Option func(*Service)

func WithCollaborative(value Collaborative) Option {
	return func(service *Service) { service.collaborative = value }
}

func WithCatalog(value Catalog) Option {
	return func(service *Service) { service.catalog = value }
}

func New(sc *soundcloud.Client, options ...Option) *Service {
	service := &Service{soundcloud: sc}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Recommend(ctx context.Context, request Request) (Result, error) {
	if s.soundcloud == nil {
		return Result{}, ErrUnavailable
	}
	query := BuildQuery(request.Preferences, request.Context, request.Likes, request.Round)
	response, err := s.soundcloud.SearchTracks(ctx, query, soundcloud.SearchOptions{Limit: 50})
	if err != nil {
		return Result{}, fmt.Errorf("search recommendation candidates: %w", err)
	}
	personalizedIDs := s.personalizedIDs(ctx, request.UserID)
	personalized := s.catalogTracks(ctx, personalizedIDs)
	candidates := make([]music.Track, 0, len(personalized)+len(response.Collection)+len(request.Seeds))
	candidates = append(candidates, personalized...)
	for _, raw := range response.Collection {
		candidates = append(candidates, music.FromSoundCloud(raw))
	}
	candidates = append(candidates, request.Seeds...)
	if s.catalog != nil {
		_ = s.catalog.Save(ctx, candidates)
	}
	if s.collaborative != nil {
		_ = s.collaborative.UpsertItems(ctx, candidates)
	}
	pool := Rank(candidates, request, 100)
	tracks, matched := Personalize(pool, personalizedIDs, 30)
	model := "rules-v0"
	if matched > 0 {
		model = "gorse-v1+rules-v0"
	}
	return Result{Tracks: tracks, ModelVersion: model, Reason: query}, nil
}

func (s *Service) personalizedIDs(ctx context.Context, userID string) []string {
	if s.collaborative == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	if err := s.collaborative.UpsertUser(ctx, userID); err != nil {
		return nil
	}
	ids, err := s.collaborative.Recommend(ctx, userID, 100)
	if err != nil {
		return nil
	}
	return ids
}

func (s *Service) catalogTracks(ctx context.Context, ids []string) []music.Track {
	if s.catalog == nil || len(ids) == 0 {
		return nil
	}
	tracks, err := s.catalog.Find(ctx, ids)
	if err != nil {
		return nil
	}
	return tracks
}

// Personalize promotes items in the collaborative order, keeps the
// deterministic rules order for everything else and retains artist variety.
func Personalize(tracks []music.Track, ids []string, limit int) ([]music.Track, int) {
	if limit <= 0 || len(tracks) == 0 {
		return nil, 0
	}
	byKey := make(map[string]music.Track, len(tracks))
	for _, track := range tracks {
		byKey[track.Key()] = track
	}
	ordered := make([]music.Track, 0, len(tracks))
	used := make(map[string]bool, len(tracks))
	matched := 0
	for _, id := range ids {
		track, exists := byKey[id]
		if !exists || used[id] {
			continue
		}
		used[id] = true
		ordered = append(ordered, track)
		matched++
	}
	for _, track := range tracks {
		if !used[track.Key()] {
			used[track.Key()] = true
			ordered = append(ordered, track)
		}
	}
	result := make([]music.Track, 0, min(limit, len(ordered)))
	for len(ordered) > 0 && len(result) < limit {
		pick := 0
		if len(result) > 0 {
			for index := range ordered {
				if ordered[index].Artist != result[len(result)-1].Artist {
					pick = index
					break
				}
			}
		}
		result = append(result, ordered[pick])
		ordered = append(ordered[:pick], ordered[pick+1:]...)
	}
	return result, matched
}

func BuildQuery(preferences Preferences, context Context, likes []music.Track, round int) string {
	if context.Artist != "" && preferences.Activity == "any" && preferences.Mood == "any" {
		return context.Artist
	}
	if context.Genre != "" && preferences.Activity == "any" && preferences.Mood == "any" {
		return context.Genre
	}
	activity := map[string]string{
		"wake": "morning music", "road": "driving music", "work": "focus music",
		"workout": "workout music", "sleep": "sleep music",
	}
	mood := map[string]string{
		"calm": "ambient chill", "energetic": "electronic dance",
		"happy": "indie pop", "sad": "melancholic acoustic",
	}
	parts := make([]string, 0, 3)
	if value := activity[preferences.Activity]; value != "" {
		parts = append(parts, value)
	}
	if value := mood[preferences.Mood]; value != "" {
		parts = append(parts, value)
	}
	if preferences.Language == "instrumental" {
		parts = append(parts, "instrumental")
	}
	if len(parts) == 0 && len(likes) > 0 && preferences.Diversity != "unknown" {
		parts = append(parts, likes[positiveMod(round, len(likes))].Artist)
	}
	if len(parts) == 0 {
		parts = append(parts, "indie electronic")
	}
	if preferences.Language == "russian" {
		parts = append([]string{"русская музыка"}, parts...)
	}
	return strings.Join(parts, " ")
}

func Rank(candidates []music.Track, request Request, limit int) []music.Track {
	denied := make(map[string]bool)
	for _, track := range append(append([]music.Track{}, request.Dislikes...), request.Exclude...) {
		denied[track.Key()] = true
	}
	familiar := make(map[string]bool)
	favoriteArtists := make(map[string]bool)
	for _, track := range append(append([]music.Track{}, request.Likes...), request.History...) {
		familiar[track.Key()] = true
		favoriteArtists[track.ArtistID+"|"+track.Artist] = true
	}
	type scored struct {
		track music.Track
		score float64
	}
	seen := make(map[string]bool)
	pool := make([]scored, 0, len(candidates))
	for index, track := range candidates {
		key := track.Key()
		if seen[key] || denied[key] || track.Access == "blocked" || (!request.Explicit && track.Explicit) {
			continue
		}
		seen[key] = true
		if request.Preferences.Diversity == "favorite" && len(familiar) > 0 && !familiar[key] {
			continue
		}
		if request.Preferences.Diversity == "unknown" && familiar[key] {
			continue
		}
		cyrillic := containsCyrillic(track.Title + " " + track.Artist)
		if request.Preferences.Language == "russian" && !cyrillic || request.Preferences.Language == "foreign" && cyrillic {
			continue
		}
		score := -float64(index) / 100
		if favoriteArtists[track.ArtistID+"|"+track.Artist] {
			score += .4
		}
		if request.Preferences.Diversity == "popular" {
			score += math.Log10(float64(track.PlaybackCount) + 1)
		}
		pool = append(pool, scored{track: track, score: score})
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].score > pool[j].score })
	result := make([]music.Track, 0, min(limit, len(pool)))
	for len(pool) > 0 && len(result) < limit {
		pick := 0
		if len(result) > 0 {
			for i := range pool {
				if pool[i].track.Artist != result[len(result)-1].Artist {
					pick = i
					break
				}
			}
		}
		result = append(result, pool[pick].track)
		pool = append(pool[:pick], pool[pick+1:]...)
	}
	return result
}

func positiveMod(value, divisor int) int {
	if divisor <= 0 {
		return 0
	}
	result := value % divisor
	if result < 0 {
		result += divisor
	}
	return result
}

func containsCyrillic(value string) bool {
	for _, char := range value {
		if char >= '\u0400' && char <= '\u04ff' {
			return true
		}
	}
	return false
}
