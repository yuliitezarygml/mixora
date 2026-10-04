package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/auth"
	"github.com/iulian/soundcloud-go/internal/events"
	"github.com/iulian/soundcloud-go/internal/library"
	"github.com/iulian/soundcloud-go/internal/mail"
	"github.com/iulian/soundcloud-go/internal/playback"
	"github.com/iulian/soundcloud-go/internal/recommendation"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "mixora_session"

type Config struct {
	Database        *pgxpool.Pool
	Auth            *auth.Service
	Libraries       *library.Store
	Events          *events.Store
	MailOutbox      *mail.Outbox
	Playback        *playback.Hub
	Recommendations *recommendation.Service
	Impressions     *recommendation.ImpressionStore
	MusicEngine     http.Handler
	PublicURL       string
	CookieSecure    bool
	SessionTTL      time.Duration
}

// Keep the HTTP layer coupled to behavior rather than storage implementations.
// Config intentionally continues to accept the concrete production services;
// these narrow interfaces make the transport contract testable without a
// PostgreSQL or external music-service process.
type authBackend interface {
	Register(context.Context, string, string, string) (auth.Registration, error)
	Login(context.Context, string, string, auth.SessionMetadata) (auth.Login, error)
	Authenticate(context.Context, string) (auth.User, auth.Session, error)
	Logout(context.Context, string) error
	VerifyEmail(context.Context, string) (auth.User, error)
	BeginPasswordReset(context.Context, string) (*auth.PasswordReset, error)
	ResetPassword(context.Context, string, string) (auth.User, error)
}

type libraryBackend interface {
	Get(context.Context, string) (library.Snapshot, error)
	Put(context.Context, string, json.RawMessage) (library.Snapshot, error)
	ListTrackPreferences(context.Context, string) ([]library.TrackPreference, error)
	SetTrackPreference(context.Context, string, library.PreferenceInput) (library.TrackPreference, error)
	GetHistory(context.Context, string, int) (library.HistorySnapshot, error)
	RecordHistory(context.Context, string, library.HistoryInput) (library.HistoryEntry, error)
	ClearHistory(context.Context, string) (int64, error)
	ListPlaylists(context.Context, string) ([]library.Playlist, error)
	ReplacePlaylist(context.Context, string, string, library.PlaylistInput) (library.Playlist, error)
	DeletePlaylist(context.Context, string, string, library.PlaylistDeleteInput) (library.PlaylistDeleteResult, error)
}

type eventBackend interface {
	Add(context.Context, string, []events.Event) error
}

type mailBackend interface {
	Enqueue(context.Context, mail.Message) (int64, error)
}

type recommendationBackend interface {
	Recommend(context.Context, recommendation.Request) (recommendation.Result, error)
}

type impressionBackend interface {
	Save(context.Context, string, string, recommendation.Request, recommendation.Result) error
	Owns(context.Context, string, string, string, string) (bool, error)
}

type Server struct {
	db              *pgxpool.Pool
	auth            authBackend
	libraries       libraryBackend
	events          eventBackend
	mailOutbox      mailBackend
	playback        *playback.Hub
	recommendations recommendationBackend
	impressions     impressionBackend
	publicURL       string
	cookieSecure    bool
	sessionTTL      time.Duration
}

type principal struct {
	User    auth.User
	Session auth.Session
	Token   string
}

type principalContextKey struct{}

func New(config Config) (http.Handler, error) {
	if config.Database == nil || config.Auth == nil || config.Libraries == nil || config.Events == nil || config.MailOutbox == nil || config.Playback == nil || config.Recommendations == nil || config.Impressions == nil {
		return nil, fmt.Errorf("http api dependencies are incomplete")
	}
	if config.MusicEngine == nil {
		return nil, fmt.Errorf("music engine handler is required")
	}
	if config.SessionTTL < time.Hour {
		return nil, fmt.Errorf("session TTL must be at least one hour")
	}
	s := &Server{
		db: config.Database, auth: config.Auth, libraries: config.Libraries,
		events: config.Events, mailOutbox: config.MailOutbox, playback: config.Playback, recommendations: config.Recommendations,
		impressions: config.Impressions,
		publicURL:   strings.TrimRight(config.PublicURL, "/"), cookieSecure: config.CookieSecure,
		sessionTTL: config.SessionTTL,
	}

	return s.handler(config.MusicEngine), nil
}

