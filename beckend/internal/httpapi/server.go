package httpapi

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"mixora/beckend/internal/auth"
	"mixora/beckend/internal/catalog"
	"mixora/beckend/internal/config"
	"mixora/beckend/internal/playlist"
	"mixora/beckend/internal/soundcloud"
	"net/http"
	"os"
	"time"
)

type Server struct {
	soundcloud *soundcloud.Client
	db         *pgxpool.Pool
	auth       *auth.Service
	catalog    *catalog.Repository
	playlists  *playlist.Repository
	cfg        config.Config
	media      *os.Root
	log        *slog.Logger
	limiter    *loginLimiter
	playback   playbackHub
}

func New(db *pgxpool.Pool, cfg config.Config, log *slog.Logger) (*Server, error) {
	media, err := os.OpenRoot(cfg.MediaDir)
	if err != nil {
		return nil, err
	}
	var tokens soundcloud.TokenSource
	if cfg.SoundCloudClientID != "" {
		t, err := soundcloud.NewTokens(db, cfg.SoundCloudClientID, cfg.SoundCloudClientSecret, cfg.TokenEncryptionKey)
		if err != nil {
			media.Close()
			return nil, err
		}
		tokens = t
	}
	return &Server{soundcloud: soundcloud.New(tokens), db: db, auth: auth.New(db), catalog: catalog.New(db), playlists: playlist.New(db), cfg: cfg, media: media, log: log, limiter: &loginLimiter{entries: map[string]attempts{}}}, nil
}
func (s *Server) Close() error { return s.media.Close() }
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/providers/soundcloud/catalog/{kind}", s.withUser(s.soundcloudSearch))
	mux.HandleFunc("GET /api/v1/providers/soundcloud/catalog/{kind}/{id}", s.withUser(s.soundcloudResource))
	mux.HandleFunc("GET /api/v1/providers/soundcloud/catalog/{kind}/{id}/{section}", s.withUser(s.soundcloudResource))
	mux.HandleFunc("GET /api/v1/providers/soundcloud/tracks/{id}/playback", s.withUser(s.soundcloudPlayback))
	mux.HandleFunc("GET /api/v1/providers/soundcloud/tracks", s.withUser(s.soundcloudSearch))
	mux.HandleFunc("GET /api/v1/providers/soundcloud/tracks/{id}/streams", s.withUser(s.soundcloudStreams))
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.db.Ping(ctx); err != nil {
			fail(w, 503, "database unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /api/v1/auth/register", s.rateLimit(s.register))
	mux.HandleFunc("POST /api/v1/auth/login", s.rateLimit(s.login))
	mux.HandleFunc("POST /api/v1/auth/resume", s.rateLimit(s.resume))
	mux.HandleFunc("GET /api/v1/auth/session", s.withUser(s.session))
	mux.HandleFunc("POST /api/v1/auth/logout", s.withUser(s.logout))
	mux.HandleFunc("GET /api/v1/me", s.withUser(func(w http.ResponseWriter, r *http.Request, u auth.User) { respond(w, 200, u) }))
	mux.HandleFunc("POST /api/v1/me/subscription", s.withUser(s.setSubscription))
	mux.HandleFunc("GET /api/v1/tracks", s.listTracks)
	mux.HandleFunc("GET /api/v1/tracks/{id}", s.getTrack)
	mux.HandleFunc("GET /api/v1/tracks/{id}/stream", s.withUser(s.stream))
	mux.HandleFunc("POST /api/v1/wave", s.withUser(s.waveSession))
	mux.HandleFunc("GET /api/v1/playback/ws", s.playbackSocket)
	mux.HandleFunc("GET /api/v1/library", s.withUser(s.getLibrary))
	mux.HandleFunc("PUT /api/v1/library", s.withUser(s.putLibrary))
	mux.HandleFunc("GET /api/v1/playlists", s.withUser(s.listPlaylists))
	mux.HandleFunc("POST /api/v1/playlists", s.withUser(s.createPlaylist))
	mux.HandleFunc("GET /api/v1/playlists/{id}/tracks", s.withUser(s.playlistTracks))
	mux.HandleFunc("POST /api/v1/playlists/{id}/tracks", s.withUser(s.addTrack))
	mux.HandleFunc("DELETE /api/v1/playlists/{id}/tracks/{trackID}", s.withUser(s.removeTrack))
	mux.HandleFunc("DELETE /api/v1/playlists/{id}", s.withUser(s.deletePlaylist))
	return s.middleware(mux)
}
