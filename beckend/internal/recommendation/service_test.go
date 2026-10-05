package recommendation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/iulian/soundcloud-go/internal/music"
)

type candidateCatalogStub struct {
	tracks []music.Track
	saved  [][]music.Track
}

func (c *candidateCatalogStub) Save(_ context.Context, tracks []music.Track) error {
	c.saved = append(c.saved, append([]music.Track(nil), tracks...))
	return nil
}

func (c *candidateCatalogStub) Find(_ context.Context, _ []string) ([]music.Track, error) {
	return nil, nil
}

func (c *candidateCatalogStub) ListCandidates(_ context.Context, _ int) ([]music.Track, error) {
	return append([]music.Track(nil), c.tracks...), nil
}

type collaborativeStub struct {
	ids []string
}

func (c collaborativeStub) UpsertUser(context.Context, string) error { return nil }

func (c collaborativeStub) UpsertItems(context.Context, []music.Track) error { return nil }

func (c collaborativeStub) Recommend(context.Context, string, int) ([]string, error) {
	return append([]string(nil), c.ids...), nil
}

func TestBuildQueryUsesContext(t *testing.T) {
	t.Parallel()
	got := BuildQuery(Preferences{Activity: "any", Mood: "any"}, Context{Artist: "Massive Attack"}, nil, 0)
	if got != "Massive Attack" {
		t.Fatalf("got %q", got)
	}
}

func TestRecommendUsesObservedMultiSourceCatalogWithoutSoundCloud(t *testing.T) {
	t.Parallel()
	catalog := &candidateCatalogStub{tracks: []music.Track{
		{Source: "spotify", ID: "preview", Title: "Spotify preview", Artist: "One", Access: "preview"},
		{Source: "youtube", ID: "video", Title: "YouTube audio", Artist: "Two", Access: "playable"},
		{Source: "bandcamp", ID: "release", Title: "Bandcamp audio", Artist: "Three", Access: "playable"},
		{Source: "spotify", ID: "connect-only", Title: "Blocked", Artist: "Four", Access: "blocked"},
	}}
	service := New(nil, WithCatalog(catalog))

	result, err := service.Recommend(context.Background(), Request{Explicit: true})
	if err != nil {
		t.Fatalf("recommend from catalog: %v", err)
	}
	if len(result.Tracks) != 3 {
		t.Fatalf("tracks = %#v, want three playable catalog tracks", result.Tracks)
	}
	seen := map[string]bool{}
	for _, track := range result.Tracks {
		seen[track.Key()] = true
		if track.Access == "blocked" {
			t.Fatalf("blocked track leaked into Wave: %#v", track)
		}
	}
	for _, key := range []string{"spotify:preview", "youtube:video", "bandcamp:release"} {
		if !seen[key] {
			t.Fatalf("catalog candidate %q missing from Wave: %#v", key, result.Tracks)
		}
	}
	if len(catalog.saved) != 0 {
		t.Fatalf("reading catalog fallback must not refresh its rows: %#v", catalog.saved)
	}
}

func TestRecommendReservesCatalogSlotsForNewProviderSources(t *testing.T) {
	t.Parallel()
	soundcloud := make([]music.Track, 0, 30)
	personalizedIDs := make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		track := music.Track{
			Source: "soundcloud", ID: fmt.Sprintf("sc-%02d", index),
			Title: fmt.Sprintf("SoundCloud %d", index), Artist: fmt.Sprintf("Artist %d", index),
			Access: "playable",
		}
		soundcloud = append(soundcloud, track)
		personalizedIDs = append(personalizedIDs, track.Key())
	}
	external := []music.Track{
		{Source: "spotify", ID: "preview", Title: "Spotify preview", Artist: "Spotify artist", Access: "preview"},
		{Source: "youtube", ID: "video", Title: "YouTube audio", Artist: "YouTube artist", Access: "playable"},
		{Source: "bandcamp", ID: "release", Title: "Bandcamp audio", Artist: "Bandcamp artist", Access: "playable"},
	}
	catalog := &candidateCatalogStub{tracks: append(external, soundcloud...)}
	service := New(nil, WithCatalog(catalog), WithCollaborative(collaborativeStub{ids: personalizedIDs}))

	result, err := service.Recommend(context.Background(), Request{UserID: "listener", Explicit: true})
	if err != nil {
		t.Fatalf("recommend: %v", err)
	}
	if len(result.Tracks) != 30 {
		t.Fatalf("tracks length = %d, want 30", len(result.Tracks))
	}
	seen := make(map[string]bool, len(result.Tracks))
	for _, track := range result.Tracks {
		seen[track.Key()] = true
	}
	for _, key := range []string{"spotify:preview", "youtube:video", "bandcamp:release"} {
		if !seen[key] {
			t.Fatalf("catalog source %q was starved by a one-source collaborative result: %#v", key, result.Tracks)
		}
	}
}