func (s *Server) handler(musicEngine http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /ready", s.ready)
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/session", s.session)
	mux.HandleFunc("POST /api/v1/auth/resume", s.resume)
	mux.HandleFunc("GET /api/v1/auth/verify-email", s.verifyEmail)
	mux.HandleFunc("POST /api/v1/auth/verify-email", s.verifyEmail)
	mux.HandleFunc("POST /api/v1/auth/password/request", s.requestPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/password/reset", s.resetPassword)
	mux.Handle("GET /api/v1/me", s.requireAuth(http.HandlerFunc(s.me)))
	mux.Handle("POST /api/v1/me/subscription", s.requireAuth(http.HandlerFunc(s.setSubscription)))
	mux.Handle("GET /api/v1/library", s.requireAuth(http.HandlerFunc(s.getLibrary)))
	mux.Handle("PUT /api/v1/library", s.requireAuth(http.HandlerFunc(s.putLibrary)))
	mux.Handle("GET /api/v1/me/track-preferences", s.requireAuth(http.HandlerFunc(s.getTrackPreferences)))
	mux.Handle("PUT /api/v1/me/track-preferences", s.requireAuth(http.HandlerFunc(s.putTrackPreference)))
	mux.Handle("GET /api/v1/history", s.requireAuth(http.HandlerFunc(s.getHistory)))
	mux.Handle("PUT /api/v1/me/history", s.requireAuth(http.HandlerFunc(s.putHistory)))
	mux.Handle("DELETE /api/v1/me/history", s.requireAuth(http.HandlerFunc(s.deleteHistory)))
	mux.Handle("GET /api/v1/me/playlists", s.requireAuth(http.HandlerFunc(s.getPlaylists)))
	mux.Handle("PUT /api/v1/me/playlists/{playlistID}", s.requireAuth(http.HandlerFunc(s.putPlaylist)))
	mux.Handle("DELETE /api/v1/me/playlists/{playlistID}", s.requireAuth(http.HandlerFunc(s.deletePlaylist)))
	mux.Handle("POST /api/v1/events", s.requireAuth(http.HandlerFunc(s.addEvents)))
	mux.Handle("POST /api/v1/wave", s.requireAuth(http.HandlerFunc(s.wave)))
	mux.Handle("POST /api/v1/wave/{sessionId}/feedback", s.requireAuth(http.HandlerFunc(s.waveFeedback)))
	mux.Handle("GET /api/v1/playback/ws", s.requireAuth(http.HandlerFunc(s.playbackWebSocket)))
	mux.Handle("/", musicEngine)
	return middleware(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "mixora-api"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "database_unavailable", "База данных недоступна")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			writeError(w, r, http.StatusUnauthorized, "authentication_required", "Войдите в аккаунт, чтобы продолжить")
			return
		}
		user, session, err := s.auth.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			s.clearSessionCookie(w)
			writeError(w, r, http.StatusUnauthorized, "invalid_session", "Сессия закончилась. Войдите снова")
			return
		}
		ctx := context.WithValue(r.Context(), principalContextKey{}, principal{User: user, Session: session, Token: cookie.Value})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func principalFrom(r *http.Request) principal {
	value, _ := r.Context().Value(principalContextKey{}).(principal)
	return value
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	maxAge := int(s.sessionTTL.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/api/v1", MaxAge: maxAge,
		Expires: time.Now().Add(s.sessionTTL), HttpOnly: true, Secure: s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/api/v1", MaxAge: -1,
		Expires: time.Unix(1, 0), HttpOnly: true, Secure: s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func sessionMetadata(r *http.Request) auth.SessionMetadata {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = ""
	}
	return auth.SessionMetadata{
		DeviceID:  strings.TrimSpace(r.Header.Get("X-Mixora-Device-ID")),
		UserAgent: truncate(r.UserAgent(), 500), IPAddress: host,
	}
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func authError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, r, http.StatusConflict, "email_taken", "Этот адрес уже зарегистрирован")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, r, http.StatusUnauthorized, "invalid_credentials", "Неверная почта или пароль")
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, r, http.StatusBadRequest, "weak_password", "Пароль должен содержать от 12 до 72 символов")
	case errors.Is(err, auth.ErrInvalidEmail), errors.Is(err, auth.ErrInvalidDisplayName):
		writeError(w, r, http.StatusBadRequest, "invalid_profile", "Проверьте почту и имя")
	case errors.Is(err, auth.ErrInvalidToken), errors.Is(err, auth.ErrTokenUnavailable):
		writeError(w, r, http.StatusBadRequest, "invalid_token", "Ссылка недействительна или устарела")
	default:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось выполнить запрос")
	}
}
