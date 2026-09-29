package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.libraries.Get(r.Context(), principalFrom(r).User.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "library_unavailable", "Не удалось загрузить библиотеку")
		return
	}
	w.Header().Set("ETag", `"`+versionString(snapshot.Version)+`"`)
	writeRawJSON(w, http.StatusOK, snapshot.Payload)
}

func (s *Server) putLibrary(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	payload, err := io.ReadAll(r.Body)
	if err != nil || !json.Valid(payload) {
		writeError(w, r, http.StatusBadRequest, "invalid_library", "Некорректная библиотека")
		return
	}
	snapshot, err := s.libraries.Put(r.Context(), principalFrom(r).User.ID, json.RawMessage(payload))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "library_unavailable", "Не удалось сохранить библиотеку")
		return
	}
	w.Header().Set("ETag", `"`+versionString(snapshot.Version)+`"`)
	writeRawJSON(w, http.StatusOK, snapshot.Payload)
}

func versionString(value int64) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	var raw [20]byte
	position := len(raw)
	for value > 0 {
		position--
		raw[position] = digits[value%10]
		value /= 10
	}
	return string(raw[position:])
}
