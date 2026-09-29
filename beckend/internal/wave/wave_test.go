package wave

import (
	"encoding/json"
	"testing"
)

func track(id string, patch Track) Track {
	patch.ID = id
	patch.Source = "soundcloud"
	if patch.Title == "" {
		patch.Title = id
	}
	if patch.Artist == "" {
		patch.Artist = "Artist"
	}
	if patch.Access == "" {
		patch.Access = "playable"
	}
	return patch
}

func TestBuildFilters(t *testing.T) {
	tracks := []Track{track("a", Track{}), track("b", Track{Access: "blocked"}), track("c", Track{}), track("d", Track{Explicit: true}), track("e", Track{})}
	result := Build(tracks, Library{Dislikes: []Track{tracks[2]}}, Preferences{}, Options{Exclude: []Track{tracks[0]}, Random: func() float64 { return 0.5 }})
	if len(result) != 1 || result[0].ID != "e" {
		t.Fatalf("got %#v", ids(result))
	}
}

func TestBuildDiversityAndLanguage(t *testing.T) {
	known, fresh := track("known", Track{}), track("new", Track{})
	library := Library{Likes: []Track{known}}
	familiar := Build([]Track{known, fresh}, library, Preferences{Diversity: "favorite"}, Options{Explicit: true, Random: func() float64 { return 0.2 }})
	if len(familiar) != 1 || familiar[0].ID != "known" {
		t.Fatalf("familiar %#v", ids(familiar))
	}
	discovered := Build([]Track{known, fresh}, library, Preferences{Diversity: "unknown"}, Options{Explicit: true, Random: func() float64 { return 0.2 }})
	if len(discovered) != 1 || discovered[0].ID != "new" {
		t.Fatalf("discover %#v", ids(discovered))
	}
	ru := track("ru", Track{Title: "Утро", Artist: "Дайте танк"})
	en := track("en", Track{Title: "Awake", Artist: "Tycho"})
	russian := Build([]Track{ru, ru, en}, Library{}, Preferences{Language: "russian"}, Options{Explicit: true})
	if len(russian) != 1 || russian[0].ID != "ru" {
		t.Fatalf("language %#v", ids(russian))
	}
}

func TestQuery(t *testing.T) {
	if got := Query(Preferences{Mood: "calm"}, nil, Context{}, 0); !contains(got, "ambient") {
		t.Fatal(got)
	}
	if got := Query(Preferences{Activity: "wake"}, nil, Context{}, 0); !contains(got, "morning") {
		t.Fatal(got)
	}
	quiet := track("quiet", Track{Title: "Piano", Genre: "instrumental", PlaybackCount: 10})
	loud := track("loud", Track{Title: "Vocal", Genre: "pop", PlaybackCount: 1000})
	popular := Build([]Track{quiet, loud}, Library{}, Preferences{Diversity: "popular"}, Options{Explicit: true, Random: func() float64 { return 0 }})
	if len(popular) != 2 || popular[0].ID != "loud" {
		t.Fatalf("popular %#v", ids(popular))
	}
	wordless := Build([]Track{quiet, loud}, Library{}, Preferences{Language: "instrumental"}, Options{Explicit: true})
	if len(wordless) != 1 || wordless[0].ID != "quiet" {
		t.Fatalf("instrumental %#v", ids(wordless))
	}
	if got := Query(Preferences{}, nil, Context{Artist: "Tycho"}, 0); got != "Tycho" {
		t.Fatal(got)
	}
}

func TestFromSoundCloud(t *testing.T) {
	raw := json.RawMessage(`{"collection":[{"urn":"soundcloud:tracks:1","title":"  Утро  ","metadata_artist":"Дайте танк","duration":2000,"user":{"urn":"soundcloud:users:9","username":"uploader"}}]}`)
	tracks, err := FromSoundCloud(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].Title != "Утро" || tracks[0].Artist != "Дайте танк" || tracks[0].Duration != 2 || tracks[0].ArtistID != "soundcloud:users:9" {
		t.Fatalf("%#v", tracks[0])
	}
}

func ids(tracks []Track) []string {
	out := make([]string, len(tracks))
	for i, track := range tracks {
		out[i] = track.ID
	}
	return out
}

func contains(value, part string) bool {
	return len(value) >= len(part) && (value == part || len(part) == 0 || (len(value) > 0 && (func() bool { return len([]rune(value)) > 0 && stringIndex(value, part) >= 0 })()))
}

func stringIndex(value, part string) int {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
