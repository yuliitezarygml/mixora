package httpapi

import (
	"errors"
	"net/http"

	"github.com/iulian/soundcloud-go/internal/library"
)

// getTrackPreferences returns the server-owned desired state. The legacy
// library snapshot remains available during migration, but callers must use
// this endpoint for likes, dislikes, and neutral retractions.
func (s *Server) getTrackPreferences(w http.ResponseWriter, r *http.Request) {
	preferences, err := s.libraries.ListTrackPreferences(r.Context(), principalFrom(r).User.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "preferences_unavailable", "Не удалось загрузить музыкальные предпочтения")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preferences": preferences})
}

// putTrackPreference stores a single idempotent desired state. Input is
// normalized at the transport boundary so invalid user data is never confused
// with an internal storage outage.
func (s *Server) putTrackPreference(w http.ResponseWriter, r *http.Request) {
	var input library.PreferenceInput
	if err := decodeJSON(w, r, &input, 64<<10); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_track_preference", "Некорректное музыкальное предпочтение")
		return
	}
	normalized, err := library.NormalizePreferenceInput(input)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_track_preference", err.Error())
		return
	}
	preference, err := s.libraries.SetTrackPreference(r.Context(), principalFrom(r).User.ID, normalized)
	if errors.Is(err, library.ErrIdempotencyKeyConflict) {
		writeError(w, r, http.StatusConflict, "idempotency_conflict", "Этот ключ идемпотентности уже использован для другого предпочтения")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "preferences_unavailable", "Не удалось сохранить музыкальное предпочтение")
		return
	}
	writeJSON(w, http.StatusOK, preference)
}
