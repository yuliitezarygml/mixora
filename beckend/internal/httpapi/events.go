package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/iulian/soundcloud-go/internal/events"
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
	if err := s.events.Add(r.Context(), principalFrom(r).User.ID, input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_events", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
