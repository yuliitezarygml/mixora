package httpapi

import (
	"mixora/beckend/internal/auth"
	"mixora/beckend/internal/wave"
	"net/http"
)

func (s *Server) waveSession(w http.ResponseWriter, r *http.Request, _ auth.User) {
	var body struct {
		Preferences wave.Preferences `json:"preferences"`
		Context     wave.Context     `json:"context"`
		Round       int              `json:"round"`
		Explicit    *bool            `json:"explicit"`
		Exclude     []wave.Track     `json:"exclude"`
		Likes       []wave.Track     `json:"likes"`
		History     []wave.Track     `json:"history"`
		Dislikes    []wave.Track     `json:"dislikes"`
		Seeds       []wave.Track     `json:"seeds"`
	}
	if !decodeFlex(w, r, &body, 128<<10) {
		return
	}
	if body.Round < 0 {
		body.Round = 0
	}
	if body.Round > 1000 {
		body.Round = 1000
	}
	preferences := wave.Defaults(body.Preferences)
	explicit := true
	if body.Explicit != nil {
		explicit = *body.Explicit
	}
	likes := limitTracks(body.Likes, 40)
	history := limitTracks(body.History, 40)
	candidates := limitTracks(body.Seeds, 40)
	if preferences.Diversity != "familiar" {
		raw, err := s.soundcloud.Search(r.Context(), wave.Query(preferences, likes, body.Context, body.Round), 40, 0)
		if err != nil {
			s.soundcloudError(w, err)
			return
		}
		found, err := wave.FromSoundCloud(raw)
		if err != nil {
			s.log.Error("wave parse failed", "error", err)
			fail(w, 502, "SoundCloud request failed")
			return
		}
		candidates = append(found, candidates...)
	}
	tracks := wave.Build(candidates, wave.Library{Likes: likes, History: history, Dislikes: limitTracks(body.Dislikes, 80)}, preferences, wave.Options{
		Explicit: explicit, Exclude: limitTracks(body.Exclude, 40), Limit: 30,
	})
	if tracks == nil {
		tracks = []wave.Track{}
	}
	respond(w, 200, map[string]any{"tracks": tracks, "query": wave.Query(preferences, likes, body.Context, body.Round)})
}

func limitTracks(tracks []wave.Track, max int) []wave.Track {
	if len(tracks) > max {
		return tracks[:max]
	}
	if tracks == nil {
		return []wave.Track{}
	}
	return tracks
}
