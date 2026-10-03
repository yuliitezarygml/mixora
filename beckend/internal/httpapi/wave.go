package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/iulian/soundcloud-go/internal/events"
	"github.com/iulian/soundcloud-go/internal/music"
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
	if sessionID == "" || s.impressions == nil {
		writeError(w, r, http.StatusServiceUnavailable, "wave_persistence_failed", "Не удалось сохранить сессию волны")
		return
	}
	if err := s.impressions.Save(r.Context(), owner.User.ID, sessionID, input, result); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "wave_persistence_failed", "Не удалось сохранить сессию волны")
		return
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
	input.Type = strings.TrimSpace(input.Type)
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	input.TrackID = music.CanonicalTrackID(input.Source, input.TrackID)
	if !waveFeedbackTypes[input.Type] {
		writeError(w, r, http.StatusBadRequest, "invalid_feedback_type", "Это событие нельзя отправить как обратную связь волны")
		return
	}
	if s.impressions == nil {
		writeError(w, r, http.StatusServiceUnavailable, "wave_persistence_failed", "Сессия волны временно недоступна")
		return
	}
	owned, err := s.impressions.Owns(r.Context(), principalFrom(r).User.ID, sessionID, input.Source, input.TrackID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "feedback_check_failed", "Не удалось проверить сессию волны")
		return
	}
	if !owned {
		writeError(w, r, http.StatusBadRequest, "unknown_wave_track", "Трек не относится к этой сессии волны")
		return
	}
	input.SessionID = sessionID
	if err := s.events.Add(r.Context(), principalFrom(r).User.ID, []events.Event{input}); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_feedback", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var waveFeedbackTypes = map[string]bool{
	"play": true, "listen_30s": true, "complete": true, "skip": true,
	"repeat": true, "like": true, "dislike": true, "add_to_playlist": true,
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
