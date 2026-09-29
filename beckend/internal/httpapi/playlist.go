package httpapi

import (
	"mixora/beckend/internal/auth"
	"net/http"
)

func (s *Server) listPlaylists(w http.ResponseWriter, r *http.Request, u auth.User) {
	p, err := s.playlists.List(r.Context(), u.ID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": p})
}
func (s *Server) createPlaylist(w http.ResponseWriter, r *http.Request, u auth.User) {
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, err := s.playlists.Create(r.Context(), u.ID, body.Name)
	if err != nil {
		s.handleError(w, err)
		return
	}
	respond(w, 201, p)
}
func (s *Server) playlistTracks(w http.ResponseWriter, r *http.Request, u auth.User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	tracks, err := s.playlists.Tracks(r.Context(), u.ID, id)
	if err != nil {
		s.handleError(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": tracks})
}
func (s *Server) addTrack(w http.ResponseWriter, r *http.Request, u auth.User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		TrackID string `json:"track_id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !validID(body.TrackID) {
		fail(w, 400, "invalid track_id")
		return
	}
	if err := s.playlists.Add(r.Context(), u.ID, id, body.TrackID); err != nil {
		s.handleError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) removeTrack(w http.ResponseWriter, r *http.Request, u auth.User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	track, ok := pathID(w, r, "trackID")
	if !ok {
		return
	}
	if err := s.playlists.Remove(r.Context(), u.ID, id, track); err != nil {
		s.handleError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) deletePlaylist(w http.ResponseWriter, r *http.Request, u auth.User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.playlists.Delete(r.Context(), u.ID, id); err != nil {
		s.handleError(w, err)
		return
	}
	w.WriteHeader(204)
}
