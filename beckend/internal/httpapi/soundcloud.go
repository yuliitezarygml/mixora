package httpapi

import (
	"errors"
	"mixora/beckend/internal/auth"
	"mixora/beckend/internal/soundcloud"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (s *Server) soundcloudSearch(w http.ResponseWriter, r *http.Request, _ auth.User) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || utf8.RuneCountInString(q) > 200 {
		fail(w, 400, "q must contain 1–200 characters")
		return
	}
	limit, offset := 20, 0
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
		if err != nil || offset < 0 || offset > 10000 {
			fail(w, 400, "offset must be 0–10000")
			return
		}
	}
	kind := r.PathValue("kind")
	if kind == "" {
		kind = "tracks"
	}
	data, err := s.soundcloud.SearchCatalog(r.Context(), kind, q, limit, offset)
	if err != nil {
		s.soundcloudError(w, err)
		return
	}
	respond(w, 200, data)
}

func (s *Server) soundcloudResource(w http.ResponseWriter, r *http.Request, _ auth.User) {
	data, err := s.soundcloud.Resource(r.Context(), r.PathValue("kind"), r.PathValue("id"), r.PathValue("section"))
	if err != nil {
		s.soundcloudError(w, err)
		return
	}
	respond(w, http.StatusOK, data)
}
func (s *Server) soundcloudStreams(w http.ResponseWriter, r *http.Request, _ auth.User) {
	data, err := s.soundcloud.Streams(r.Context(), r.PathValue("id"))
	if err != nil {
		s.soundcloudError(w, err)
		return
	}
	respond(w, 200, data)
}
func (s *Server) soundcloudError(w http.ResponseWriter, err error) {
	var upstream *soundcloud.APIError
	switch {
	case errors.Is(err, soundcloud.ErrDisabled):
		fail(w, 503, err.Error())
	case errors.Is(err, soundcloud.ErrPlaybackUnavailable):
		fail(w, 403, err.Error())
	case errors.Is(err, soundcloud.ErrID):
		fail(w, 400, err.Error())
	case errors.As(err, &upstream):
		switch upstream.Status {
		case 404:
			fail(w, 404, "SoundCloud track not found")
		case 403:
			fail(w, 403, "SoundCloud playback unavailable")
		case 429:
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "SoundCloud rate limit reached")
		default:
			s.log.Error("SoundCloud failed", "error", err)
			fail(w, 502, "SoundCloud request failed")
		}
	default:
		s.log.Error("SoundCloud failed", "error", err)
		fail(w, 502, "SoundCloud unavailable")
	}
}

func (s *Server) soundcloudPlayback(w http.ResponseWriter, r *http.Request, _ auth.User) {
	playback, err := s.soundcloud.Playback(r.Context(), r.PathValue("id"))
	if err != nil {
		s.soundcloudError(w, err)
		return
	}
	respond(w, http.StatusOK, playback)
}
