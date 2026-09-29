package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"mixora/beckend/internal/auth"
	"mixora/beckend/internal/playlist"
	"net/http"
	"strings"
)

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		fail(w, 415, "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		fail(w, 400, "invalid JSON body")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		fail(w, 400, "expected one JSON object")
		return false
	}
	return true
}
func (s *Server) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		fail(w, 404, "not found")
	case errors.Is(err, auth.ErrInvalid), errors.Is(err, playlist.ErrName):
		fail(w, 400, err.Error())
	case errors.Is(err, auth.ErrConflict):
		fail(w, 409, err.Error())
	case errors.Is(err, auth.ErrCredentials):
		fail(w, 401, "authentication required or invalid credentials")
	default:
		s.log.Error("request failed", "error", err)
		fail(w, 500, "internal error")
	}
}
func validID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
func pathID(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id := r.PathValue(name)
	if !validID(id) {
		fail(w, 400, "invalid "+name)
		return "", false
	}
	return id, true
}
