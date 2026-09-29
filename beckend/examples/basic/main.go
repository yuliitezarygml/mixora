package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/iulian/soundcloud-go/pkg/soundcloud"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1. Initialize client (auto-scrapes client_id from SoundCloud website)
	client, err := soundcloud.New(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}
	fmt.Printf("Connected using ClientID: %s\n\n", client.ClientID())

	// 2. Search for tracks
	query := "Hans Zimmer Interstellar"
	fmt.Printf("Searching for: %q...\n", query)
	results, err := client.SearchTracks(ctx, query, soundcloud.SearchOptions{Limit: 3})
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	if len(results.Collection) == 0 {
		fmt.Println("No tracks found.")
		return
	}

	firstTrack := &results.Collection[0]
	fmt.Printf("Found: %s - %s (ID: %d)\n", firstTrack.User.Username, firstTrack.Title, firstTrack.ID)

	// 3. Obtain direct playable stream URL
	streamURL, format, err := client.GetBestStreamURL(ctx, firstTrack)
	if err != nil {
		log.Fatalf("Failed to resolve stream URL: %v", err)
	}

	fmt.Printf("Stream Format: %s\n", format)
	fmt.Printf("Direct playable audio URL: %s\n", streamURL)
}
