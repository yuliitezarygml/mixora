package httpapi

import (
	"mixora/beckend/internal/auth"
	"net/http"
)

func (s *Server) withUser(next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("mixora_session")
		if err != nil {
			fail(w, 401, "authentication required")
			return
		}
		u, err := s.auth.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			s.handleError(w, err)
			return
		}
		next(w, r, u)
	}
}

type accountSession struct {
	auth.User
	Token string `json:"token"`
}

func (s *Server) setCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: "mixora_session", Value: token, Path: "/", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := s.auth.Register(r.Context(), body.Email, body.Password, body.DisplayName)
	if err != nil {
		s.handleError(w, err)
		return
	}
	respond(w, 201, u)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, token, err := s.auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.setCookie(w, token, int(auth.SessionTTL.Seconds()))
	respond(w, 200, accountSession{User: u, Token: token})
}
func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := s.auth.Authenticate(r.Context(), body.Token)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.setCookie(w, body.Token, int(auth.SessionTTL.Seconds()))
	respond(w, 200, accountSession{User: u, Token: body.Token})
}
func (s *Server) session(w http.ResponseWriter, r *http.Request, u auth.User) {
	cookie, err := r.Cookie("mixora_session")
	if err != nil || cookie.Value == "" {
		fail(w, 401, "authentication required")
		return
	}
	respond(w, 200, accountSession{User: u, Token: cookie.Value})
}
func (s *Server) setSubscription(w http.ResponseWriter, r *http.Request, u auth.User) {
	var body struct {
		Plus bool `json:"plus"`
	}
	if !decode(w, r, &body) {
		return
	}
	updated, err := s.auth.SetPlus(r.Context(), u.ID, body.Plus)
	if err != nil {
		s.handleError(w, err)
		return
	}
	respond(w, 200, updated)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request, _ auth.User) {
	cookie, _ := r.Cookie("mixora_session")
	if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
		s.handleError(w, err)
		return
	}
	s.setCookie(w, "", -1)
	w.WriteHeader(204)
}