func TestRecommendIsUnavailableOnlyWithoutAnyCandidateSource(t *testing.T) {
	t.Parallel()
	_, err := New(nil).Recommend(context.Background(), Request{Explicit: true})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestRankExcludesDislikesAndSpreadsArtists(t *testing.T) {
	t.Parallel()
	candidates := []music.Track{
		{ID: "1", Source: "soundcloud", Artist: "A", Access: "playable"},
		{ID: "2", Source: "soundcloud", Artist: "A", Access: "playable"},
		{ID: "3", Source: "soundcloud", Artist: "B", Access: "playable"},
	}
	got := Rank(candidates, Request{Explicit: true, Dislikes: []music.Track{{ID: "1", Source: "soundcloud"}}}, 10)
	if len(got) != 2 || got[0].ID != "2" || got[1].ID != "3" {
		t.Fatalf("unexpected ranking: %#v", got)
	}
}

func TestPersonalizePromotesKnownItemsAndKeepsFallback(t *testing.T) {
	t.Parallel()
	tracks := []music.Track{
		{ID: "1", Source: "soundcloud", Artist: "A"},
		{ID: "2", Source: "soundcloud", Artist: "B"},
		{ID: "3", Source: "soundcloud", Artist: "C"},
	}
	got, matched := Personalize(tracks, []string{"soundcloud:3", "missing:7"}, 3)
	if matched != 1 {
		t.Fatalf("matched = %d", matched)
	}
	if len(got) != 3 || got[0].ID != "3" || got[1].ID != "1" || got[2].ID != "2" {
		t.Fatalf("personalized order = %#v", got)
	}
}

func TestBlendRankingsReservesEveryThirdPositionForContent(t *testing.T) {
	t.Parallel()
	got := BlendRankings(
		[]string{"track:1", "track:2", "track:3", "track:4"},
		[]string{"track:c1", "track:2", "track:c2"},
		7,
	)
	want := []string{"track:1", "track:2", "track:c1", "track:3", "track:4", "track:c2"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestModelVersionDescribesActiveLayers(t *testing.T) {
	t.Parallel()
	if got := modelVersion(false, ""); got != "rules-v0" {
		t.Fatalf("rules model = %q", got)
	}
	if got := modelVersion(true, "embeddinggemma-q4-768-doc-v1"); got != "gorse-v1+embeddinggemma-q4-768-doc-v1+rules-v0" {
		t.Fatalf("hybrid model = %q", got)
	}
}

func TestMergeStoredPreferencesOverridesStaleClientState(t *testing.T) {
	t.Parallel()
	track := func(id string) music.Track {
		return music.Track{Source: "soundcloud", ID: id, Title: "Track " + id, Artist: "Artist"}
	}
	merged := mergeStoredPreferences(Request{
		Likes:    []music.Track{track("1")},
		Dislikes: []music.Track{track("2")},
	}, []music.Track{track("2")}, []music.Track{track("1")}, nil)
	if len(merged.Likes) != 1 || merged.Likes[0].ID != "2" {
		t.Fatalf("likes = %#v, want persisted like for track 2", merged.Likes)
	}
	if len(merged.Dislikes) != 1 || merged.Dislikes[0].ID != "1" {
		t.Fatalf("dislikes = %#v, want persisted dislike for track 1", merged.Dislikes)
	}
}

func TestMergeStoredPreferencesNeutralRetractsStaleClientState(t *testing.T) {
	t.Parallel()
	track := func(id string) music.Track {
		return music.Track{Source: "soundcloud", ID: id, Title: "Track " + id, Artist: "Artist"}
	}
	merged := mergeStoredPreferences(Request{
		Likes:    []music.Track{track("1")},
		Dislikes: []music.Track{track("2")},
	}, nil, nil, []music.Track{track("1"), track("2")})
	if len(merged.Likes) != 0 || len(merged.Dislikes) != 0 {
		t.Fatalf("neutral state must retract stale client state, got likes=%#v dislikes=%#v", merged.Likes, merged.Dislikes)
	}
}

func TestMergeStoredHistoryKeepsServerRecencyAndOfflineFallback(t *testing.T) {
	t.Parallel()
	track := func(id string) music.Track {
		return music.Track{Source: "soundcloud", ID: id, Title: "Track " + id, Artist: "Artist"}
	}
	merged := mergeStoredHistory(
		[]music.Track{track("server-new"), track("same")},
		[]music.Track{track("same"), track("local-only")},
		3,
	)
	if len(merged) != 3 || merged[0].ID != "server-new" || merged[1].ID != "same" || merged[2].ID != "local-only" {
		t.Fatalf("merged history = %#v", merged)
	}
}
