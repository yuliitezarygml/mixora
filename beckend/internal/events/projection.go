package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/iulian/soundcloud-go/internal/recommendationlock"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FeedbackAggregate struct {
	UserID   string
	Type     string
	Source   string
	TrackID  string
	Count    float64
	LatestAt time.Time
}

type FeedbackProjector interface {
	Project(context.Context, []FeedbackAggregate) error
}

type ProjectionOptions struct {
	BatchSize int
	Interval  time.Duration
	Timeout   time.Duration
	OnError   func(error)
}

type ProjectionWorker struct {
	db        *pgxpool.Pool
	projector FeedbackProjector
	options   ProjectionOptions
}

type projectionKey struct {
	UserID  string `json:"user_id"`
	Type    string `json:"event_type"`
	Source  string `json:"track_source"`
	TrackID string `json:"track_id"`
}

type pendingProjection struct {
	ID  string
	Key projectionKey
}

func DefaultProjectionOptions() ProjectionOptions {
	return ProjectionOptions{BatchSize: 100, Interval: 5 * time.Second, Timeout: 5 * time.Second}
}

func NewProjectionWorker(db *pgxpool.Pool, projector FeedbackProjector, options ProjectionOptions) (*ProjectionWorker, error) {
	if db == nil || projector == nil {
		return nil, errors.New("projection worker dependencies are required")
	}
	if options.BatchSize <= 0 || options.BatchSize > 1000 {
		return nil, errors.New("projection batch size must be between 1 and 1000")
	}
	if options.Interval <= 0 || options.Timeout <= 0 {
		return nil, errors.New("projection interval and timeout must be positive")
	}
	return &ProjectionWorker{db: db, projector: projector, options: options}, nil
}

func (w *ProjectionWorker) Run(ctx context.Context) error {
	if err := w.processAndReport(ctx); err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	ticker := time.NewTicker(w.options.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_ = w.processAndReport(ctx)
		}
	}
}

func (w *ProjectionWorker) processAndReport(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, w.options.Timeout)
	defer cancel()
	err := w.Process(ctx)
	if err != nil && w.options.OnError != nil {
		w.options.OnError(err)
	}
	return err
}

// Process exports one durable batch. Values are recomputed from the complete
// event history and sent with PUT semantics, so repeating a batch cannot
// inflate a user's signal.
func (w *ProjectionWorker) Process(ctx context.Context) error {
	lease, locked, err := recommendationlock.TryAcquire(ctx, w.db)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer lease.Release()
	connection := lease.Connection()

	rows, err := connection.Query(ctx, `
		SELECT id::text, user_id::text, event_type, track_source, track_id
		FROM listening_events
		WHERE recommendation_projected_at IS NULL
		  AND recommendation_projection_available_at <= now()
		  AND track_source <> '' AND track_id <> ''
		ORDER BY created_at, id
		LIMIT $1
	`, w.options.BatchSize)
	if err != nil {
		return fmt.Errorf("read recommendation projection batch: %w", err)
	}
	pending := make([]pendingProjection, 0, w.options.BatchSize)
	keys := make(map[projectionKey]struct{})
	for rows.Next() {
		var value pendingProjection
		if err := rows.Scan(&value.ID, &value.Key.UserID, &value.Key.Type, &value.Key.Source, &value.Key.TrackID); err != nil {
			rows.Close()
			return fmt.Errorf("scan recommendation projection: %w", err)
		}
		pending = append(pending, value)
		keys[value.Key] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate recommendation projection: %w", err)
	}
	rows.Close()
	if len(pending) == 0 {
		return nil
	}

	keyList := make([]projectionKey, 0, len(keys))
	for key := range keys {
		keyList = append(keyList, key)
	}
	encodedKeys, err := json.Marshal(keyList)
	if err != nil {
		return fmt.Errorf("encode recommendation projection keys: %w", err)
	}
	aggregateRows, err := connection.Query(ctx, `
		WITH keys AS (
			SELECT user_id::uuid, event_type, track_source, track_id
			FROM jsonb_to_recordset($1::jsonb) AS value(
				user_id text, event_type text, track_source text, track_id text
			)
		)
		SELECT events.user_id::text, events.event_type, events.track_source,
		       events.track_id, count(*)::float8, max(events.occurred_at)
		FROM listening_events AS events
		JOIN keys USING (user_id, event_type, track_source, track_id)
		LEFT JOIN user_track_preferences AS preferences
		  ON preferences.user_id = events.user_id
		 AND preferences.track_source = events.track_source
		 AND preferences.track_id = events.track_id
		WHERE NOT (
			events.event_type IN ('like', 'dislike')
			AND preferences.user_id IS NOT NULL
		)
		GROUP BY events.user_id, events.event_type, events.track_source, events.track_id
	`, encodedKeys)
	if err != nil {
		w.retry(ctx, pending, err)
		return fmt.Errorf("aggregate recommendation feedback: %w", err)
	}
	aggregates := make([]FeedbackAggregate, 0, len(keys))
	for aggregateRows.Next() {
		var value FeedbackAggregate
		if err := aggregateRows.Scan(&value.UserID, &value.Type, &value.Source, &value.TrackID, &value.Count, &value.LatestAt); err != nil {
			aggregateRows.Close()
			w.retry(ctx, pending, err)
			return fmt.Errorf("scan recommendation aggregate: %w", err)
		}
		aggregates = append(aggregates, value)
	}
	if err := aggregateRows.Err(); err != nil {
		aggregateRows.Close()
		w.retry(ctx, pending, err)
		return fmt.Errorf("iterate recommendation aggregates: %w", err)
	}
	aggregateRows.Close()

	if err := w.projector.Project(ctx, aggregates); err != nil {
		w.retry(ctx, pending, err)
		return fmt.Errorf("project recommendation feedback: %w", err)
	}
	ids := projectionIDs(pending)
	if _, err := connection.Exec(ctx, `
		UPDATE listening_events
		SET recommendation_projected_at=now(), recommendation_projection_error=NULL
		WHERE id::text = ANY($1::text[])
	`, ids); err != nil {
		return fmt.Errorf("mark recommendation feedback projected: %w", err)
	}
	return nil
}

func (w *ProjectionWorker) retry(ctx context.Context, pending []pendingProjection, cause error) {
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, _ = w.db.Exec(ctx, `
		UPDATE listening_events
		SET recommendation_projection_attempts=recommendation_projection_attempts+1,
		    recommendation_projection_error=$2,
		    recommendation_projection_available_at=now() + interval '15 seconds'
		WHERE id::text = ANY($1::text[])
	`, projectionIDs(pending), message)
}

func projectionIDs(values []pendingProjection) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}
