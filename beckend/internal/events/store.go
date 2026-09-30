package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var allowedTypes = map[string]bool{
	"impression":      true,
	"play":            true,
	"listen_30s":      true,
	"complete":        true,
	"skip":            true,
	"repeat":          true,
	"seek":            true,
	"like":            true,
	"dislike":         true,
	"add_to_playlist": true,
	"search":          true,
}

type Event struct {
	Key        string          `json:"idempotency_key"`
	Type       string          `json:"type"`
	Source     string          `json:"track_source"`
	TrackID    string          `json:"track_id"`
	SessionID  string          `json:"session_id,omitempty"`
	PositionMS *int            `json:"position_ms,omitempty"`
	DurationMS *int            `json:"duration_ms,omitempty"`
	Context    json.RawMessage `json:"context,omitempty"`
	OccurredAt time.Time       `json:"occurred_at,omitempty"`
}

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func Validate(event Event) error {
	if strings.TrimSpace(event.Key) == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	if !allowedTypes[event.Type] {
		return fmt.Errorf("unsupported event type %q", event.Type)
	}
	if event.Type != "search" && (strings.TrimSpace(event.Source) == "" || strings.TrimSpace(event.TrackID) == "") {
		return fmt.Errorf("track_source and track_id are required")
	}
	if len(event.Key) > 120 || len(event.Source) > 40 || len(event.TrackID) > 240 || len(event.SessionID) > 120 {
		return fmt.Errorf("event field is too long")
	}
	if len(event.Context) > 16*1024 {
		return fmt.Errorf("event context is too large")
	}
	if len(event.Context) > 0 && !json.Valid(event.Context) {
		return fmt.Errorf("event context is not valid JSON")
	}
	return nil
}

func (s *Store) Add(ctx context.Context, userID string, input []Event) error {
	if len(input) == 0 || len(input) > 100 {
		return fmt.Errorf("events batch must contain 1 to 100 items")
	}
	batch := &pgx.Batch{}
	for _, event := range input {
		event.Key = strings.TrimSpace(event.Key)
		event.Type = strings.TrimSpace(event.Type)
		event.Source = strings.ToLower(strings.TrimSpace(event.Source))
		event.TrackID = music.CanonicalTrackID(event.Source, event.TrackID)
		if err := Validate(event); err != nil {
			return err
		}
		occurredAt := event.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		contextJSON := event.Context
		if len(contextJSON) == 0 {
			contextJSON = json.RawMessage(`{}`)
		}
		batch.Queue(`
			INSERT INTO listening_events(
				event_key, user_id, event_type, track_source, track_id,
				session_id, position_ms, duration_ms, context, occurred_at
			) VALUES (NULLIF($1,''),$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10)
			ON CONFLICT (user_id, event_key) WHERE event_key IS NOT NULL DO NOTHING
		`, event.Key, userID, event.Type, event.Source, event.TrackID,
			event.SessionID, event.PositionMS, event.DurationMS, []byte(contextJSON), occurredAt)
	}
	results := s.db.SendBatch(ctx, batch)
	defer results.Close()
	for range input {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("save listening event: %w", err)
		}
	}
	return nil
}
