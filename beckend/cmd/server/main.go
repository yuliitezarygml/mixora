package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/iulian/soundcloud-go/internal/api"
	"github.com/iulian/soundcloud-go/internal/auth"
	"github.com/iulian/soundcloud-go/internal/config"
	"github.com/iulian/soundcloud-go/internal/database"
	"github.com/iulian/soundcloud-go/internal/events"
	"github.com/iulian/soundcloud-go/internal/httpapi"
	"github.com/iulian/soundcloud-go/internal/library"
	"github.com/iulian/soundcloud-go/internal/mail"
	"github.com/iulian/soundcloud-go/internal/playback"
	"github.com/iulian/soundcloud-go/internal/recommendation"
	"github.com/iulian/soundcloud-go/pkg/soundcloud"
	"github.com/iulian/soundcloud-go/pkg/spotify"
	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

func main() {
	port := flag.String("port", "", "Override the HTTP port from MIXORA_HTTP_ADDR")

	// SoundCloud options
	clientID := flag.String("client-id", os.Getenv("SOUNDCLOUD_CLIENT_ID"), "SoundCloud client_id (optional, auto-scraped if empty)")
	authToken := flag.String("auth-token", os.Getenv("SOUNDCLOUD_AUTH_TOKEN"), "SoundCloud OAuth token (optional)")

	// Spotify options
	spotifyClientID := flag.String("spotify-client-id", os.Getenv("SPOTIFY_CLIENT_ID"), "Spotify Client ID (optional, for official Web API)")
	spotifyClientSecret := flag.String("spotify-client-secret", os.Getenv("SPOTIFY_CLIENT_SECRET"), "Spotify Client Secret (optional)")
	spotifyConnectURL := flag.String("spotify-connect-url", "http://127.0.0.1:24879", "Spotify Connect daemon REST API URL (go-librespot / librespot)")
	spotifyAutoDaemon := flag.Bool("spotify-auto-daemon", false, "Automatically launch local go-librespot/librespot daemon if installed")
	spotifyDeviceName := flag.String("spotify-device-name", "SoundCloud-Spotify-Go", "Spotify Connect device name announced on the network")

	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Invalid application configuration: %v", err)
	}
	if strings.TrimSpace(*port) != "" {
		cfg.HTTPAddr = ":" + strings.TrimPrefix(strings.TrimSpace(*port), ":")
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startupCancel()
	db, err := database.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Database is unavailable: %v", err)
	}
	defer db.Close()
	if err := database.Migrate(startupCtx, db); err != nil {
		log.Fatalf("[FATAL] Database migration failed: %v", err)
	}
	log.Println("[SUCCESS] PostgreSQL connected and migrations applied")

	authStore, err := auth.NewStore(db)
	if err != nil {
		log.Fatalf("[FATAL] Initialize auth store: %v", err)
	}
	authOptions := auth.DefaultOptions()
	authOptions.SessionTTL = cfg.SessionTTL
	authService, err := auth.NewService(authStore, authOptions)
	if err != nil {
		log.Fatalf("[FATAL] Initialize auth service: %v", err)
	}
	mailOutbox, err := mail.NewOutbox(db)
	if err != nil {
		log.Fatalf("[FATAL] Initialize mail outbox: %v", err)
	}
	mailSender, err := mail.NewSMTPSender(mail.SMTPConfig{
		Addr: cfg.SMTPAddr, From: cfg.SMTPFrom, Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword, Timeout: 15 * time.Second,
	})
	if err != nil {
		log.Fatalf("[FATAL] Initialize SMTP: %v", err)
	}
	mailOptions := mail.DefaultWorkerOptions()
	mailOptions.OnError = func(err error) { log.Printf("[WARN] Mail worker: %v", err) }
	mailWorker, err := mail.NewWorker(mailOutbox, mailSender, mailOptions)
	if err != nil {
		log.Fatalf("[FATAL] Initialize mail worker: %v", err)
	}
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go func() {
		if err := mailWorker.Run(workerCtx); err != nil && err != context.Canceled {
			log.Printf("[WARN] Mail worker stopped: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1. Initialize SoundCloud Client
	log.Println("[INFO] Initializing SoundCloud Client...")
	var scOpts []soundcloud.Option
	if *clientID != "" {
		scOpts = append(scOpts, soundcloud.WithClientID(*clientID))
	}
	if *authToken != "" {
		scOpts = append(scOpts, soundcloud.WithAuthToken(*authToken))
	}

	scClient, err := soundcloud.New(ctx, scOpts...)
	if err != nil {
		log.Printf("[WARN] Failed to initialize SoundCloud client: %v", err)
	} else {
		log.Printf("[SUCCESS] SoundCloud Client initialized with client_id: %s", scClient.ClientID())
	}

	// 2. Initialize Spotify Client
	log.Println("[INFO] Initializing Spotify & Connect Client...")
	var spOpts []spotify.Option
	if *spotifyClientID != "" && *spotifyClientSecret != "" {
		spOpts = append(spOpts, spotify.WithCredentials(*spotifyClientID, *spotifyClientSecret))
	}
	if *spotifyConnectURL != "" {
		spOpts = append(spOpts, spotify.WithConnectURL(*spotifyConnectURL))
	}

	spClient, err := spotify.New(ctx, spOpts...)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize Spotify client: %v", err)
	}

	// Check Spotify Connect daemon availability
	sup := spClient.Supervisor()
	if sup.IsInstalled() {
		binPath, binType := sup.BinaryInfo()
		log.Printf("[INFO] Discovered Spotify Connect binary: %s (%s)", binType, binPath)

		if *spotifyAutoDaemon && !spClient.Connect().IsAvailable(ctx) {
			log.Printf("[INFO] Starting background %s daemon with name '%s'...", binType, *spotifyDeviceName)
			err := sup.Start(context.Background(), spotify.DaemonConfig{
				DeviceName: *spotifyDeviceName,
				Port:       24879,
			})
			if err != nil {
				log.Printf("[WARN] Could not auto-start Spotify Connect daemon: %v", err)
			} else {
				log.Printf("[SUCCESS] Spotify Connect daemon started successfully.")
			}
		}
	} else {
		log.Println("[INFO] Note: go-librespot / librespot binary not found in PATH.")
		log.Println("       To enable Spotify Connect hardware streaming, run: brew install go-librespot")
	}

	if spClient.Connect().IsAvailable(ctx) {
		log.Printf("[SUCCESS] Spotify Connect daemon is ACTIVE and responding at %s", *spotifyConnectURL)
	} else {
		log.Printf("[INFO] Spotify Connect daemon is currently offline at %s (metadata & preview streams still active)", *spotifyConnectURL)
	}

	// 3. Initialize yt-dlp Extractor
	log.Println("[INFO] Initializing Universal Extractor (yt-dlp)...")
	ytClient := ytdlp.New()
	if ytClient.IsInstalled() {
		ver, _ := ytClient.Version(ctx)
		log.Printf("[SUCCESS] yt-dlp detected: version %s (%s)", ver, ytClient.BinaryPath())
	} else {
		log.Println("[WARN] yt-dlp is not found on PATH. Universal extraction will be disabled.")
		log.Println("       Install via: brew install yt-dlp (macOS) or apt install yt-dlp (Linux)")
	}

	// 4. Keep the existing music engine intact and mount the Mixora app API in
	// front of it. Unknown routes fall through to the engine router.
	musicHandler := api.NewHandler(scClient, spClient, ytClient)
	musicRouter := api.NewRouter(musicHandler)
	router, err := httpapi.New(httpapi.Config{
		Database:        db,
		Auth:            authService,
		Libraries:       library.New(db),
		Events:          events.New(db),
		MailOutbox:      mailOutbox,
		Playback:        playback.NewHub(playback.DefaultMaxMessageBytes),
		Recommendations: recommendation.New(scClient),
		MusicEngine:     musicRouter,
		PublicURL:       cfg.PublicURL,
		CookieSecure:    cfg.CookieSecure,
		SessionTTL:      cfg.SessionTTL,
	})
	if err != nil {
		log.Fatalf("[FATAL] Initialize Mixora HTTP API: %v", err)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0, // WebSocket connections are long-lived; handlers set their own deadlines.
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] Server listening on %s", cfg.HTTPAddr)
		log.Println("[INFO] Available Endpoints:")
		log.Println("  -- Mixora application --")
		log.Println("  - GET  /health")
		log.Println("  - GET  /ready")
		log.Println("  - POST /api/v1/auth/{register|login|logout}")
		log.Println("  - GET  /api/v1/auth/session")
		log.Println("  - GET  /api/v1/me")
		log.Println("  - GET/PUT /api/v1/library")
		log.Println("  - POST /api/v1/events")
		log.Println("  - POST /api/v1/wave")
		log.Println("  - GET  /api/v1/playback/ws")
		log.Println("  -- Existing music engine --")
		log.Println("  -- SoundCloud --")
		log.Println("  - GET  /api/v1/resolve?url=<soundcloud_url>")
		log.Println("  - GET  /api/v1/search?q=<query>&type=tracks|playlists|users")
		log.Println("  - GET  /api/v1/tracks/{id}")
		log.Println("  - GET  /api/v1/tracks/{id}/stream")
		log.Println("  - GET  /api/v1/tracks/{id}/lyrics")
		log.Println("  - GET  /api/v1/tracks/{id}/waveform")
		log.Println("  - GET  /api/v1/tracks/{id}/download")
		log.Println("  - GET  /api/v1/tracks/{id}/related")
		log.Println("  - GET  /api/v1/charts/trending")
		log.Println("  - GET  /api/v1/users/{id}")
		log.Println("  - GET  /api/v1/users/{id}/tracks")
		log.Println("  - GET  /api/v1/playlists/{id}")
		log.Println("  -- Spotify & Librespot Connect --")
		log.Println("  - GET  /api/v1/spotify/resolve?url=<spotify_url>")
		log.Println("  - GET  /api/v1/spotify/search?q=<query>&type=track|album|artist|playlist")
		log.Println("  - GET  /api/v1/spotify/tracks/{id}")
		log.Println("  - GET  /api/v1/spotify/tracks/{id}/stream")
		log.Println("  - GET  /api/v1/spotify/tracks/{id}/lyrics")
		log.Println("  - GET  /api/v1/spotify/albums/{id}")
		log.Println("  - GET  /api/v1/spotify/artists/{id}")
		log.Println("  - GET  /api/v1/spotify/playlists/{id}")
		log.Println("  - GET  /api/v1/spotify/connect/status")
		log.Println("  - GET  /api/v1/spotify/connect/info")
		log.Println("  - POST /api/v1/spotify/connect/player/{play|pause|play-pause|next|prev|volume|seek|load}")
		log.Println("  -- Universal Extractor (YouTube, VK, Bandcamp via yt-dlp) --")
		log.Println("  - GET  /api/v1/extract?url=<any_supported_url>")
		log.Println("  - GET  /api/v1/youtube/search?q=<query>&limit=5")
		log.Println("  - GET  /api/v1/youtube/stream?url=<yt_url_or_id>")
		log.Println("  - GET  /api/v1/bandcamp/resolve?url=<bandcamp_url>")
		log.Println("  - GET  /api/v1/vk/resolve?url=<vk_url>")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server listen error: %v", err)
		}
	}()

	<-stop
	log.Println("[INFO] Shutting down server gracefully...")
	stopWorkers()

	// Stop Spotify supervisor daemon if running
	if sup.IsRunning() {
		log.Println("[INFO] Stopping supervised Spotify Connect daemon...")
		_ = sup.Stop()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Server shutdown error: %v", err)
	}

	fmt.Println("[INFO] Server stopped.")
}
