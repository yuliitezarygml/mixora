package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/iulian/soundcloud-go/pkg/spotify"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// 1. Initialize Spotify Client (Zero configuration, no API keys needed)
	spClient, err := spotify.New(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize Spotify client: %v", err)
	}

	// 2. Resolve a Spotify Track
	trackURI := "spotify:track:6rqhFgbbKwnb9MLmUQDhG6" // Pink Floyd - Speak To Me
	fmt.Printf("Resolving track: %s\n", trackURI)
	track, err := spClient.GetTrack(ctx, trackURI)
	if err != nil {
		log.Fatalf("Failed to fetch track: %v", err)
	}

	fmt.Printf("Track Title: %s\n", track.Title)
	if len(track.Artists) > 0 {
		fmt.Printf("Artist:      %s\n", track.Artists[0].Name)
	}
	fmt.Printf("Duration:    %d ms\n", track.DurationMs)
	fmt.Printf("Preview MP3: %s\n\n", track.PreviewURL)

	// 3. Search Spotify Catalog
	query := "Daft Punk"
	fmt.Printf("Searching for %q...\n", query)
	res, err := spClient.Search(ctx, query, spotify.TypeTrack, 3)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}
	for i, t := range res.Tracks {
		artist := "Unknown"
		if len(t.Artists) > 0 {
			artist = t.Artists[0].Name
		}
		fmt.Printf("  %d. %s - %s (%s)\n", i+1, artist, t.Title, t.URI)
	}
	fmt.Println()

	// 4. Spotify Connect daemon status & controls
	conn := spClient.Connect()
	if conn.IsAvailable(ctx) {
		status, err := conn.Status(ctx)
		if err == nil {
			fmt.Printf("Spotify Connect speaker detected: %s (Type: %s)\n", status.DeviceName, status.DeviceType)
			fmt.Println("To play full Spotify tracks, call: conn.Load(ctx, track.URI, true)")
		}
	} else {
		fmt.Println("Spotify Connect daemon not running locally.")
		fmt.Println("Install go-librespot (`brew install go-librespot`) to enable Connect hardware playback.")
	}
}
