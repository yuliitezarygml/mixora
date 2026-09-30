package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PendingItem struct {
	Source      string
	ID          string
	Input       string
	ContentHash string
	Attempts    int
}

type Store struct {
	db         *pgxpool.Pool
	model      string
	dimensions int
}

func NewStore(db *pgxpool.Pool, model string, dimensions int) (*Store, error) {
	if db == nil {
		return nil, errors.New("embedding database is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("embedding model is required")
	}
	if dimensions != 768 {
		return nil, fmt.Errorf("embedding dimensions must be 768, got %d", dimensions)
	}
	return &Store{db: db, model: model, dimensions: dimensions}, nil
}

// TrackDocument intentionally uses only provider-neutral metadata. Stream URLs,
// credentials and provider internals never enter the local embedding model.
func TrackDocument(track music.Track) (string, string) {
	title := strings.Join(strings.Fields(track.Title), " ")
	if title == "" {
		title = "none"
	}
	fields := []struct {
		name  string
		value string
	}{
		{"artist", track.Artist},
		{"album", track.Album},
		{"genre", track.Genre},
		{"tags", track.TagList},
	}
	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.Join(strings.Fields(field.value), " ")
		if value != "" {
			lines = append(lines, field.name+": "+value)
		}
	}
	content := strings.Join(lines, " | ")
	if content == "" {
		content = "unknown track"
	}
	document := "title: " + title + " | text: " + content
	sum := sha256.Sum256([]byte(document))
	return document, hex.EncodeToString(sum[:])
}

func QueryDocument(value string) string {
	query := strings.Join(strings.Fields(value), " ")
	if query == "" {
		query = "music"
	}
	return "task: search result | query: " + query
}

func (s *Store) Pending(ctx context.Context, limit int) ([]PendingItem, error) {
	if limit <= 0 {
		return nil, nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin embedding claim: %w", err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT c.track_source, c.track_id, c.payload,
		       c.embedding_input, c.embedding_input_hash, c.embedding_attempts
		FROM track_catalog c
		LEFT JOIN track_embeddings e
		  ON e.track_source=c.track_source
		 AND e.track_id=c.track_id
		 AND e.model_version=$1
		WHERE c.embedding_available_at <= now()
		  AND (c.embedding_claimed_until IS NULL OR c.embedding_claimed_until <= now())
		  AND (c.embedding_input_hash=''
		       OR e.content_hash IS DISTINCT FROM c.embedding_input_hash)
		ORDER BY c.updated_at, c.track_source, c.track_id
		LIMIT $2
		FOR UPDATE OF c SKIP LOCKED
	`, s.model, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending embeddings: %w", err)
	}
	items := make([]PendingItem, 0, limit)
	for rows.Next() {
		var source, id, input, contentHash string
		var attempts int
		var payload []byte
		if err := rows.Scan(&source, &id, &payload, &input, &contentHash, &attempts); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan pending embedding: %w", err)
		}
		if input == "" || contentHash == "" {
			var track music.Track
			if err := json.Unmarshal(payload, &track); err != nil {
				return nil, fmt.Errorf("decode embedding track %s:%s: %w", source, id, err)
			}
			input, contentHash = TrackDocument(track)
		}
		items = append(items, PendingItem{
			Source: source, ID: id, Input: input, ContentHash: contentHash, Attempts: attempts + 1,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate pending embeddings: %w", err)
	}
	rows.Close()
	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			UPDATE track_catalog
			SET embedding_input=$3,
			    embedding_input_hash=$4,
			    embedding_claimed_until=now() + interval '5 minutes',
			    embedding_attempts=embedding_attempts + 1,
			    embedding_error=NULL
			WHERE track_source=$1 AND track_id=$2
		`, item.Source, item.ID, item.Input, item.ContentHash); err != nil {
			return nil, fmt.Errorf("claim pending embedding: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit embedding claim: %w", err)
	}
	return items, nil
}

func (s *Store) Save(ctx context.Context, items []PendingItem, vectors [][]float32) error {
	if len(items) != len(vectors) {
		return fmt.Errorf("embedding count mismatch: %d items and %d vectors", len(items), len(vectors))
	}
	batch := &pgx.Batch{}
	for index, item := range items {
		if len(vectors[index]) != s.dimensions {
			return fmt.Errorf("embedding %d has %d dimensions, want %d", index, len(vectors[index]), s.dimensions)
		}
		batch.Queue(`
			WITH saved AS (
			  INSERT INTO track_embeddings(
				track_source, track_id, model_version, dimensions, content_hash, embedding
			  ) VALUES ($1,$2,$3,$4,$5,$6::vector)
			  ON CONFLICT (track_source, track_id) DO UPDATE
			  SET model_version=EXCLUDED.model_version,
			      dimensions=EXCLUDED.dimensions,
			      content_hash=EXCLUDED.content_hash,
			      embedding=EXCLUDED.embedding,
			      embedded_at=now()
			  RETURNING 1
			)
			UPDATE track_catalog
			SET embedding_claimed_until=NULL,
			    embedding_attempts=0,
			    embedding_error=NULL,
			    embedding_available_at=now()
			WHERE track_source=$1 AND track_id=$2
		`, item.Source, item.ID, s.model, s.dimensions, item.ContentHash, vectorLiteral(vectors[index]))
	}
	if batch.Len() == 0 {
		return nil
	}
	results := s.db.SendBatch(ctx, batch)
	defer results.Close()
	for range items {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("save track embedding: %w", err)
		}
	}
	return nil
}

