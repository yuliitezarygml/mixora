package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/iulian/soundcloud-go/internal/events"
	"github.com/iulian/soundcloud-go/internal/recommendation"
)

func (s *Server) wave(w http.ResponseWriter, r *http.Request) {
	var input recommendation.Request
	if err := decodeJSON(w, r, &input, 1<<20); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_wave", "Некорректные настройки волны")
		return
	}
	owner := principalFrom(r)
	input.UserID = owner.User.ID
	result, err := s.recommendations.Recommend(r.Context(), input)
	if err != nil {
		if errors.Is(err, recommendation.ErrUnavailable) {
			writeError(w, r, http.StatusServiceUnavailable, "music_unavailable", "Музыкальный сервис временно недоступен")
			return
		}
		writeError(w, r, http.StatusBadGateway, "recommendation_failed", "Не удалось собрать волну")
		return
	}
	sessionID := randomUUID()
	if s.impressions != nil {
		_ = s.impressions.Save(r.Context(), owner.User.ID, sessionID, input, result)
	}
	response := map[string]any{
		"tracks": result.Tracks, "session_id": sessionID,
		"model_version": result.ModelVersion, "reason": result.Reason,
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) waveFeedback(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionId"))
	if !validUUID(sessionID) {
		writeError(w, r, http.StatusBadRequest, "invalid_wave_session", "Некорректная сессия волны")
		return
	}
	var input events.Event
	if err := decodeJSON(w, r, &input, 64<<10); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_feedback", "Некорректная обратная связь")
		return
	}
	input.SessionID = sessionID
	if input.Key == "" {
		input.Key = randomID()
	}
	if err := s.events.Add(r.Context(), principalFrom(r).User.ID, []events.Event{input}); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_feedback", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func randomID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return hex.EncodeToString(value)
}

func randomUUID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}
