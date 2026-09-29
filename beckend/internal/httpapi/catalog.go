package httpapi

import (
	"errors"
	"mixora/beckend/internal/auth"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (s *Server) listTracks(w http.ResponseWriter, r *http.Request) {
	limit, offset := 50, 0
	var err error
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > 100 {
			fail(w, 400, "limit must be 1–100")
			return
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 {
			fail(w, 400, "offset must be non-negative")
			return
		}
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 200 {
		fail(w, 400, "query too long")
		return
	}
	tracks, err := s.catalog.List(r.Context(), q, limit, offset)
	if err != nil {
		s.handleError(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": tracks, "limit": limit, "offset": offset})
}
func (s *Server) getTrack(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	t, err := s.catalog.Get(r.Context(), id)
	if err != nil {
		s.handleError(w, err)
		return
	}
	respond(w, 200, t)
}
func (s *Server) stream(w http.ResponseWriter, r *http.Request, _ auth.User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	t, err := s.catalog.Get(r.Context(), id)
	if err != nil {
		s.handleError(w, err)
		return
	}
	// Root.Open prevents escaping the media directory, including through symlinks.
	f, err := s.media.Open(t.MediaKey)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, "audio unavailable")
		} else {
			s.handleError(w, err)
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		s.handleError(w, err)
		return
	}
	if !info.Mode().IsRegular() {
		fail(w, 404, "audio unavailable")
		return
	}
	http.ServeContent(w, r, t.MediaKey, info.ModTime(), f)
}
