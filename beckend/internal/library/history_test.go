package library

import (
	"strings"
	"testing"
	"time"

	"github.com/iulian/soundcloud-go/internal/music"
)

func TestNormalizeHistoryInputBuildsCanonicalCompactTrack(t *testing.T) {
	t.Parallel()
	occurredAt := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.FixedZone("UTC+3", 3*60*60))
	input, err := NormalizeHistoryInput(HistoryInput{
		IdempotencyKey: " history-1 ",
		OccurredAt:     occurredAt,
		Track: music.Track{
			Source: " SoundCloud ", ID: "soundcloud:tracks:42", Title: " Song ", Artist: " Artist ",
			Description: "must not be persisted", PlaybackCount: 42,
		},
	})
	if err != nil {
		t.Fatalf("NormalizeHistoryInput() error = %v", err)
	}
	if input.IdempotencyKey != "history-1" || input.Track.Source != "soundcloud" || input.Track.ID != "42" {
		t.Fatalf("normalized input = %#v", input)
	}
	if input.Track.Description != "" || input.Track.PlaybackCount != 0 {
		t.Fatalf("history input retained non-compact data: %#v", input.Track)
	}
	if !input.OccurredAt.Equal(occurredAt.UTC()) {
		t.Fatalf("occurred_at = %s, want UTC %s", input.OccurredAt, occurredAt.UTC())
	}
}

func TestNormalizeHistoryInputRejectsMissingOrInvalidOccurrence(t *testing.T) {
	t.Parallel()
	base := HistoryInput{
		IdempotencyKey: "history-1",
		Track:          music.Track{Source: "music", ID: "42", Title: "Song", Artist: "Artist"},
	}
	if _, err := NormalizeHistoryInput(base); err == nil || !strings.Contains(err.Error(), "occurred_at") {
		t.Fatalf("missing occurred_at error = %v", err)
	}
	base.OccurredAt = time.Now().Add(48 * time.Hour)
	if _, err := NormalizeHistoryInput(base); err == nil || !strings.Contains(err.Error(), "occurred_at") {
		t.Fatalf("future occurred_at error = %v", err)
	}
}

func TestHistoryFingerprintIncludesOccurrenceAndNotIdempotencyKey(t *testing.T) {
	t.Parallel()
	base := HistoryInput{
		IdempotencyKey: "first", OccurredAt: time.Date(2026, time.October, 3, 10, 0, 0, 0, time.UTC),
		Track: music.Track{Source: "music", ID: "42", Title: "Song", Artist: "Artist"},
	}
	first, err := NormalizeHistoryInput(base)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.IdempotencyKey = "second"
	if got, want := string(HistoryRequestFingerprint(second)), string(HistoryRequestFingerprint(first)); got != want {
		t.Fatalf("idempotency key changed fingerprint: %q != %q", got, want)
	}
	second.OccurredAt = second.OccurredAt.Add(time.Minute)
	if got, unchanged := string(HistoryRequestFingerprint(second)), string(HistoryRequestFingerprint(first)); got == unchanged {
		t.Fatal("occurrence did not change fingerprint")
	}
}

func TestHistoryLockKeyKeepsFieldBoundariesDistinct(t *testing.T) {
	t.Parallel()
	if first, second := historyLockKey("track", "a:b", "c"), historyLockKey("track", "a", "b:c"); first == second {
		t.Fatalf("history lock keys collided: %q", first)
	}
}