func (s *Store) Fail(ctx context.Context, items []PendingItem, cause error) error {
	if len(items) == 0 {
		return nil
	}
	message := "embedding failed"
	if cause != nil {
		message = cause.Error()
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	batch := &pgx.Batch{}
	for _, item := range items {
		delay := 5 * time.Second * time.Duration(1<<min(item.Attempts, 6))
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
		batch.Queue(`
			UPDATE track_catalog
			SET embedding_claimed_until=NULL,
			    embedding_available_at=now() + $3::interval,
			    embedding_error=$4
			WHERE track_source=$1 AND track_id=$2
		`, item.Source, item.ID, delay.String(), message)
	}
	results := s.db.SendBatch(ctx, batch)
	defer results.Close()
	for range items {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("release failed embedding: %w", err)
		}
	}
	return nil
}

// Similar returns opaque track keys ordered by cosine distance from the
// average embedding of the supplied taste seeds.
func (s *Store) Similar(ctx context.Context, seedKeys []string, limit int) ([]string, error) {
	if len(seedKeys) == 0 || limit <= 0 {
		return nil, nil
	}
	var taste *string
	err := s.db.QueryRow(ctx, `
		SELECT avg(embedding)::text
		FROM track_embeddings
		WHERE model_version=$1
		  AND track_source || ':' || track_id = ANY($2::text[])
	`, s.model, seedKeys).Scan(&taste)
	if errors.Is(err, pgx.ErrNoRows) || taste == nil || *taste == "" {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("build taste embedding: %w", err)
	}
	return s.nearest(ctx, *taste, seedKeys, limit)
}

func (s *Store) Nearest(ctx context.Context, vector []float32, exclude []string, limit int) ([]string, error) {
	if len(vector) != s.dimensions {
		return nil, fmt.Errorf("query embedding has %d dimensions, want %d", len(vector), s.dimensions)
	}
	return s.nearest(ctx, vectorLiteral(vector), exclude, limit)
}

func (s *Store) nearest(ctx context.Context, vector string, exclude []string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT track_source || ':' || track_id
		FROM track_embeddings
		WHERE model_version=$1
		  AND NOT (track_source || ':' || track_id = ANY($2::text[]))
		ORDER BY embedding <=> $3::vector, track_source, track_id
		LIMIT $4
	`, s.model, exclude, vector, limit)
	if err != nil {
		return nil, fmt.Errorf("query nearest tracks: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0, limit)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan similar track: %w", err)
		}
		result = append(result, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate similar tracks: %w", err)
	}
	return result, nil
}

func (s *Store) TasteSeeds(ctx context.Context, userID string, limit int) ([]string, error) {
	if strings.TrimSpace(userID) == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT track_source || ':' || track_id AS track_key
		FROM listening_events
		WHERE user_id=$1::uuid
		  AND track_source <> '' AND track_id <> ''
		  AND occurred_at >= now() - interval '180 days'
		GROUP BY track_source, track_id
		HAVING sum(CASE event_type
			WHEN 'like' THEN 8
			WHEN 'add_to_playlist' THEN 7
			WHEN 'repeat' THEN 5
			WHEN 'complete' THEN 4
			WHEN 'listen_30s' THEN 2
			WHEN 'play' THEN 1
			WHEN 'skip' THEN -2
			WHEN 'dislike' THEN -100
			ELSE 0 END) > 0
		ORDER BY sum(CASE event_type
			WHEN 'like' THEN 8 WHEN 'add_to_playlist' THEN 7
			WHEN 'repeat' THEN 5 WHEN 'complete' THEN 4
			WHEN 'listen_30s' THEN 2 WHEN 'play' THEN 1
			WHEN 'skip' THEN -2 WHEN 'dislike' THEN -100 ELSE 0 END) DESC,
			max(occurred_at) DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query user taste seeds: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0, limit)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan user taste seed: %w", err)
		}
		result = append(result, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user taste seeds: %w", err)
	}
	return result, nil
}

func vectorLiteral(vector []float32) string {
	var builder strings.Builder
	builder.Grow(len(vector) * 8)
	builder.WriteByte('[')
	for index, value := range vector {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte(']')
	return builder.String()
}
