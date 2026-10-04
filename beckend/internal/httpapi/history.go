package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iulian/soundcloud-go/internal/library"
)

const defaultHistoryLimit = 50

func (s *Server) getHistory(w http.ResponseWriter, r *http.Request) {
	limit, err := historyLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_history_limit", "Некорректный лимит истории")
		return
	}
	history, err := s.libraries.GetHistory(r.Context(), principalFrom(r).User.ID, limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "history_unavailable", "Не удалось загрузить историю прослушивания")
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) putHistory(w http.ResponseWriter, r *http.Request) {
	var input library.HistoryInput
	if err := decodeJSON(w, r, &input, 64<<10); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_history", "Некорректная запись истории")
		return
	}
	normalized, err := library.NormalizeHistoryInput(input)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_history", err.Error())
		return
	}
	entry, err := s.libraries.RecordHistory(r.Context(), principalFrom(r).User.ID, normalized)
	if errors.Is(err, library.ErrHistoryIdempotencyKeyConflict) {
		writeError(w, r, http.StatusConflict, "idempotency_conflict", "Этот ключ идемпотентности уже использован для другой записи истории")
		return
	}
	if errors.Is(err, library.ErrHistoryGenerationConflict) {
		writeError(w, r, http.StatusConflict, "history_generation_conflict", "История была очищена до сохранения этой записи")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "history_unavailable", "Не удалось сохранить историю прослушивания")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) deleteHistory(w http.ResponseWriter, r *http.Request) {
	generation, err := s.libraries.ClearHistory(r.Context(), principalFrom(r).User.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "history_unavailable", "Не удалось очистить историю прослушивания")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"generation": generation})
}

func historyLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultHistoryLimit, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 100 {
		return 0, errors.New("history limit must be between 1 and 100")
	}
	return value, nil
}
