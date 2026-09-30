package embedding

import (
	"strings"
	"testing"

	"github.com/iulian/soundcloud-go/internal/music"
)

func TestTrackDocumentUsesProviderNeutralMetadata(t *testing.T) {
	document, hash := TrackDocument(music.Track{
		ID: "secret-provider-id", Source: "soundcloud", Title: "  Моя   песня ",
		Artist: "Артист", Album: "Альбом", Genre: "pop", TagList: "dream pop",
		Permalink: "https://example.test/private-stream",
	})
	if strings.Contains(document, "secret-provider-id") || strings.Contains(document, "example.test") {
		t.Fatalf("provider internals leaked into document: %q", document)
	}
	if document != "title: Моя песня | text: artist: Артист | album: Альбом | genre: pop | tags: dream pop" {
		t.Fatalf("unexpected document: %q", document)
	}
	if len(hash) != 64 {
		t.Fatalf("unexpected hash: %q", hash)
	}
}

func TestVectorLiteral(t *testing.T) {
	if got := vectorLiteral([]float32{1, -0.25, 0}); got != "[1,-0.25,0]" {
		t.Fatalf("unexpected vector literal %q", got)
	}
}
