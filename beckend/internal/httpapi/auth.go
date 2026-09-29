package httpapi

import (
	"database/sql"
	"net/http"

	"github.com/iulian/soundcloud-go/internal/auth"
	"github.com/iulian/soundcloud-go/internal/mail"
)

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var input registerRequest
	if err := decodeJSON(w, r, &input, 32*1024); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "Некорректные данные регистрации")
		return
	}
	registration, err := s.auth.Register(r.Context(), input.Email, input.Password, input.DisplayName)
	if err != nil {
		authError(w, r, err)
		return
	}
	message := mail.VerificationMessage(
		s.publicURL, registration.User.Email, registration.User.DisplayName,
		registration.EmailVerificationToken,
	)
	if _, err := s.mailOutbox.Enqueue(r.Context(), message); err != nil {
		// The account is valid even if SMTP is temporarily down; outbox insertion
		// failing is surfaced so the verification can be requested again later.
		writeError(w, r, http.StatusInternalServerError, "email_queue_failed", "Аккаунт создан, но письмо пока не отправлено")
		return
	}
	writeJSON(w, http.StatusCreated, registration.User)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := decodeJSON(w, r, &input, 16*1024); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "Некорректные данные входа")
		return
	}
	result, err := s.auth.Login(r.Context(), input.Email, input.Password, sessionMetadata(r))
	if err != nil {
		authError(w, r, err)
		return
	}
	s.setSessionCookie(w, result.SessionToken)
	writeJSON(w, http.StatusOK, result.User)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = s.auth.Logout(r.Context(), cookie.Value)
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, principalFrom(r).User)
	})).ServeHTTP(w, r)
}

// resume is a migration endpoint for old desktop profiles. New clients do not
// persist or receive session tokens outside the HttpOnly cookie.
func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &input, 8*1024); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "Некорректная сессия")
		return
	}
	user, _, err := s.auth.Authenticate(r.Context(), input.Token)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "invalid_session", "Сессия закончилась. Войдите снова")
		return
	}
	s.setSessionCookie(w, input.Token)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if r.Method == http.MethodPost {
		var input struct {
			Token string `json:"token"`
		}
		if err := decodeJSON(w, r, &input, 8*1024); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_json", "Некорректная ссылка")
			return
		}
		token = input.Token
	}
	user, err := s.auth.VerifyEmail(r.Context(), token)
	if err != nil {
		authError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &input, 8*1024); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "Некорректная почта")
		return
	}
	reset, err := s.auth.BeginPasswordReset(r.Context(), input.Email)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось выполнить запрос")
		return
	}
	if reset != nil {
		message := mail.PasswordResetMessage(s.publicURL, reset.User.Email, reset.User.DisplayName, reset.Token)
		_, _ = s.mailOutbox.Enqueue(r.Context(), message)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input, 16*1024); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "Некорректные данные")
		return
	}
	user, err := s.auth.ResetPassword(r.Context(), input.Token, input.Password)
	if err != nil {
		authError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, principalFrom(r).User)
}

func (s *Server) setSubscription(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Plus bool `json:"plus"`
	}
	if err := decodeJSON(w, r, &input, 4*1024); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "Некорректные данные")
		return
	}
	var user auth.User
	var verified sql.NullTime
	err := s.db.QueryRow(r.Context(), `
		UPDATE users SET plus=$2, updated_at=now() WHERE id=$1
		RETURNING id::text,email,display_name,email_verified_at,status,plus,created_at,updated_at
	`, principalFrom(r).User.ID, input.Plus).Scan(
		&user.ID, &user.Email, &user.DisplayName, &verified, &user.Status,
		&user.Plus, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось обновить профиль")
		return
	}
	if verified.Valid {
		user.EmailVerifiedAt = &verified.Time
	}
	writeJSON(w, http.StatusOK, user)
}
