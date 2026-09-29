package httpapi

import (
	"context"
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
	MusicEngine     http.Handler
	PublicURL       string
	CookieSecure    bool
	SessionTTL      time.Duration
}

type Server struct {
	db              *pgxpool.Pool
	auth            *auth.Service
	libraries       *library.Store
	events          *events.Store
	mailOutbox      *mail.Outbox
	playback        *playback.Hub
	recommendations *recommendation.Service
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
	if config.Database == nil || config.Auth == nil || config.Libraries == nil || config.Events == nil || config.MailOutbox == nil || config.Playback == nil || config.Recommendations == nil {
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
		publicURL: strings.TrimRight(config.PublicURL, "/"), cookieSecure: config.CookieSecure,
		sessionTTL: config.SessionTTL,
	}

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
	mux.Handle("POST /api/v1/events", s.requireAuth(http.HandlerFunc(s.addEvents)))
	mux.Handle("POST /api/v1/wave", s.requireAuth(http.HandlerFunc(s.wave)))
	mux.Handle("GET /api/v1/playback/ws", s.requireAuth(http.HandlerFunc(s.playbackWebSocket)))
	mux.Handle("/", config.MusicEngine)
	return middleware(mux), nil
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
