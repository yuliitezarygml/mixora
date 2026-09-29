package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Initialize client
	ytClient := ytdlp.New()
	if !ytClient.IsInstalled() {
		log.Fatal("yt-dlp is not installed on this machine")
	}

	version, _ := ytClient.Version(ctx)
	fmt.Printf("Using yt-dlp version %s (%s)\n\n", version, ytClient.BinaryPath())

	// 2. Extract Bandcamp track
	bcURL := "https://disasterpeace.bandcamp.com/track/compass"
	fmt.Printf("Extracting Bandcamp: %s\n", bcURL)
	item, err := ytClient.Extract(ctx, bcURL)
	if err != nil {
		log.Fatalf("Bandcamp extract failed: %v", err)
	}
	fmt.Printf("  Track:    %s\n", item.Title)
	fmt.Printf("  Artist:   %s\n", item.Artist)
	fmt.Printf("  Album:    %s\n", item.Album)
	fmt.Printf("  Duration: %.1f sec\n", item.Duration)
	fmt.Printf("  Stream:   %s\n\n", item.AudioURL)

	// 3. Search YouTube audio
	query := "Hans Zimmer Time"
	fmt.Printf("Searching YouTube for %q...\n", query)
	res, err := ytClient.SearchYouTube(ctx, query, 3)
	if err != nil {
		log.Fatalf("YouTube search failed: %v", err)
	}
	for i, t := range res.Items {
		fmt.Printf("  %d. %s (Channel: %s, %.0f sec)\n", i+1, t.Title, t.Uploader, t.Duration)
	}
}
