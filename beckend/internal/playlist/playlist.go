package playlist

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"mixora/beckend/internal/catalog"
	"strings"
	"unicode/utf8"
)

var ErrName = errors.New("playlist name must contain 1–120 characters")

type Playlist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, user, name string) (Playlist, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 120 {
		return Playlist{}, ErrName
	}
	var p Playlist
	err := r.db.QueryRow(ctx, "INSERT INTO playlists(user_id,name) VALUES($1,$2) RETURNING id,name", user, name).Scan(&p.ID, &p.Name)
	return p, err
}
func (r *Repository) List(ctx context.Context, user string) ([]Playlist, error) {
	rows, err := r.db.Query(ctx, "SELECT id,name FROM playlists WHERE user_id=$1 ORDER BY created_at DESC,id", user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Playlist{}
	for rows.Next() {
		var p Playlist
		if err = rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func (r *Repository) Tracks(ctx context.Context, user, id string) ([]catalog.Track, error) {
	var owned bool
	if err := r.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM playlists WHERE id=$1 AND user_id=$2)", id, user).Scan(&owned); err != nil {
		return nil, err
	}
	if !owned {
		return nil, pgx.ErrNoRows
	}
	rows, err := r.db.Query(ctx, "SELECT t.id,t.title,t.artist,t.album,t.explicit FROM playlist_tracks pt JOIN tracks t ON t.id=pt.track_id WHERE pt.playlist_id=$1 ORDER BY pt.added_at,t.id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []catalog.Track{}
	for rows.Next() {
		var t catalog.Track
		if err = rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.Explicit); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
func (r *Repository) Add(ctx context.Context, user, id, track string) error {
	// The ownership check is part of the write; another user's playlist is never writable.
	tag, err := r.db.Exec(ctx, `INSERT INTO playlist_tracks(playlist_id,track_id) SELECT p.id,t.id FROM playlists p CROSS JOIN tracks t WHERE p.id=$1 AND p.user_id=$2 AND t.id=$3 ON CONFLICT(playlist_id,track_id) DO UPDATE SET track_id=EXCLUDED.track_id`, id, user, track)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
func (r *Repository) Remove(ctx context.Context, user, id, track string) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM playlist_tracks pt USING playlists p WHERE pt.playlist_id=p.id AND p.id=$1 AND p.user_id=$2 AND pt.track_id=$3", id, user, track)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
func (r *Repository) Delete(ctx context.Context, user, id string) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM playlists WHERE id=$1 AND user_id=$2", id, user)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
