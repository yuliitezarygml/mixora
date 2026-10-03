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
	content       ContentBased
	preferences   PreferenceSource
	history       HistorySource
}

type Collaborative interface {
	UpsertUser(context.Context, string) error
	UpsertItems(context.Context, []music.Track) error
	Recommend(context.Context, string, int) ([]string, error)
}

type ContentBased interface {
	Recommend(context.Context, string, []string, string, int) ([]string, error)
	Version() string
}

// PreferenceSource supplies the server-owned current desired state. It
// is deliberately a narrow read interface so recommendation stays independent
// from the snapshot-library implementation and callers can still use local
// optimistic state while an offline mutation waits to be delivered.
type PreferenceSource interface {
	RecommendationTracks(context.Context, string) (likes []music.Track, dislikes []music.Track, neutral []music.Track, err error)
}

// HistorySource returns bounded, server-owned recent tracks. It is separate
// from ContentBased because user history is an application fact, not an ML
// implementation detail.
type HistorySource interface {
	RecommendationHistory(context.Context, string, int) ([]music.Track, error)
}

type Option func(*Service)

func WithCollaborative(value Collaborative) Option {
	return func(service *Service) { service.collaborative = value }
}

func WithCatalog(value Catalog) Option {
	return func(service *Service) { service.catalog = value }
}

func WithContentBased(value ContentBased) Option {
	return func(service *Service) { service.content = value }
}

func WithPreferenceSource(value PreferenceSource) Option {
	return func(service *Service) { service.preferences = value }
}

func WithHistorySource(value HistorySource) Option {
	return func(service *Service) { service.history = value }
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
	request = canonicalRequest(request)
	request = s.withStoredPreferences(ctx, request)
	request = s.withStoredHistory(ctx, request)
	query := BuildQuery(request.Preferences, request.Context, request.Likes, request.Round)
	response, err := s.soundcloud.SearchTracks(ctx, query, soundcloud.SearchOptions{Limit: 50})
	if err != nil {
		return Result{}, fmt.Errorf("search recommendation candidates: %w", err)
	}
	collaborativeIDs := s.personalizedIDs(ctx, request.UserID)
	contentIDs := s.contentIDs(ctx, request.UserID, tasteSeedKeys(request), query)
	personalizedIDs := BlendRankings(collaborativeIDs, contentIDs, 100)
	personalized := s.catalogTracks(ctx, personalizedIDs)
	trusted := make([]music.Track, 0, len(personalized)+len(response.Collection))
	trusted = append(trusted, personalized...)
	for _, raw := range response.Collection {
		trusted = append(trusted, music.FromSoundCloud(raw))
	}
	candidates := make([]music.Track, 0, len(trusted)+len(request.Seeds))
	candidates = append(candidates, trusted...)
	candidates = append(candidates, request.Seeds...)
	if s.catalog != nil {
		_ = s.catalog.Save(ctx, trusted)
	}
	if s.collaborative != nil {
		_ = s.collaborative.UpsertItems(ctx, trusted)
	}
	pool := Rank(candidates, request, 100)
	tracks, _ := Personalize(pool, personalizedIDs, 30)
	model := modelVersion(
		countMatches(pool, collaborativeIDs) > 0,
		s.contentVersion(countMatches(pool, contentIDs) > 0),
	)
	return Result{Tracks: tracks, ModelVersion: model, Reason: query}, nil
}

func canonicalRequest(request Request) Request {
	request.Exclude = canonicalTracks(request.Exclude)
	request.Likes = canonicalTracks(request.Likes)
	request.History = canonicalTracks(request.History)
	request.Dislikes = canonicalTracks(request.Dislikes)
	request.Seeds = canonicalTracks(request.Seeds)
	return request
}

func canonicalTracks(tracks []music.Track) []music.Track {
	result := make([]music.Track, len(tracks))
	for index, track := range tracks {
		result[index] = music.CanonicalTrack(track)
	}
	return result
}

func (s *Service) withStoredPreferences(ctx context.Context, request Request) Request {
	if s.preferences == nil || strings.TrimSpace(request.UserID) == "" {
		return request
	}
	likes, dislikes, neutral, err := s.preferences.RecommendationTracks(ctx, request.UserID)
	if err != nil {
		// Recommendations remain available if the preference read is briefly
		// unavailable. The caller's local optimistic state is still useful.
		return request
	}
	return mergeStoredPreferences(request, canonicalTracks(likes), canonicalTracks(dislikes), canonicalTracks(neutral))
}

