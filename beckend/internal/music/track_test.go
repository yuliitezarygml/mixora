package music

import "testing"

func TestCanonicalTrackNormalizesLegacySoundCloudURN(t *testing.T) {
	track := CanonicalTrack(Track{
		Source: " SoundCloud ", ID: "soundcloud:tracks:1534086151",
		ArtistID: "soundcloud:users:1030983220",
	})
	if track.Source != "soundcloud" || track.ID != "1534086151" || track.ArtistID != "1030983220" {
		t.Fatalf("unexpected canonical track: %#v", track)
	}
}
