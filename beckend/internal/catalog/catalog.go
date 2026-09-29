package catalog

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Track struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Explicit bool   `json:"explicit"`
	MediaKey string `json:"-"`
}
type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) List(ctx context.Context, q string, limit, offset int) ([]Track, error) {
	rows, err := r.db.Query(ctx, `SELECT id,title,artist,album,explicit FROM tracks WHERE $1='' OR strpos(lower(title),lower($1))>0 OR strpos(lower(artist),lower($1))>0 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tracks := []Track{}
	for rows.Next() {
		var t Track
		if err = rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.Explicit); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id string) (Track, error) {
	var t Track
	err := r.db.QueryRow(ctx, "SELECT id,title,artist,album,explicit,media_key FROM tracks WHERE id=$1", id).Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.Explicit, &t.MediaKey)
	return t, err
}
func (r *Repository) Create(ctx context.Context, t Track) (Track, error) {
	err := r.db.QueryRow(ctx, "INSERT INTO tracks(title,artist,album,explicit,media_key) VALUES($1,$2,$3,$4,$5) RETURNING id", t.Title, t.Artist, t.Album, t.Explicit, t.MediaKey).Scan(&t.ID)
	return t, err
}
