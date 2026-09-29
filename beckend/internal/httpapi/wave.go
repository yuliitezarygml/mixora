package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/iulian/soundcloud-go/internal/recommendation"
)

func (s *Server) wave(w http.ResponseWriter, r *http.Request) {
	var input recommendation.Request
	if err := decodeJSON(w, r, &input, 1<<20); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_wave", "Некорректные настройки волны")
		return
	}
	result, err := s.recommendations.Recommend(r.Context(), input)
	if err != nil {
		if errors.Is(err, recommendation.ErrUnavailable) {
			writeError(w, r, http.StatusServiceUnavailable, "music_unavailable", "Музыкальный сервис временно недоступен")
			return
		}
		writeError(w, r, http.StatusBadGateway, "recommendation_failed", "Не удалось собрать волну")
		return
	}
	sessionID := randomID()
	response := map[string]any{
		"tracks": result.Tracks, "session_id": sessionID,
		"model_version": result.ModelVersion, "reason": result.Reason,
	}
	writeJSON(w, http.StatusOK, response)
}

func randomID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return hex.EncodeToString(value)
}
