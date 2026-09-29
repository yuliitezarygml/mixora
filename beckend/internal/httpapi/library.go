package httpapi

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/jackc/pgx/v5"
	"mixora/beckend/internal/auth"
	"net/http"
)

type libraryDocument struct {
	Likes          []json.RawMessage `json:"likes"`
	Dislikes       []json.RawMessage `json:"dislikes"`
	History        []json.RawMessage `json:"history"`
	Playlists      []json.RawMessage `json:"playlists"`
	Artists        []json.RawMessage `json:"artists"`
	Searches       []json.RawMessage `json:"searches"`
	SavedPlaylists []json.RawMessage `json:"savedPlaylists"`
	Albums         []json.RawMessage `json:"albums"`
	Episodes       []json.RawMessage `json:"episodes"`
	Listens        []json.RawMessage `json:"listens"`
	Pins           []json.RawMessage `json:"pins"`
}

func emptyLibrary() libraryDocument {
	return libraryDocument{
		Likes: []json.RawMessage{}, Dislikes: []json.RawMessage{}, History: []json.RawMessage{}, Playlists: []json.RawMessage{},
		Artists: []json.RawMessage{}, Searches: []json.RawMessage{}, SavedPlaylists: []json.RawMessage{}, Albums: []json.RawMessage{},
		Episodes: []json.RawMessage{}, Listens: []json.RawMessage{},
	}
}

func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request, u auth.User) {
	userID := u.ID
	var data []byte
	err := s.db.QueryRow(r.Context(), "SELECT data FROM user_libraries WHERE user_id=$1", userID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		respond(w, 200, emptyLibrary())
		return
	}
	if err != nil {
		s.log.Error("library read failed", "error", err)
		fail(w, 500, "internal error")
		return
	}
	var doc libraryDocument
	if json.Unmarshal(data, &doc) != nil {
		respond(w, 200, emptyLibrary())
		return
	}
	fillLibrary(&doc)
	respond(w, 200, doc)
}

func (s *Server) putLibrary(w http.ResponseWriter, r *http.Request, u auth.User) {
	userID := u.ID
	var doc libraryDocument
	if !decodeFlex(w, r, &doc, 512<<10) {
		return
	}
	fillLibrary(&doc)
	limits := []struct {
		name  string
		items []json.RawMessage
		max   int
	}{
		{"likes", doc.Likes, 400}, {"dislikes", doc.Dislikes, 400}, {"history", doc.History, 100},
		{"playlists", doc.Playlists, 50}, {"artists", doc.Artists, 200}, {"searches", doc.Searches, 40},
		{"savedPlaylists", doc.SavedPlaylists, 100}, {"albums", doc.Albums, 100}, {"episodes", doc.Episodes, 50},
		{"listens", doc.Listens, 200}, {"pins", doc.Pins, 40},
	}
	for _, item := range limits {
		if len(item.items) > item.max {
			fail(w, 400, item.name+" exceeds the saved limit")
			return
		}
	}
	payload, err := json.Marshal(doc)
	if err != nil {
		fail(w, 400, "invalid library")
		return
	}
	_, err = s.db.Exec(r.Context(), `INSERT INTO user_libraries(user_id,data) VALUES($1,$2::jsonb)
		ON CONFLICT(user_id) DO UPDATE SET data=EXCLUDED.data, updated_at=now()`, userID, payload)
	if err != nil {
		s.log.Error("library save failed", "error", err)
		fail(w, 500, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func fillLibrary(doc *libraryDocument) {
	if doc.Likes == nil {
		doc.Likes = []json.RawMessage{}
	}
	if doc.Dislikes == nil {
		doc.Dislikes = []json.RawMessage{}
	}
	if doc.History == nil {
		doc.History = []json.RawMessage{}
	}
	if doc.Playlists == nil {
		doc.Playlists = []json.RawMessage{}
	}
	if doc.Artists == nil {
		doc.Artists = []json.RawMessage{}
	}
	if doc.Searches == nil {
		doc.Searches = []json.RawMessage{}
	}
	if doc.SavedPlaylists == nil {
		doc.SavedPlaylists = []json.RawMessage{}
	}
	if doc.Albums == nil {
		doc.Albums = []json.RawMessage{}
	}
	if doc.Episodes == nil {
		doc.Episodes = []json.RawMessage{}
	}
	if doc.Listens == nil {
		doc.Listens = []json.RawMessage{}
	}
}

func decodeFlex(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if len(r.Header.Values("Content-Type")) == 0 || r.Header.Get("Content-Type") == "" {
		fail(w, 415, "Content-Type must be application/json")
		return false
	}
	media := r.Header.Get("Content-Type")
	if media != "application/json" && len(media) >= 16 && media[:16] != "application/json" {
		fail(w, 415, "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		fail(w, 400, "invalid JSON body")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		fail(w, 400, "expected one JSON object")
		return false
	}
	return true
}
