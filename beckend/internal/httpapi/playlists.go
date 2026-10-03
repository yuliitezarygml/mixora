package httpapi

import (
	"errors"
	"net/http"

	"github.com/iulian/soundcloud-go/internal/library"
)

// getPlaylists returns account-owned playlists only. Source playlists remain
// available through the music engine's existing read-only catalog routes.
func (s *Server) getPlaylists(w http.ResponseWriter, r *http.Request) {
	playlists, err := s.libraries.ListPlaylists(r.Context(), principalFrom(r).User.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "playlists_unavailable", "Не удалось загрузить ваши плейлисты")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"playlists": playlists})
}

func (s *Server) putPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistID, err := library.NormalizePlaylistID(r.PathValue("playlistID"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_playlist", err.Error())
		return
	}
	var input library.PlaylistInput
	if err := decodeJSON(w, r, &input, 512<<10); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_playlist", "Некорректные данные плейлиста")
		return
	}
	normalized, err := library.NormalizePlaylistInput(input)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_playlist", err.Error())
		return
	}
	playlist, err := s.libraries.ReplacePlaylist(r.Context(), principalFrom(r).User.ID, playlistID, normalized)
	if errors.Is(err, library.ErrPlaylistIdempotencyConflict) {
		writeError(w, r, http.StatusConflict, "idempotency_conflict", "Этот ключ идемпотентности уже использован для другого изменения плейлиста")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "playlists_unavailable", "Не удалось сохранить плейлист")
		return
	}
	writeJSON(w, http.StatusOK, playlist)
}

func (s *Server) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	playlistID, err := library.NormalizePlaylistID(r.PathValue("playlistID"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_playlist", err.Error())
		return
	}
	var input library.PlaylistDeleteInput
	if err := decodeJSON(w, r, &input, 8<<10); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_playlist", "Некорректное удаление плейлиста")
		return
	}
	normalized, err := library.NormalizePlaylistDeleteInput(input)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_playlist", err.Error())
		return
	}
	result, err := s.libraries.DeletePlaylist(r.Context(), principalFrom(r).User.ID, playlistID, normalized)
	if errors.Is(err, library.ErrPlaylistIdempotencyConflict) {
		writeError(w, r, http.StatusConflict, "idempotency_conflict", "Этот ключ идемпотентности уже использован для другого изменения плейлиста")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "playlists_unavailable", "Не удалось удалить плейлист")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