// mergeStoredPreferences gives the persisted desired state the last word for
// a matching track while preserving unrelated, not-yet-synchronised local
// choices. This keeps a second device correct without making offline UX wait
// for a round trip.
func mergeStoredPreferences(request Request, storedLikes, storedDislikes, storedNeutral []music.Track) Request {
	type desiredState uint8
	const (
		stateNeutral desiredState = iota
		stateLiked
		stateDisliked
	)
	type choice struct {
		track music.Track
		state desiredState
	}
	capacity := len(request.Likes) + len(request.Dislikes) + len(storedLikes) + len(storedDislikes) + len(storedNeutral)
	choices := make(map[string]choice, capacity)
	order := make([]string, 0, capacity)
	apply := func(tracks []music.Track, state desiredState) {
		for _, track := range tracks {
			if track.Source == "" || track.ID == "" {
				continue
			}
			key := track.Key()
			if _, exists := choices[key]; !exists {
				order = append(order, key)
			}
			choices[key] = choice{track: track, state: state}
		}
	}
	apply(request.Likes, stateLiked)
	apply(request.Dislikes, stateDisliked)
	apply(storedLikes, stateLiked)
	apply(storedDislikes, stateDisliked)
	apply(storedNeutral, stateNeutral)

	request.Likes = make([]music.Track, 0, len(choices))
	request.Dislikes = make([]music.Track, 0, len(choices))
	for _, key := range order {
		choice := choices[key]
		switch choice.state {
		case stateLiked:
			request.Likes = append(request.Likes, choice.track)
		case stateDisliked:
			request.Dislikes = append(request.Dislikes, choice.track)
		}
	}
	return request
}

func (s *Service) withStoredHistory(ctx context.Context, request Request) Request {
	if s.history == nil || strings.TrimSpace(request.UserID) == "" {
		return request
	}
	stored, err := s.history.RecommendationHistory(ctx, request.UserID, 100)
	if err != nil {
		// The current client request can still carry a freshly heard track while
		// the durable history store is temporarily unavailable.
		return request
	}
	request.History = mergeStoredHistory(canonicalTracks(stored), request.History, 100)
	return request
}

// mergeStoredHistory preserves durable recency as the primary order, then
// adds unknown local tracks as an optimistic offline fallback. Provider keys
// are opaque, so de-duplication is only by their canonical source/id pair.
func mergeStoredHistory(stored, local []music.Track, limit int) []music.Track {
	if limit <= 0 {
		return nil
	}
	result := make([]music.Track, 0, min(limit, len(stored)+len(local)))
	seen := make(map[string]bool, cap(result))
	for _, group := range [][]music.Track{stored, local} {
		for _, track := range group {
			if track.Source == "" || track.ID == "" || seen[track.Key()] {
				continue
			}
			seen[track.Key()] = true
			result = append(result, track)
			if len(result) == limit {
				return result
			}
		}
	}
	return result
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

func (s *Service) contentIDs(ctx context.Context, userID string, seedKeys []string, query string) []string {
	if s.content == nil {
		return nil
	}
	ids, err := s.content.Recommend(ctx, userID, seedKeys, query, 100)
	if err != nil {
		return nil
	}
	return ids
}

func (s *Service) contentVersion(active bool) string {
	if !active || s.content == nil {
		return ""
	}
	return s.content.Version()
}

func tasteSeedKeys(request Request) []string {
	denied := make(map[string]bool, len(request.Dislikes)+len(request.Exclude))
	for _, track := range append(append([]music.Track{}, request.Dislikes...), request.Exclude...) {
		if track.Source != "" && track.ID != "" {
			denied[track.Key()] = true
		}
	}
	tracks := make([]music.Track, 0, len(request.Likes)+len(request.Seeds)+len(request.History))
	tracks = append(tracks, request.Likes...)
	tracks = append(tracks, request.Seeds...)
	tracks = append(tracks, request.History...)
	result := make([]string, 0, min(50, len(tracks)))
	seen := make(map[string]bool, len(tracks))
	for _, track := range tracks {
		key := track.Key()
		if track.Source == "" || track.ID == "" || seen[key] || denied[key] {
			continue
		}
		seen[key] = true
		result = append(result, key)
		if len(result) == 50 {
			break
		}
	}
	return result
}

// BlendRankings keeps collaborative discovery dominant while reserving every
// third position for a content-similar item. Duplicate opaque track keys are
// removed without parsing their provider-specific shape.
func BlendRankings(collaborative, content []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	result := make([]string, 0, min(limit, len(collaborative)+len(content)))
	seen := make(map[string]bool, cap(result))
	appendNext := func(source []string, index *int) {
		for *index < len(source) && len(result) < limit {
			value := source[*index]
			*index = *index + 1
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			result = append(result, value)
			return
		}
	}
	collaborativeIndex, contentIndex := 0, 0
	for len(result) < limit && (collaborativeIndex < len(collaborative) || contentIndex < len(content)) {
		appendNext(collaborative, &collaborativeIndex)
		appendNext(collaborative, &collaborativeIndex)
		appendNext(content, &contentIndex)
	}
	return result
}

func countMatches(tracks []music.Track, ids []string) int {
	available := make(map[string]bool, len(tracks))
	for _, track := range tracks {
		available[track.Key()] = true
	}
	count := 0
	matched := make(map[string]bool, len(ids))
	for _, id := range ids {
		if available[id] && !matched[id] {
			matched[id] = true
			count++
		}
	}
	return count
}

func modelVersion(collaborative bool, contentVersion string) string {
	parts := make([]string, 0, 3)
	if collaborative {
		parts = append(parts, "gorse-v1")
	}
	if contentVersion != "" {
		parts = append(parts, contentVersion)
	}
	parts = append(parts, "rules-v0")
	return strings.Join(parts, "+")
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
