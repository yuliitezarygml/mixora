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
