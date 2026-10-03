package recommendation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ImpressionStore struct {
	db *pgxpool.Pool
}

func (s *ImpressionStore) Owns(ctx context.Context, userID, sessionID, source, trackID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM recommendation_impressions
			WHERE user_id=$1::uuid AND wave_session_id=$2::uuid
			  AND track_source=$3 AND track_id=$4
		)
	`, userID, sessionID, source, trackID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check recommendation impression: %w", err)
	}
	return exists, nil
}

func NewImpressionStore(db *pgxpool.Pool) *ImpressionStore {
	return &ImpressionStore{db: db}
}

func (s *ImpressionStore) Save(ctx context.Context, userID, sessionID string, request Request, result Result) error {
	contextJSON, err := json.Marshal(map[string]any{
		"preferences": request.Preferences,
		"context":     request.Context,
		"round":       request.Round,
	})
	if err != nil {
		return fmt.Errorf("encode recommendation context: %w", err)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin recommendation impression save: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for rank, track := range result.Tracks {
		batch.Queue(`
			WITH saved AS (
			  INSERT INTO recommendation_impressions(
				  user_id, wave_session_id, track_source, track_id, rank,
				  reason, model_version, context
			  ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			  RETURNING 1
			)
			INSERT INTO listening_events(
				event_key, user_id, event_type, track_source, track_id,
				session_id, context, occurred_at
			) VALUES ($9,$1,'impression',$3,$4,$2::text,$8,now())
			ON CONFLICT (user_id, event_key) WHERE event_key IS NOT NULL DO NOTHING
		`, userID, sessionID, track.Source, track.ID, rank+1,
			result.Reason, result.ModelVersion, contextJSON,
			impressionEventKey(sessionID, track.Source, track.ID))
	}
	if batch.Len() == 0 {
		return nil
	}
	results := tx.SendBatch(ctx, batch)
	for range result.Tracks {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("save recommendation impression: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("close recommendation impression batch: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recommendation impression save: %w", err)
	}
	return nil
}

func impressionEventKey(sessionID, source, trackID string) string {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + source + "\x00" + trackID))
	return fmt.Sprintf("wave-impression:%x", sum[:])
}
