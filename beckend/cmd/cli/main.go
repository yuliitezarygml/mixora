package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/pkg/soundcloud"
	"github.com/iulian/soundcloud-go/pkg/spotify"
	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

func main() {
	clientID := flag.String("client-id", "", "Custom client_id (optional)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Special top-level routers
	if command == "spotify" {
		handleSpotifyCLI(ctx, args[1:])
		return
	}
	if command == "extract" || command == "yt" || command == "bandcamp" || command == "bc" || command == "vk" {
		handleYtDlpCLI(ctx, command, args[1:])
		return
	}

	var opts []soundcloud.Option
	if *clientID != "" {
		opts = append(opts, soundcloud.WithClientID(*clientID))
	}

	client, err := soundcloud.New(ctx, opts...)
	if err != nil {
		log.Fatalf("Error creating SoundCloud client: %v", err)
	}

	switch command {
	case "resolve":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli resolve <url>")
		}
		raw, err := client.Resolve(ctx, args[1])
		if err != nil {
			log.Fatalf("Resolve failed: %v", err)
		}
		prettyPrint(raw)

	case "stream":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli stream <track_url_or_id>")
		}
		var trackID int64
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err == nil {
			trackID = id
		} else {
			track, err := client.ResolveTrack(ctx, args[1])
			if err != nil {
				log.Fatalf("Resolve track URL failed: %v", err)
			}
			trackID = track.ID
		}

		track, err := client.GetTrack(ctx, trackID)
		if err != nil {
			log.Fatalf("GetTrack failed: %v", err)
		}

		streamURL, format, err := client.GetBestStreamURL(ctx, track)
		if err != nil {
			log.Fatalf("GetStreamURL failed: %v", err)
		}

		fmt.Printf("🎵 Track: %s\n", track.Title)
		fmt.Printf("👤 Artist: %s\n", track.User.Username)
		fmt.Printf("⏱️  Duration: %d ms\n", track.Duration)
		fmt.Printf("📻 Best Stream Format: %s\n", format)
		fmt.Printf("🔗 Direct Audio URL:\n%s\n", streamURL)

		formats := client.GetAllStreamFormats(ctx, track)
		if len(formats) > 0 {
			fmt.Println("\n🎛️  All Available Formats & Qualities:")
			for i, f := range formats {
				fmt.Printf("  %d. [%s] Codec: %s | Bitrate: %s | Protocol: %s\n", i+1, strings.ToUpper(f.Quality), strings.ToUpper(f.Codec), f.Bitrate, f.Protocol)
				if f.URL != "" {
					fmt.Printf("     Stream URL: %s\n", f.URL)
				}
			}
		}

	case "search":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli search <query>")
		}
		results, err := client.SearchTracks(ctx, args[1], soundcloud.SearchOptions{Limit: 5})
		if err != nil {
			log.Fatalf("Search failed: %v", err)
		}
		fmt.Printf("Found %d tracks:\n\n", len(results.Collection))
		for i, t := range results.Collection {
			fmt.Printf("%d. [%d] %s - %s (%d ms)\n   URL: %s\n", i+1, t.ID, t.User.Username, t.Title, t.Duration, t.PermalinkURL)
		}

	case "track":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli track <id>")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			log.Fatalf("Invalid track ID: %v", err)
		}
		track, err := client.GetTrack(ctx, id)
		if err != nil {
			log.Fatalf("GetTrack failed: %v", err)
		}
		prettyPrint(track)

	case "lyrics":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli lyrics <track_url_or_id>")
		}
		var trackID int64
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err == nil {
			trackID = id
		} else {
			track, err := client.ResolveTrack(ctx, args[1])
			if err != nil {
				log.Fatalf("Resolve track URL failed: %v", err)
			}
			trackID = track.ID
		}

		track, err := client.GetTrack(ctx, trackID)
		if err != nil {
			log.Fatalf("GetTrack failed: %v", err)
		}

		lyrics, err := client.GetTrackLyrics(ctx, track)
		if err != nil {
			log.Fatalf("GetTrackLyrics failed: %v", err)
		}

		fmt.Printf("🎤 Lyrics for %q (Artist: %s | Source: %s):\n\n", lyrics.TrackTitle, lyrics.ArtistName, lyrics.Source)
		if lyrics.SyncedLyrics != "" {
			fmt.Println(lyrics.SyncedLyrics)
		} else if lyrics.PlainLyrics != "" {
			fmt.Println(lyrics.PlainLyrics)
		} else {
			fmt.Println("No lyrics found.")
		}

	case "user":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli user <id>")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			log.Fatalf("Invalid user ID: %v", err)
		}
		user, err := client.GetUser(ctx, id)
		if err != nil {
			log.Fatalf("GetUser failed: %v", err)
		}
		prettyPrint(user)

	case "trending":
		genre := "all-music"
		if len(args) >= 2 {
			genre = args[1]
		}
		res, err := client.GetTrending(ctx, genre, 10)
		if err != nil {
			log.Fatalf("GetTrending failed: %v", err)
		}
		fmt.Printf("🔥 Trending Tracks (Genre: %s):\n\n", res.Genre)
		for i, item := range res.Collection {
			t := item.Track
			fmt.Printf("%2d. [%d] %s - %s (%d ms)\n    Stream: %s\n", i+1, t.ID, t.User.Username, t.Title, t.Duration, t.PermalinkURL)
		}

	case "waveform":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli waveform <track_id_or_url>")
		}
		var trackID int64
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err == nil {
			trackID = id
		} else {
			track, err := client.ResolveTrack(ctx, args[1])
			if err != nil {
				log.Fatalf("Resolve track URL failed: %v", err)
			}
			trackID = track.ID
		}
		track, err := client.GetTrack(ctx, trackID)
		if err != nil {
			log.Fatalf("GetTrack failed: %v", err)
		}
		wf, err := client.GetTrackWaveform(ctx, track)
		if err != nil {
			log.Fatalf("GetTrackWaveform failed: %v", err)
		}
		fmt.Printf("🌊 Waveform for %q: %d samples loaded (width: %d, height: %d)\n", track.Title, len(wf.Samples), wf.Width, wf.Height)
		if len(wf.Samples) > 20 {
			fmt.Printf("First 20 amplitude samples: %v...\n", wf.Samples[:20])
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func handleSpotifyCLI(ctx context.Context, args []string) {
	if len(args) < 1 {
		printSpotifyUsage()
		os.Exit(1)
	}

	spClient, err := spotify.New(ctx)
	if err != nil {
		log.Fatalf("Error creating Spotify client: %v", err)
	}

	subcmd := args[0]
	switch subcmd {
	case "resolve":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify resolve <spotify_url_or_uri>")
		}
		res, err := spClient.Resolve(ctx, args[1])
		if err != nil {
			log.Fatalf("Spotify Resolve failed: %v", err)
		}
		prettyPrint(res)

	case "track":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify track <track_id_or_url>")
		}
		track, err := spClient.GetTrack(ctx, args[1])
		if err != nil {
			log.Fatalf("GetTrack failed: %v", err)
		}
		fmt.Printf("🎵 Spotify Track: %s\n", track.Title)
		if len(track.Artists) > 0 {
			fmt.Printf("👤 Artist: %s\n", track.Artists[0].Name)
		}
		fmt.Printf("⏱️  Duration: %d ms\n", track.DurationMs)
		fmt.Printf("🔗 URI: %s\n", track.URI)
		if track.PreviewURL != "" {
			fmt.Printf("📻 MP3 Preview Stream: %s\n", track.PreviewURL)
		}
		fmt.Printf("🌐 Open in Spotify: %s\n", track.ExternalURL)

	case "stream":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify stream <track_id_or_url>")
		}
		info, err := spClient.GetStream(ctx, args[1])
		if err != nil {
			log.Fatalf("GetStream failed: %v", err)
		}
		fmt.Printf("🎵 %s (%s)\n", info.Title, info.Artists)
		fmt.Printf("🔗 URI: %s\n", info.URI)
		if info.PreviewURL != "" {
			fmt.Printf("📻 Direct MP3 Preview Stream:\n%s\n", info.PreviewURL)
		}
		if len(info.AvailableQualities) > 0 {
			fmt.Println("\n🎛️  Spotify Audio Formats & Quality Tiers:")
			for i, q := range info.AvailableQualities {
				status := "✓ Direct Stream"
				if !q.Available {
					status = "Requires Spotify Connect"
				}
				fmt.Printf("  %d. %s [%s] - %s (%s)\n", i+1, q.Name, strings.ToUpper(q.Codec), q.Bitrate, status)
				if q.URL != "" {
					fmt.Printf("     Stream URL: %s\n", q.URL)
				}
			}
		}
		fmt.Println()
		if info.ConnectAvailable {
			fmt.Printf("🔊 Spotify Connect Receiver: ACTIVE (%s)\n", info.ConnectDevice)
			fmt.Println("   To play full audio on this speaker, run:")
			fmt.Printf("   music-cli spotify connect play %s\n", info.URI)
		} else {
			fmt.Println("ℹ️  Spotify Connect daemon is not running.")
			fmt.Println("   For full-length streaming, start go-librespot (`brew install go-librespot`)")
		}

	case "lyrics":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify lyrics <track_id_or_url>")
		}
		lyrics, err := spClient.GetTrackLyrics(ctx, args[1])
		if err != nil {
			log.Fatalf("GetTrackLyrics failed: %v", err)
		}
		fmt.Printf("🎤 Lyrics for %q (Artist: %s | Source: %s):\n\n", lyrics.TrackName, lyrics.Artist, lyrics.Source)
		if lyrics.Synced && len(lyrics.Lines) > 0 {
			for _, line := range lyrics.Lines {
				mins := line.TimeMs / 60000
				secs := (line.TimeMs % 60000) / 1000
				ms := (line.TimeMs % 1000) / 10
				fmt.Printf("[%02d:%02d.%02d] %s\n", mins, secs, ms, line.Text)
			}
		} else if lyrics.Plain != "" {
			fmt.Println(lyrics.Plain)
		} else {
			fmt.Println("No lyrics found.")
		}

	case "album":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify album <album_id_or_url>")
		}
		album, err := spClient.GetAlbum(ctx, args[1])
		if err != nil {
			log.Fatalf("GetAlbum failed: %v", err)
		}
		fmt.Printf("💿 Album: %s\n", album.Name)
		if len(album.Artists) > 0 {
			fmt.Printf("👤 Artist: %s\n", album.Artists[0].Name)
		}
		fmt.Printf("Tracks (%d):\n", len(album.Tracks))
		for _, t := range album.Tracks {
			fmt.Printf("  %2d. %s (%d ms) [%s]\n", t.TrackNumber, t.Title, t.DurationMs, t.URI)
		}

	case "artist":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify artist <artist_id_or_url>")
		}
		artist, err := spClient.GetArtist(ctx, args[1])
		if err != nil {
			log.Fatalf("GetArtist failed: %v", err)
		}
		fmt.Printf("👤 Artist: %s\n", artist.Name)
		fmt.Printf("Top Tracks (%d):\n", len(artist.TopTracks))
		for i, t := range artist.TopTracks {
			fmt.Printf("  %2d. %s (%d ms) [%s]\n", i+1, t.Title, t.DurationMs, t.URI)
		}

	case "playlist":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify playlist <playlist_id_or_url>")
		}
		playlist, err := spClient.GetPlaylist(ctx, args[1])
		if err != nil {
			log.Fatalf("GetPlaylist failed: %v", err)
		}
		fmt.Printf("📜 Playlist: %s (by %s)\n", playlist.Name, playlist.Owner)
		fmt.Printf("Tracks (%d):\n", len(playlist.Tracks))
		for i, t := range playlist.Tracks {
			fmt.Printf("  %2d. %s (%d ms)\n", i+1, t.Title, t.DurationMs)
		}

	case "search":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify search <query>")
		}
		res, err := spClient.Search(ctx, args[1], spotify.TypeTrack, 5)
		if err != nil {
			log.Fatalf("Search failed: %v", err)
		}
		fmt.Printf("Found %d tracks:\n\n", len(res.Tracks))
		for i, t := range res.Tracks {
			artist := "Unknown"
			if len(t.Artists) > 0 {
				artist = t.Artists[0].Name
			}
			fmt.Printf("%d. %s - %s (%d ms)\n   URI: %s\n   URL: %s\n", i+1, artist, t.Title, t.DurationMs, t.URI, t.ExternalURL)
		}

	case "connect":
		handleSpotifyConnectCLI(ctx, spClient, args[1:])

	default:
		printSpotifyUsage()
		os.Exit(1)
	}
}

