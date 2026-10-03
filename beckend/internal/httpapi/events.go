package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/iulian/soundcloud-go/internal/events"
	"github.com/iulian/soundcloud-go/internal/music"
)

func (s *Server) addEvents(w http.ResponseWriter, r *http.Request) {
	var body json.RawMessage
	if err := decodeJSON(w, r, &body, 512*1024); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_events", "Некорректные события")
		return
	}
	var input []events.Event
	if len(body) > 0 && body[0] == '[' {
		if err := json.Unmarshal(body, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_events", "Некорректные события")
			return
		}
	} else {
		var envelope struct {
			Events []events.Event `json:"events"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_events", "Некорректные события")
			return
		}
		input = envelope.Events
	}
	if !s.ownsWaveEvents(w, r, input) {
		return
	}
	if err := s.events.Add(r.Context(), principalFrom(r).User.ID, input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_events", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ownsWaveEvents keeps the generic offline event endpoint from bypassing the
// ownership check enforced by /wave/{sessionId}/feedback. A Wave session is
// attached only to tracks the server actually returned to this user.
func (s *Server) ownsWaveEvents(w http.ResponseWriter, r *http.Request, input []events.Event) bool {
	for _, event := range input {
		sessionID := strings.TrimSpace(event.SessionID)
		if sessionID == "" {
			continue
		}
		if !validUUID(sessionID) {
			writeError(w, r, http.StatusBadRequest, "invalid_wave_session", "Некорректная сессия волны")
			return false
		}
		if !events.IsWaveSessionEventType(event.Type) {
			writeError(w, r, http.StatusBadRequest, "invalid_wave_event", "Это событие нельзя привязать к сессии волны")
			return false
		}
		if s.impressions == nil {
			writeError(w, r, http.StatusBadRequest, "unknown_wave_track", "Трек не относится к этой сессии волны")
			return false
		}
		source := strings.ToLower(strings.TrimSpace(event.Source))
		trackID := music.CanonicalTrackID(source, event.TrackID)
		owned, err := s.impressions.Owns(r.Context(), principalFrom(r).User.ID, sessionID, source, trackID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "feedback_check_failed", "Не удалось проверить сессию волны")
			return false
		}
		if !owned {
			writeError(w, r, http.StatusBadRequest, "unknown_wave_track", "Трек не относится к этой сессии волны")
			return false
		}
	}
	return true
}
