package recommendation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Catalog interface {
	Save(context.Context, []music.Track) error
	Find(context.Context, []string) ([]music.Track, error)
}

type PostgresCatalog struct {
	db *pgxpool.Pool
}

func NewCatalog(db *pgxpool.Pool) *PostgresCatalog {
	return &PostgresCatalog{db: db}
}

func (c *PostgresCatalog) Save(ctx context.Context, tracks []music.Track) error {
	batch := &pgx.Batch{}
	seen := make(map[string]struct{}, len(tracks))
	for _, track := range tracks {
		if track.Source == "" || track.ID == "" {
			continue
		}
		if _, exists := seen[track.Key()]; exists {
			continue
		}
		seen[track.Key()] = struct{}{}
		payload, err := json.Marshal(track)
		if err != nil {
			return fmt.Errorf("encode catalog track: %w", err)
		}
		batch.Queue(`
			INSERT INTO track_catalog(track_source, track_id, payload)
			VALUES ($1,$2,$3)
			ON CONFLICT (track_source, track_id) DO UPDATE
			SET payload=EXCLUDED.payload, updated_at=now()
		`, track.Source, track.ID, payload)
	}
	if batch.Len() == 0 {
		return nil
	}
	results := c.db.SendBatch(ctx, batch)
	defer results.Close()
	for range seen {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("save catalog track: %w", err)
		}
	}
	return nil
}

func (c *PostgresCatalog) Find(ctx context.Context, keys []string) ([]music.Track, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := c.db.Query(ctx, `
		SELECT payload
		FROM track_catalog
		WHERE track_source || ':' || track_id = ANY($1::text[])
	`, keys)
	if err != nil {
		return nil, fmt.Errorf("find catalog tracks: %w", err)
	}
	defer rows.Close()
	byKey := make(map[string]music.Track, len(keys))
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan catalog track: %w", err)
		}
		var track music.Track
		if err := json.Unmarshal(payload, &track); err != nil {
			return nil, fmt.Errorf("decode catalog track: %w", err)
		}
		byKey[track.Key()] = track
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate catalog tracks: %w", err)
	}
	result := make([]music.Track, 0, len(byKey))
	for _, key := range keys {
		if track, exists := byKey[key]; exists {
			result = append(result, track)
		}
	}
	return result, nil
}
