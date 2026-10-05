package recommendation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iulian/soundcloud-go/internal/embedding"
	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Catalog interface {
	Save(context.Context, []music.Track) error
	Find(context.Context, []string) ([]music.Track, error)
}

// CandidateCatalog is optional because older catalog implementations only
// needed direct lookup for Gorse/content IDs. A provider-neutral candidate
// pool lets Wave stay available when SoundCloud is temporarily unavailable and
// lets observed Spotify, YouTube, Bandcamp and VK tracks participate in a
// cold-start fallback.
type CandidateCatalog interface {
	ListCandidates(context.Context, int) ([]music.Track, error)
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
		track = music.CanonicalTrack(track)
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
		embeddingInput, embeddingHash := embedding.TrackDocument(track)
		batch.Queue(`
			INSERT INTO track_catalog(
				track_source, track_id, payload, embedding_input, embedding_input_hash
			)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (track_source, track_id) DO UPDATE
			SET payload=EXCLUDED.payload,
			    embedding_input=EXCLUDED.embedding_input,
			    embedding_input_hash=EXCLUDED.embedding_input_hash,
			    embedding_available_at=CASE
			      WHEN track_catalog.embedding_input_hash IS DISTINCT FROM EXCLUDED.embedding_input_hash
			      THEN now() ELSE track_catalog.embedding_available_at END,
			    embedding_claimed_until=CASE
			      WHEN track_catalog.embedding_input_hash IS DISTINCT FROM EXCLUDED.embedding_input_hash
			      THEN NULL ELSE track_catalog.embedding_claimed_until END,
			    updated_at=now()
		`, track.Source, track.ID, payload, embeddingInput, embeddingHash)
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
	canonicalKeys, legacySoundCloudIDs := canonicalCatalogLookupKeys(keys)
	if len(canonicalKeys) == 0 {
		return nil, nil
	}
	rows, err := c.db.Query(ctx, `
		SELECT track_source, track_id, payload
		FROM track_catalog
		WHERE track_source || ':' || track_id = ANY($1::text[])
		   OR (
			LOWER(track_source) = 'soundcloud'
			AND track_id ~ '(^|:|/)[0-9]+$'
			AND substring(track_id FROM '([0-9]+)$') = ANY($2::text[])
		   )
	`, canonicalKeys, legacySoundCloudIDs)
	if err != nil {
		return nil, fmt.Errorf("find catalog tracks: %w", err)
	}
	defer rows.Close()
	byKey := make(map[string]music.Track, len(keys))
	for rows.Next() {
		var source, id string
		var payload []byte
		if err := rows.Scan(&source, &id, &payload); err != nil {
			return nil, fmt.Errorf("scan catalog track: %w", err)
		}
		var track music.Track
		if err := json.Unmarshal(payload, &track); err != nil {
			return nil, fmt.Errorf("decode catalog track: %w", err)
		}
		// The catalog primary key is authoritative: old payloads may contain a
		// legacy SoundCloud URN/path-like identity even when the row is valid.
		track.Source = source
		track.ID = id
		track = music.CanonicalTrack(track)
		byKey[track.Key()] = track
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate catalog tracks: %w", err)
	}
	result := make([]music.Track, 0, len(byKey))
	for _, key := range canonicalKeys {
		if track, exists := byKey[key]; exists {
			result = append(result, track)
		}
	}
	return result, nil
}

// canonicalCatalogLookupKeys keeps catalog reads compatible with pre-canonical
// SoundCloud rows without widening a lookup beyond the requested provider IDs.
// New writes are canonicalized by Save; the second result is only used by Find
// to recognize historical SoundCloud URN/path-like database keys.
func canonicalCatalogLookupKeys(keys []string) ([]string, []string) {
	canonical := make([]string, 0, len(keys))
	legacySoundCloudIDs := make([]string, 0, len(keys))
	seenCanonical := make(map[string]struct{}, len(keys))
	seenLegacy := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		source, id, found := strings.Cut(key, ":")
		if !found || strings.TrimSpace(source) == "" || strings.TrimSpace(id) == "" {
			if key != "" {
				if _, exists := seenCanonical[key]; !exists {
					seenCanonical[key] = struct{}{}
					canonical = append(canonical, key)
				}
			}
			continue
		}
		track := music.CanonicalTrack(music.Track{Source: source, ID: id})
		canonicalKey := track.Key()
		if track.Source == "" || track.ID == "" || canonicalKey == ":" {
			continue
		}
		if _, exists := seenCanonical[canonicalKey]; !exists {
			seenCanonical[canonicalKey] = struct{}{}
			canonical = append(canonical, canonicalKey)
		}
		if track.Source == "soundcloud" && decimalTrackID(track.ID) {
			if _, exists := seenLegacy[track.ID]; !exists {
				seenLegacy[track.ID] = struct{}{}
				legacySoundCloudIDs = append(legacySoundCloudIDs, track.ID)
			}
		}
	}
	return canonical, legacySoundCloudIDs
}

func decimalTrackID(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// ListCandidates returns recently observed provider-neutral tracks. It is not
// a search endpoint and deliberately returns only compact catalog snapshots;
// playback still refreshes an audio URL through the owning music provider.
func (c *PostgresCatalog) ListCandidates(ctx context.Context, limit int) ([]music.Track, error) {
	if c == nil || c.db == nil || limit <= 0 {
		return nil, nil
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := c.db.Query(ctx, `
		SELECT payload
		FROM track_catalog
		WHERE COALESCE(payload->>'access', '') <> 'blocked'
		ORDER BY updated_at DESC, track_source, track_id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list catalog candidates: %w", err)
	}
	defer rows.Close()
	tracks := make([]music.Track, 0, limit)
	seen := make(map[string]struct{}, limit)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan catalog candidate: %w", err)
		}
		var track music.Track
		if err := json.Unmarshal(payload, &track); err != nil {
			return nil, fmt.Errorf("decode catalog candidate: %w", err)
		}
		track = music.CanonicalTrack(track)
		if track.Source == "" || track.ID == "" || track.Access == "blocked" {
			continue
		}
		if _, exists := seen[track.Key()]; exists {
			continue
		}
		seen[track.Key()] = struct{}{}
		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate catalog candidates: %w", err)
	}
	return tracks, nil
}

var _ CandidateCatalog = (*PostgresCatalog)(nil)
