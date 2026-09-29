package ytdlp_test

import (
	"context"
	"testing"
	"time"

	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

func TestClientDiscovery(t *testing.T) {
	client := ytdlp.New()

	if !client.IsInstalled() {
		t.Skip("yt-dlp not installed on host, skipping discovery test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	version, err := client.Version(ctx)
	if err != nil {
		t.Fatalf("failed to get yt-dlp version: %v", err)
	}

	if version == "" {
		t.Fatal("expected non-empty version string")
	}
	t.Logf("Detected yt-dlp version: %s at %s", version, client.BinaryPath())
}
