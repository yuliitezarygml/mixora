package library

import (
	"strings"
	"testing"

	"github.com/iulian/soundcloud-go/internal/music"
)

func TestNormalizePreferenceInputBuildsCompactCanonicalSnapshot(t *testing.T) {
	t.Parallel()

	normalized, err := NormalizePreferenceInput(PreferenceInput{
		IdempotencyKey: "  preference-42  ",
		Preference:     PreferenceLiked,
		Track: music.Track{
			Source:        " SoundCloud ",
			ID:            "soundcloud:tracks:1534086151",
			Title:         "  A Song  ",
			Artist:        "  The Artist  ",
			ArtistID:      "soundcloud:users:1030983220",
			Artwork:       " https://example.test/art.jpg ",
			Duration:      180.5,
			Explicit:      true,
			Access:        " PLAYABLE ",
			Permalink:     " https://example.test/song ",
			Description:   "This deliberately must not enter the snapshot.",
			PlaybackCount: 987,
		},
	})
	if err != nil {
		t.Fatalf("NormalizePreferenceInput() error = %v", err)
	}

	if normalized.IdempotencyKey != "preference-42" {
		t.Fatalf("idempotency key = %q", normalized.IdempotencyKey)
	}
	if normalized.Preference != PreferenceLiked {
		t.Fatalf("preference = %q", normalized.Preference)
	}
	if got, want := normalized.Track.Source, "soundcloud"; got != want {
		t.Fatalf("snapshot source = %q, want %q", got, want)
	}
	if got, want := normalized.Track.ID, "1534086151"; got != want {
		t.Fatalf("snapshot id = %q, want %q", got, want)
	}
	if got, want := normalized.Track.ArtistID, "1030983220"; got != want {
		t.Fatalf("snapshot artist id = %q, want %q", got, want)
	}
	if normalized.Track.Title != "A Song" || normalized.Track.Artist != "The Artist" || normalized.Track.Access != "playable" {
		t.Fatalf("normalized snapshot = %#v", normalized.Track)
	}
	if normalized.Track.Description != "" || normalized.Track.PlaybackCount != 0 {
		t.Fatalf("snapshot kept non-compact metadata: %#v", normalized.Track)
	}
}

func TestNormalizePreferenceInputRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	validTrack := music.Track{Source: "soundcloud", ID: "42", Title: "Song", Artist: "Artist", Duration: 1}
	tests := []struct {
		name  string
		input PreferenceInput
	}{
		{
			name:  "missing idempotency key",
			input: PreferenceInput{Preference: PreferenceLiked, Track: validTrack},
		},
		{
			name:  "unsupported preference",
			input: PreferenceInput{IdempotencyKey: "key", Preference: Preference("boosted"), Track: validTrack},
		},
		{
			name:  "missing canonical track id",
			input: PreferenceInput{IdempotencyKey: "key", Preference: PreferenceLiked, Track: music.Track{Source: "soundcloud", Title: "Song", Artist: "Artist"}},
		},
		{
			name:  "missing title",
			input: PreferenceInput{IdempotencyKey: "key", Preference: PreferenceLiked, Track: music.Track{Source: "soundcloud", ID: "42", Artist: "Artist"}},
		},
		{
			name:  "negative duration",
			input: PreferenceInput{IdempotencyKey: "key", Preference: PreferenceLiked, Track: music.Track{Source: "soundcloud", ID: "42", Title: "Song", Artist: "Artist", Duration: -1}},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NormalizePreferenceInput(test.input)
			if err == nil {
				t.Fatal("NormalizePreferenceInput() error = nil, want validation error")
			}
		})
	}
}

func TestPreferenceRequestFingerprintIsStableAndChangesWithDesiredState(t *testing.T) {
	t.Parallel()

	base := PreferenceInput{
		IdempotencyKey: "same-key",
		Preference:     PreferenceLiked,
		Track:          music.Track{Source: "soundcloud", ID: "42", Title: "Song", Artist: "Artist"},
	}
	first, err := NormalizePreferenceInput(base)
	if err != nil {
		t.Fatalf("normalize first input: %v", err)
	}
	second, err := NormalizePreferenceInput(base)
	if err != nil {
		t.Fatalf("normalize second input: %v", err)
	}
	if got, want := string(PreferenceRequestFingerprint(first)), string(PreferenceRequestFingerprint(second)); got != want {
		t.Fatalf("same request fingerprints differ: %q != %q", got, want)
	}

	second.Preference = PreferenceDisliked
	if got, unchanged := string(PreferenceRequestFingerprint(second)), string(PreferenceRequestFingerprint(first)); got == unchanged {
		t.Fatal("different desired preference has the same fingerprint")
	}
}

func TestCompactTrackSnapshotDoesNotAcceptControlCharacters(t *testing.T) {
	t.Parallel()

	_, err := NormalizePreferenceInput(PreferenceInput{
		IdempotencyKey: "key",
		Preference:     PreferenceNeutral,
		Track:          music.Track{Source: "sound\x00cloud", ID: "42", Title: "Song", Artist: "Artist"},
	})
	if err == nil || !strings.Contains(err.Error(), "track source") {
		t.Fatalf("NormalizePreferenceInput() error = %v, want track source validation error", err)
	}
}

func TestPreferenceLockKeyKeepsFieldBoundariesDistinct(t *testing.T) {
	t.Parallel()
	first := preferenceLockKey("track", "user:1", "soundcloud", "42")
	second := preferenceLockKey("track", "user", "1:soundcloud", "42")
	if first == second {
		t.Fatalf("preference lock keys collided: %q", first)
	}
}

func TestPartitionRecommendationTracksKeepsNeutralRetractions(t *testing.T) {
	t.Parallel()
	track := func(id string) TrackSnapshot {
		return TrackSnapshot{Source: "soundcloud", ID: id, Title: "Track " + id, Artist: "Artist"}
	}
	likes, dislikes, neutral, err := partitionRecommendationTracks([]TrackPreference{
		{Track: track("1"), Preference: PreferenceLiked},
		{Track: track("2"), Preference: PreferenceDisliked},
		{Track: track("3"), Preference: PreferenceNeutral},
	})
	if err != nil {
		t.Fatalf("partitionRecommendationTracks() error = %v", err)
	}
	if len(likes) != 1 || likes[0].ID != "1" || len(dislikes) != 1 || dislikes[0].ID != "2" || len(neutral) != 1 || neutral[0].ID != "3" {
		t.Fatalf("partitioned preferences = likes=%#v dislikes=%#v neutral=%#v", likes, dislikes, neutral)
	}
}
