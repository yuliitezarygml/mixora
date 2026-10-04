package library

import (
	"bytes"
	"testing"

	"github.com/iulian/soundcloud-go/internal/music"
)

func playlistRevision(value int64) *int64 { return &value }

func TestNormalizePlaylistInputCanonicalizesTracksAndRejectsDuplicates(t *testing.T) {
	input, err := NormalizePlaylistInput(PlaylistInput{
		IdempotencyKey:   " playlist-write-1 ",
		ExpectedRevision: playlistRevision(0),
		Name:             " В дороге ",
		Description:      " Любимая музыка ",
		Pinned:           true,
		Tracks: []music.Track{{
			Source: " SoundCloud ", ID: "soundcloud:tracks:42", Title: " Track ", Artist: " Artist ",
		}},
	})
	if err != nil {
		t.Fatalf("NormalizePlaylistInput() error = %v", err)
	}
	if input.IdempotencyKey != "playlist-write-1" || input.Name != "В дороге" || input.Tracks[0].Source != "soundcloud" || input.Tracks[0].ID != "42" {
		t.Fatalf("normalized input = %#v", input)
	}

	_, err = NormalizePlaylistInput(PlaylistInput{
		IdempotencyKey: "playlist-write-2", ExpectedRevision: playlistRevision(0), Name: "Duplicates",
		Tracks: []music.Track{
			{Source: "soundcloud", ID: "42", Title: "Track", Artist: "Artist"},
			{Source: "soundcloud", ID: "soundcloud:tracks:42", Title: "Track", Artist: "Artist"},
		},
	})
	if err == nil {
		t.Fatal("NormalizePlaylistInput() accepted a duplicate canonical track")
	}
}

func TestPlaylistRequestFingerprintIncludesOrderAndAddress(t *testing.T) {
	input, err := NormalizePlaylistInput(PlaylistInput{
		IdempotencyKey: "playlist-write", ExpectedRevision: playlistRevision(0), Name: "Order",
		Tracks: []music.Track{
			{Source: "music", ID: "one", Title: "One", Artist: "Artist"},
			{Source: "music", ID: "two", Title: "Two", Artist: "Artist"},
		},
	})
	if err != nil {
		t.Fatalf("NormalizePlaylistInput() error = %v", err)
	}
	first := PlaylistRequestFingerprint("123e4567-e89b-12d3-a456-426614174000", input)
	input.Tracks[0], input.Tracks[1] = input.Tracks[1], input.Tracks[0]
	reordered := PlaylistRequestFingerprint("123e4567-e89b-12d3-a456-426614174000", input)
	otherID := PlaylistRequestFingerprint("123e4567-e89b-12d3-a456-426614174001", input)
	if bytes.Equal(first, reordered) || bytes.Equal(reordered, otherID) {
		t.Fatalf("playlist fingerprints did not distinguish mutation: first=%x reorder=%x other=%x", first, reordered, otherID)
	}
	deleteInput, err := NormalizePlaylistDeleteInput(PlaylistDeleteInput{
		IdempotencyKey: "playlist-delete", ExpectedRevision: playlistRevision(1),
	})
	if err != nil {
		t.Fatalf("NormalizePlaylistDeleteInput() error = %v", err)
	}
	if bytes.Equal(PlaylistDeleteRequestFingerprint("123e4567-e89b-12d3-a456-426614174000", deleteInput), first) {
		t.Fatal("delete and replace fingerprint collided")
	}
	input.ExpectedRevision = playlistRevision(1)
	if bytes.Equal(first, PlaylistRequestFingerprint("123e4567-e89b-12d3-a456-426614174000", input)) {
		t.Fatal("expected revision did not change fingerprint")
	}
}

func TestPlaylistFingerprintsDoNotPanicBeforeValidation(t *testing.T) {
	playlistID := "123e4567-e89b-12d3-a456-426614174000"
	replace := PlaylistRequestFingerprint(playlistID, PlaylistInput{Name: "Unvalidated"})
	delete := PlaylistDeleteRequestFingerprint(playlistID, PlaylistDeleteInput{})
	if bytes.Equal(replace, delete) {
		t.Fatal("unvalidated replace and delete fingerprints collided")
	}
}

func TestNormalizePlaylistIDAndLockKey(t *testing.T) {
	value, err := NormalizePlaylistID(" 123E4567-E89B-12D3-A456-426614174000 ")
	if err != nil || value != "123e4567-e89b-12d3-a456-426614174000" {
		t.Fatalf("NormalizePlaylistID() = %q, %v", value, err)
	}
	if _, err := NormalizePlaylistID("not-a-playlist"); err == nil {
		t.Fatal("NormalizePlaylistID() accepted invalid input")
	}
	if first, second := playlistLockKey("playlist", "a:b", "c"), playlistLockKey("playlist", "a", "b:c"); first == second {
		t.Fatalf("playlist lock keys collided: %q", first)
	}
}
