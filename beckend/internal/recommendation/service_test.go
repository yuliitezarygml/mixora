package recommendation

import (
	"testing"

	"github.com/iulian/soundcloud-go/internal/music"
)

func TestBuildQueryUsesContext(t *testing.T) {
	t.Parallel()
	got := BuildQuery(Preferences{Activity: "any", Mood: "any"}, Context{Artist: "Massive Attack"}, nil, 0)
	if got != "Massive Attack" {
		t.Fatalf("got %q", got)
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