func handleSpotifyConnectCLI(ctx context.Context, spClient *spotify.Client, args []string) {
	if len(args) < 1 {
		fmt.Println("Spotify Connect Commands:")
		fmt.Println("  status                     Get current device playback status")
		fmt.Println("  play [uri]                 Resume or play track URI (spotify:track:...)")
		fmt.Println("  pause                      Pause playback")
		fmt.Println("  next                       Skip to next track")
		fmt.Println("  prev                       Skip to previous track")
		fmt.Println("  volume <0-100>             Set volume percentage")
		fmt.Println("  info                       Show daemon detection & binary info")
		os.Exit(1)
	}

	conn := spClient.Connect()
	action := args[0]

	switch action {
	case "info":
		sup := spClient.Supervisor()
		path, bType := sup.BinaryInfo()
		fmt.Println("🎧 Spotify Connect Daemon Info:")
		fmt.Printf("  Binary Installed: %v\n", sup.IsInstalled())
		if sup.IsInstalled() {
			fmt.Printf("  Binary Type:      %s\n", bType)
			fmt.Printf("  Binary Path:      %s\n", path)
		} else {
			fmt.Printf("\nInstallation Instructions:\n%s\n", sup.InstallInstructions())
		}
		fmt.Printf("  Daemon Reachable: %v (at %s)\n", conn.IsAvailable(ctx), conn.BaseURL())

	case "status":
		status, err := conn.Status(ctx)
		if err != nil {
			log.Fatalf("Connect Status failed: %v", err)
		}
		fmt.Printf("🔊 Device: %s (Type: %s, ID: %s)\n", status.DeviceName, status.DeviceType, status.DeviceID)
		fmt.Printf("👤 User:   %s\n", status.Username)
		fmt.Printf("📊 State:  Stopped=%v, Paused=%v, Buffering=%v\n", status.Stopped, status.Paused, status.Buffering)
		fmt.Printf("🔊 Volume: %d (steps: %d)\n", status.Volume, status.VolumeSteps)
		if status.Track != nil {
			fmt.Printf("🎵 Now Playing: %s\n", status.Track.Name)
			fmt.Printf("👤 Artist:      %v\n", status.Track.ArtistNames)
			fmt.Printf("💿 Album:       %s\n", status.Track.AlbumName)
			fmt.Printf("⏱️  Progress:    %d / %d ms\n", status.Track.Position, status.Track.Duration)
		}

	case "play":
		if len(args) >= 2 {
			uri := args[1]
			if err := conn.Load(ctx, uri, true); err != nil {
				log.Fatalf("Load & play failed: %v", err)
			}
			fmt.Printf("▶️ Playing %s on Spotify Connect speaker\n", uri)
		} else {
			if err := conn.Play(ctx); err != nil {
				log.Fatalf("Play failed: %v", err)
			}
			fmt.Println("▶️ Resumed playback")
		}

	case "pause":
		if err := conn.Pause(ctx); err != nil {
			log.Fatalf("Pause failed: %v", err)
		}
		fmt.Println("⏸️ Paused playback")

	case "next":
		if err := conn.Next(ctx); err != nil {
			log.Fatalf("Next track failed: %v", err)
		}
		fmt.Println("⏭️ Skipped to next track")

	case "prev":
		if err := conn.Prev(ctx); err != nil {
			log.Fatalf("Previous track failed: %v", err)
		}
		fmt.Println("⏮️ Skipped to previous track")

	case "volume":
		if len(args) < 2 {
			log.Fatal("Usage: music-cli spotify connect volume <0-100>")
		}
		vol, err := strconv.Atoi(args[1])
		if err != nil {
			log.Fatalf("Invalid volume percentage: %v", err)
		}
		if err := conn.SetVolume(ctx, vol); err != nil {
			log.Fatalf("Set volume failed: %v", err)
		}
		fmt.Printf("🔊 Volume set to %d%%\n", vol)

	default:
		log.Fatalf("Unknown connect action: %s", action)
	}
}

