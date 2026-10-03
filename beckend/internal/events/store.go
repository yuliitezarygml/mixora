package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

var waveSessionTypes = map[string]bool{
	"play": true, "listen_30s": true, "complete": true, "skip": true,
	"repeat": true, "seek": true, "like": true, "dislike": true,
	"add_to_playlist": true,
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
	db transactionStarter
}

type transactionStarter interface {
	Begin(context.Context) (pgx.Tx, error)
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

// IsWaveSessionEventType limits a Wave session to feedback about one of its
// delivered tracks. Search and impression events are recorded by their own
// server-side flows and must never be client-injected into a Wave session.
func IsWaveSessionEventType(eventType string) bool {
	return waveSessionTypes[strings.TrimSpace(eventType)]
}

func deduplicatesWaveEvent(event Event) bool {
	return strings.TrimSpace(event.SessionID) != "" && IsWaveSessionEventType(event.Type)
}

func (s *Store) Add(ctx context.Context, userID string, input []Event) error {
	if len(input) == 0 || len(input) > 100 {
		return fmt.Errorf("events batch must contain 1 to 100 items")
	}
	normalized := make([]Event, 0, len(input))
	for _, event := range input {
		event.Key = strings.TrimSpace(event.Key)
		event.Type = strings.TrimSpace(event.Type)
		event.Source = strings.ToLower(strings.TrimSpace(event.Source))
		event.TrackID = music.CanonicalTrackID(event.Source, event.TrackID)
		event.SessionID = strings.TrimSpace(event.SessionID)
		if err := Validate(event); err != nil {
			return err
		}
		normalized = append(normalized, event)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin listening event save: %w", err)
	}
	defer tx.Rollback(ctx)

	// The journal key and the Wave receipt are distinct uniqueness domains. Lock
	// both in one deterministic order so a duplicate event key cannot create a
	// receipt for an event that ultimately loses the journal conflict.
	for _, lockKey := range eventWriteLockKeys(userID, normalized) {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
			return fmt.Errorf("lock listening event write: %w", err)
		}
	}

	batch := &pgx.Batch{}
	for _, event := range normalized {
		occurredAt := event.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		contextJSON := event.Context
		if len(contextJSON) == 0 {
			contextJSON = json.RawMessage(`{}`)
		}
		batch.Queue(`
			WITH existing_event AS (
				SELECT 1
				FROM listening_events
				WHERE user_id=$2 AND event_key=NULLIF($1,'')
			),
			existing_receipt AS (
				SELECT 1
				FROM wave_event_receipts
				WHERE user_id=$2 AND session_id=$6
				  AND event_type=$3 AND track_source=$4 AND track_id=$5
			),
			receipt AS (
				INSERT INTO wave_event_receipts(
					user_id, session_id, event_type, track_source, track_id
				)
				SELECT $2, $6, $3, $4, $5
				WHERE $11::boolean
				  AND NOT EXISTS (SELECT 1 FROM existing_event)
				  AND NOT EXISTS (SELECT 1 FROM existing_receipt)
				RETURNING 1
			)
			INSERT INTO listening_events(
				event_key, user_id, event_type, track_source, track_id,
				session_id, position_ms, duration_ms, context, occurred_at
			)
			SELECT NULLIF($1,''),$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10
			WHERE NOT EXISTS (SELECT 1 FROM existing_event)
			  AND (NOT $11::boolean OR EXISTS (SELECT 1 FROM receipt))
		`, event.Key, userID, event.Type, event.Source, event.TrackID,
			event.SessionID, event.PositionMS, event.DurationMS, []byte(contextJSON), occurredAt,
			deduplicatesWaveEvent(event))
	}
	results := tx.SendBatch(ctx, batch)
	for range normalized {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("save listening event: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("close listening event batch: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit listening event save: %w", err)
	}
	return nil
}

func eventWriteLockKeys(userID string, input []Event) []string {
	seen := make(map[string]struct{}, len(input)*2)
	for _, event := range input {
		seen[scopedEventLockKey("idempotency", userID, event.Key)] = struct{}{}
		if deduplicatesWaveEvent(event) {
			seen[scopedEventLockKey("wave-receipt", userID, event.SessionID, event.Type, event.Source, event.TrackID)] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// scopedEventLockKey uses length-prefixed fields to prevent ambiguous tuples
// such as ("ab", "c") and ("a", "bc") from sharing a lock by construction.
func scopedEventLockKey(scope string, values ...string) string {
	var builder strings.Builder
	builder.WriteString(scope)
	for _, value := range values {
		fmt.Fprintf(&builder, ":%d:%s", len(value), value)
	}
	return builder.String()
}
