package httpapi

import (
	"net/http"

	"github.com/iulian/soundcloud-go/internal/library"
)

func (s *Server) getTaste(w http.ResponseWriter, r *http.Request) {
	profile, err := s.libraries.GetTaste(r.Context(), principalFrom(r).User.ID)
	if err != nil {
		writeError(w, r, 503, "taste_unavailable", "Не удалось загрузить музыкальные интересы")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}
func (s *Server) putTaste(w http.ResponseWriter, r *http.Request) {
	var input library.TasteInput
	if err := decodeJSON(w, r, &input, 16<<10); err != nil {
		writeError(w, r, 400, "invalid_taste", "Некорректные музыкальные интересы")
		return
	}
	if _, err := library.NormalizeTaste(input); err != nil {
		writeError(w, r, 400, "invalid_taste", "Выберите минимум пять разных артистов и доступные жанры")
		return
	}
	profile, err := s.libraries.PutTaste(r.Context(), principalFrom(r).User.ID, input)
	if err != nil {
		writeError(w, r, 503, "taste_unavailable", "Не удалось сохранить музыкальные интересы")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}
