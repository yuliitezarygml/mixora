package api

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// LoggingMiddleware logs request duration and status.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}

// CORSMiddleware handles browser pre-flight requests.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NewRouter registers all routes and returns the configured http.Handler.
func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", h.HealthHandler)
	mux.HandleFunc("/api/v1/resolve", h.ResolveHandler)
	mux.HandleFunc("/api/v1/search", h.SearchHandler)
	mux.HandleFunc("/api/v1/charts/trending", h.GetTrendingHandler)

	// Tracks routes
	mux.HandleFunc("/api/v1/tracks/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/stream"):
			h.GetStreamHandler(w, r)
		case strings.HasSuffix(path, "/lyrics"):
			h.GetLyricsHandler(w, r)
		case strings.HasSuffix(path, "/waveform"):
			h.GetWaveformHandler(w, r)
		case strings.HasSuffix(path, "/download"):
			h.GetOriginalDownloadHandler(w, r)
		case strings.HasSuffix(path, "/related"):
			h.GetRelatedHandler(w, r)
		default:
			h.GetTrackHandler(w, r)
		}
	})

	// Users routes
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/tracks") {
			h.GetUserTracksHandler(w, r)
		} else {
			h.GetUserHandler(w, r)
		}
	})

	// Playlists routes
	mux.HandleFunc("/api/v1/playlists/", h.GetPlaylistHandler)

	// Spotify routes
	mux.HandleFunc("/api/v1/spotify/resolve", h.SpotifyResolveHandler)
	mux.HandleFunc("/api/v1/spotify/search", h.SpotifySearchHandler)
	mux.HandleFunc("/api/v1/spotify/connect/status", h.SpotifyConnectStatusHandler)
	mux.HandleFunc("/api/v1/spotify/connect/info", h.SpotifyConnectInfoHandler)
	mux.HandleFunc("/api/v1/spotify/connect/player/", h.SpotifyConnectActionHandler)
	mux.HandleFunc("/api/v1/spotify/albums/", h.SpotifyAlbumHandler)
	mux.HandleFunc("/api/v1/spotify/artists/", h.SpotifyArtistHandler)
	mux.HandleFunc("/api/v1/spotify/playlists/", h.SpotifyPlaylistHandler)
	mux.HandleFunc("/api/v1/spotify/tracks/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/stream"):
			h.SpotifyStreamHandler(w, r)
		case strings.HasSuffix(path, "/lyrics"):
			h.SpotifyLyricsHandler(w, r)
		default:
			h.SpotifyTrackHandler(w, r)
		}
	})

	// Universal Extractor (YouTube, VK, Bandcamp, etc. via yt-dlp)
	mux.HandleFunc("/api/v1/extract", h.UniversalExtractHandler)
	mux.HandleFunc("/api/v1/media/stream", h.ExternalStreamHandler)
	mux.HandleFunc("/api/v1/youtube/search", h.YouTubeSearchHandler)
	mux.HandleFunc("/api/v1/youtube/stream", h.YouTubeStreamHandler)
	mux.HandleFunc("/api/v1/bandcamp/resolve", h.BandcampResolveHandler)
	mux.HandleFunc("/api/v1/vk/resolve", h.VKResolveHandler)

	var handler http.Handler = mux
	handler = LoggingMiddleware(handler)
	handler = CORSMiddleware(handler)

	return handler
}
