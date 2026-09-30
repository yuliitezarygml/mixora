package recommendation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ImpressionStore struct {
	db *pgxpool.Pool
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
	batch := &pgx.Batch{}
	for rank, track := range result.Tracks {
		batch.Queue(`
			INSERT INTO recommendation_impressions(
				user_id, wave_session_id, track_source, track_id, rank,
				reason, model_version, context
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, userID, sessionID, track.Source, track.ID, rank+1,
			result.Reason, result.ModelVersion, contextJSON)
	}
	if batch.Len() == 0 {
		return nil
	}
	results := s.db.SendBatch(ctx, batch)
	defer results.Close()
	for range result.Tracks {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("save recommendation impression: %w", err)
		}
	}
	return nil
}
