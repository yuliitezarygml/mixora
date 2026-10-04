package httpapi

import (
	"errors"
	"net/http"

	"github.com/iulian/soundcloud-go/internal/library"
)

// The domain permits 500 compact snapshots. Eight MiB covers their valid JSON
// representation even when strings require escaping, while still bounding an
// untrusted request well below an unbounded decoder allocation.
const maxPlaylistRequestBytes = 8 << 20

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
	if err := decodeJSON(w, r, &input, maxPlaylistRequestBytes); err != nil {
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
	if errors.Is(err, library.ErrPlaylistRevisionConflict) {
		writeError(w, r, http.StatusConflict, "playlist_revision_conflict", "Плейлист был изменён на другом устройстве")
		return
	}
	if errors.Is(err, library.ErrPlaylistLimitReached) {
		writeError(w, r, http.StatusConflict, "playlist_limit_reached", "В аккаунте можно хранить не более 50 плейлистов")
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
	if errors.Is(err, library.ErrPlaylistRevisionConflict) {
		writeError(w, r, http.StatusConflict, "playlist_revision_conflict", "Плейлист был изменён на другом устройстве")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "playlists_unavailable", "Не удалось удалить плейлист")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