func handleYtDlpCLI(ctx context.Context, cmd string, args []string) {
	ytClient := ytdlp.New()
	if !ytClient.IsInstalled() {
		log.Fatal("yt-dlp is not installed. Install via 'brew install yt-dlp' (macOS) or 'apt install yt-dlp' (Linux)")
	}

	switch cmd {
	case "extract", "bandcamp", "bc", "vk":
		if len(args) < 1 {
			log.Fatalf("Usage: music-cli %s <url>", cmd)
		}
		targetURL := args[0]
		item, err := ytClient.Extract(ctx, targetURL)
		if err != nil {
			log.Fatalf("Extraction failed: %v", err)
		}

		fmt.Printf("📦 Source:   %s\n", strings.ToUpper(item.Extractor))
		fmt.Printf("🎵 Title:    %s\n", item.Title)
		if item.Artist != "" {
			fmt.Printf("👤 Artist:   %s\n", item.Artist)
		}
		if item.Album != "" {
			fmt.Printf("💿 Album:    %s\n", item.Album)
		}
		fmt.Printf("⏱️  Duration: %.1f sec\n", item.Duration)
		if item.Thumbnail != "" {
			fmt.Printf("🖼️  Cover:    %s\n", item.Thumbnail)
		}
		if item.AudioURL != "" {
			fmt.Printf("📻 Best Stream (%s @ %.0f kbps):\n%s\n", strings.ToUpper(item.AudioFormat), item.Bitrate, item.AudioURL)
		}
		if len(item.AvailableQualities) > 0 {
			fmt.Println("\n🎛️  All Available Audio Qualities & Formats:")
			for i, q := range item.AvailableQualities {
				fmt.Printf("  %d. %s | Codec: %s | Ext: .%s | Sample Rate: %d Hz\n",
					i+1, q.Name, strings.ToUpper(q.Codec), q.Extension, q.SampleRate)
				if q.URL != "" {
					fmt.Printf("     Stream URL: %s\n", q.URL)
				}
			}
		}
		if item.Lyrics != nil {
			fmt.Printf("\n🎤 Lyrics for %q (Artist: %s | Source: %s | Synced: %v):\n\n",
				item.Lyrics.TrackName, item.Lyrics.Artist, item.Lyrics.Source, item.Lyrics.Synced)
			if item.Lyrics.Synced && len(item.Lyrics.Lines) > 0 {
				for _, line := range item.Lyrics.Lines {
					mins := line.TimeMs / 60000
					secs := (line.TimeMs % 60000) / 1000
					ms := (line.TimeMs % 1000) / 10
					fmt.Printf("[%02d:%02d.%02d] %s\n", mins, secs, ms, line.Text)
				}
			} else if item.Lyrics.Plain != "" {
				fmt.Println(item.Lyrics.Plain)
			}
		}

	case "yt":
		if len(args) < 1 {
			fmt.Println("YouTube Commands:")
			fmt.Println("  search <query>             Search YouTube audio tracks")
			fmt.Println("  stream <url_or_id>         Extract direct Opus/AAC audio stream URL")
			os.Exit(1)
		}
		ytSub := args[0]
		switch ytSub {
		case "search":
			if len(args) < 2 {
				log.Fatal("Usage: music-cli yt search <query>")
			}
			query := strings.Join(args[1:], " ")
			res, err := ytClient.SearchYouTube(ctx, query, 5)
			if err != nil {
				log.Fatalf("YouTube search failed: %v", err)
			}
			fmt.Printf("Found %d YouTube tracks for %q:\n\n", len(res.Items), query)
			for i, item := range res.Items {
				fmt.Printf("%d. %s (%.0f sec)\n   Channel: %s\n   URL:     %s\n", i+1, item.Title, item.Duration, item.Uploader, item.WebpageURL)
			}

		case "stream":
			if len(args) < 2 {
				log.Fatal("Usage: music-cli yt stream <url_or_id>")
			}
			target := args[1]
			if !strings.HasPrefix(target, "http") {
				target = "https://www.youtube.com/watch?v=" + target
			}
			audioURL, err := ytClient.ExtractAudioURL(ctx, target)
			if err != nil {
				log.Fatalf("Failed to extract audio stream: %v", err)
			}
			fmt.Printf("📻 Direct YouTube Audio Stream URL:\n%s\n", audioURL)

		default:
			log.Fatalf("Unknown YouTube command: %s", ytSub)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Music CLI Tool (SoundCloud + Spotify Librespot + YouTube/Bandcamp/VK)")
	fmt.Println("Usage: music-cli [flags] <command> [arguments]")
	fmt.Println("\n-- SoundCloud Commands --")
	fmt.Println("  resolve <url>              Resolve any SoundCloud URL")
	fmt.Println("  stream <url_or_id>         Resolve direct audio playback stream URL")
	fmt.Println("  lyrics <url_or_id>         Get synced / plain lyrics for track")
	fmt.Println("  waveform <url_or_id>       Get audio waveform samples")
	fmt.Println("  user <id>                  Get user profile info")
	fmt.Println("  trending [genre]           Get trending / hot charts (e.g. all-music, hiphoprap)")
	fmt.Println("  search <query>             Search SoundCloud tracks")
	fmt.Println("  track <id>                 Get SoundCloud track details by ID")
	fmt.Println("\n-- Spotify & Librespot Commands --")
	fmt.Println("  spotify resolve <url>      Resolve any Spotify URL (track, album, playlist, artist)")
	fmt.Println("  spotify track <id_or_url>  Get Spotify track metadata & direct MP3 preview")
	fmt.Println("  spotify stream <id_or_url> Get stream info & Connect status")
	fmt.Println("  spotify lyrics <id_or_url> Get synchronized karaoke timecoded lyrics")
	fmt.Println("  spotify album <id_or_url>  Get album tracklist & metadata")
	fmt.Println("  spotify artist <id_or_url> Get artist top tracks")
	fmt.Println("  spotify playlist <url>     Get playlist tracks")
	fmt.Println("  spotify search <query>     Search Spotify catalog")
	fmt.Println("  spotify connect <action>   Control Spotify Connect player (status, play, pause, next, volume, info)")
	fmt.Println("\n-- Universal Extractor (YouTube, VK, Bandcamp via yt-dlp) --")
	fmt.Println("  extract <url>              Extract metadata & direct audio stream from ANY supported site")
	fmt.Println("  yt search <query>          Search YouTube audio tracks")
	fmt.Println("  yt stream <url_or_id>      Extract direct YouTube Opus/AAC stream URL")
	fmt.Println("  bc <url>                   Extract Bandcamp track / album audio")
	fmt.Println("  vk <url>                   Extract VK audio stream")
}

func printSpotifyUsage() {
	fmt.Println("Spotify & Librespot CLI Commands:")
	fmt.Println("  spotify resolve <url>      Resolve any Spotify URL")
	fmt.Println("  spotify track <id_or_url>  Get track metadata & direct MP3 preview")
	fmt.Println("  spotify stream <id_or_url> Get stream info & Connect playback instructions")
	fmt.Println("  spotify lyrics <id_or_url> Get synchronized lyrics")
	fmt.Println("  spotify album <id_or_url>  Get album details & tracks")
	fmt.Println("  spotify artist <id_or_url> Get artist top tracks")
	fmt.Println("  spotify playlist <url>     Get playlist tracks")
	fmt.Println("  spotify search <query>     Search Spotify catalog")
	fmt.Println("  spotify connect <action>   Control Spotify Connect player (status, play, pause, next, volume, info)")
}

func prettyPrint(v any) {
	var b []byte
	var err error
	if raw, ok := v.([]byte); ok {
		var obj any
		_ = json.Unmarshal(raw, &obj)
		b, err = json.MarshalIndent(obj, "", "  ")
	} else {
		b, err = json.MarshalIndent(v, "", "  ")
	}
	if err == nil {
		fmt.Println(string(b))
	}
}
