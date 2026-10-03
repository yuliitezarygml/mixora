package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/jackc/pgx/v5"
)

var (
	ErrHistoryIdempotencyKeyConflict = errors.New("history idempotency key has already been used for a different record")
	ErrHistoryStoreNoDatabase        = errors.New("track history store requires a database")
)

// HistoryInput records one meaningful listen. It is intentionally separate
// from events.Event: the event journal retains recommendation signals while
// this input retains only the compact data needed to render the user's recent
// listening history.
type HistoryInput struct {
	IdempotencyKey string      `json:"idempotency_key"`
	Track          music.Track `json:"track"`
	OccurredAt     time.Time   `json:"occurred_at"`
}

type HistoryEntry struct {
	Track           TrackSnapshot `json:"track"`
	FirstListenedAt time.Time     `json:"first_listened_at"`
	LastListenedAt  time.Time     `json:"last_listened_at"`
	PlayCount       int           `json:"play_count"`
}

// NormalizeHistoryInput validates a client record once, canonicalizes legacy
// SoundCloud references, and strips unneeded provider data from the snapshot.
func NormalizeHistoryInput(input HistoryInput) (HistoryInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if err := validatePreferenceText("idempotency key", input.IdempotencyKey, maxPreferenceIdempotencyKey, true); err != nil {
		return HistoryInput{}, err
	}
	if input.OccurredAt.IsZero() {
		return HistoryInput{}, errors.New("occurred_at is required")
	}
	if input.OccurredAt.Year() < 2000 || input.OccurredAt.After(time.Now().Add(24*time.Hour)) {
		return HistoryInput{}, errors.New("occurred_at is outside the accepted range")
	}
	snapshot, err := NewTrackSnapshot(input.Track)
	if err != nil {
		return HistoryInput{}, err
	}
	input.Track = snapshot.musicTrack()
	input.OccurredAt = input.OccurredAt.UTC()
	return input, nil
}

// HistoryRequestFingerprint excludes the idempotency key but includes the
// exact occurrence instant. Reusing a key for a different event is rejected
// instead of silently changing the listener's history.
func HistoryRequestFingerprint(input HistoryInput) []byte {
	snapshot, _ := NewTrackSnapshot(input.Track)
	body, _ := json.Marshal(struct {
		Track      TrackSnapshot `json:"track"`
		OccurredAt time.Time     `json:"occurred_at"`
	}{Track: snapshot, OccurredAt: input.OccurredAt.UTC()})
	sum := sha256.Sum256(body)
	return append([]byte(nil), sum[:]...)
}

// RecordHistory atomically updates one current history row and an idempotency
// receipt. The per-user lock serializes a clear operation with an in-flight
// listen, while the per-track lock protects the absent-row case and the UPSERT
// keeps play counters correct.
func (s *Store) RecordHistory(ctx context.Context, userID string, input HistoryInput) (HistoryEntry, error) {
	if s == nil || s.db == nil {
		return HistoryEntry{}, ErrHistoryStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return HistoryEntry{}, err
	}
	normalized, err := NormalizeHistoryInput(input)
	if err != nil {
		return HistoryEntry{}, err
	}
	snapshot, err := NewTrackSnapshot(normalized.Track)
	if err != nil {
		return HistoryEntry{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return HistoryEntry{}, fmt.Errorf("encode track history snapshot: %w", err)
	}
	requestHash := HistoryRequestFingerprint(normalized)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return HistoryEntry{}, fmt.Errorf("begin track history save: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, lockKey := range []string{
		historyLockKey("user", userID),
		historyLockKey("idempotency", userID, normalized.IdempotencyKey),
		historyLockKey("track", userID, snapshot.Source, snapshot.ID),
	} {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
			return HistoryEntry{}, fmt.Errorf("lock track history: %w", err)
		}
	}

	if replay, found, err := findHistoryReplay(ctx, tx, userID, normalized.IdempotencyKey, requestHash); err != nil {
		return HistoryEntry{}, err
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return HistoryEntry{}, fmt.Errorf("commit track history replay: %w", err)
		}
		return replay, nil
	}

	entry, persistedSnapshotJSON, err := upsertHistoryEntry(ctx, tx, userID, snapshot, snapshotJSON, normalized.OccurredAt)
	if err != nil {
		return HistoryEntry{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_track_history_idempotency(
			user_id, idempotency_key, request_hash, track_source, track_id,
			track_snapshot, first_listened_at, last_listened_at, play_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, userID, normalized.IdempotencyKey, requestHash, snapshot.Source, snapshot.ID,
		persistedSnapshotJSON, entry.FirstListenedAt, entry.LastListenedAt, entry.PlayCount); err != nil {
		return HistoryEntry{}, fmt.Errorf("save track history idempotency receipt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return HistoryEntry{}, fmt.Errorf("commit track history save: %w", err)
	}
	return entry, nil
}

// ClearHistory removes the account's normalized history and its idempotency
// receipts. The shared per-user advisory lock gives a clear a deterministic
// ordering relative to a listen arriving from another connected device.
func (s *Store) ClearHistory(ctx context.Context, userID string) error {
	if s == nil || s.db == nil {
		return ErrHistoryStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin track history clear: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, historyLockKey("user", userID)); err != nil {
		return fmt.Errorf("lock track history clear: %w", err)
	}
	// user_track_history_idempotency has an ON DELETE CASCADE foreign key, so
	// its replay receipts disappear with their referenced history rows.
	if _, err := tx.Exec(ctx, `DELETE FROM user_track_history WHERE user_id=$1`, userID); err != nil {
		return fmt.Errorf("clear track history: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit track history clear: %w", err)
	}
	return nil
}

func findHistoryReplay(ctx context.Context, tx pgx.Tx, userID, key string, requestHash []byte) (HistoryEntry, bool, error) {
	var storedHash, snapshotJSON []byte
	var result HistoryEntry
	err := tx.QueryRow(ctx, `
		SELECT request_hash, track_snapshot, first_listened_at, last_listened_at, play_count
		FROM user_track_history_idempotency
		WHERE user_id=$1 AND idempotency_key=$2
	`, userID, key).Scan(&storedHash, &snapshotJSON, &result.FirstListenedAt, &result.LastListenedAt, &result.PlayCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return HistoryEntry{}, false, nil
	}
	if err != nil {
		return HistoryEntry{}, false, fmt.Errorf("read track history idempotency receipt: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash) {
		return HistoryEntry{}, false, ErrHistoryIdempotencyKeyConflict
	}
	snapshot, err := decodeTrackSnapshot(snapshotJSON)
	if err != nil {
		return HistoryEntry{}, false, fmt.Errorf("decode replayed track history: %w", err)
	}
	result.Track = snapshot
	return result, true, nil
}

func upsertHistoryEntry(ctx context.Context, tx pgx.Tx, userID string, snapshot TrackSnapshot, snapshotJSON []byte, occurredAt time.Time) (HistoryEntry, []byte, error) {
	var entry HistoryEntry
	var rawSnapshot []byte
	err := tx.QueryRow(ctx, `
		INSERT INTO user_track_history(
			user_id, track_source, track_id, track_snapshot,
			first_listened_at, last_listened_at, play_count
		) VALUES ($1,$2,$3,$4,$5,$5,1)
		ON CONFLICT (user_id, track_source, track_id) DO UPDATE
		SET track_snapshot=CASE
				WHEN EXCLUDED.last_listened_at >= user_track_history.last_listened_at
					THEN EXCLUDED.track_snapshot
				ELSE user_track_history.track_snapshot END,
			first_listened_at=LEAST(user_track_history.first_listened_at, EXCLUDED.first_listened_at),
			last_listened_at=GREATEST(user_track_history.last_listened_at, EXCLUDED.last_listened_at),
			play_count=user_track_history.play_count + 1,
			updated_at=now()
		RETURNING track_snapshot, first_listened_at, last_listened_at, play_count
	`, userID, snapshot.Source, snapshot.ID, snapshotJSON, occurredAt).Scan(
		&rawSnapshot, &entry.FirstListenedAt, &entry.LastListenedAt, &entry.PlayCount,
	)
	if err != nil {
		return HistoryEntry{}, nil, fmt.Errorf("upsert track history: %w", err)
	}
	decoded, err := decodeTrackSnapshot(rawSnapshot)
	if err != nil {
		return HistoryEntry{}, nil, fmt.Errorf("decode saved track history: %w", err)
	}
	entry.Track = decoded
	// The idempotency receipt must preserve the response snapshot, which can
	// differ from the incoming one for an out-of-order listen. For example, an
	// older event must retain the metadata of a newer current history row.
	return entry, rawSnapshot, nil
}

// ListHistory returns the most recently listened distinct tracks. `limit` is
// deliberately capped here as well as in HTTP so direct callers cannot turn a
// collection view into an unbounded database read.
func (s *Store) ListHistory(ctx context.Context, userID string, limit int) ([]HistoryEntry, error) {
	if s == nil || s.db == nil {
		return nil, ErrHistoryStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []HistoryEntry{}, nil
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT track_snapshot, first_listened_at, last_listened_at, play_count
		FROM user_track_history
		WHERE user_id=$1
		ORDER BY last_listened_at DESC, track_source, track_id
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list track history: %w", err)
	}
	defer rows.Close()

	result := make([]HistoryEntry, 0, limit)
	for rows.Next() {
		var entry HistoryEntry
		var rawSnapshot []byte
		if err := rows.Scan(&rawSnapshot, &entry.FirstListenedAt, &entry.LastListenedAt, &entry.PlayCount); err != nil {
			return nil, fmt.Errorf("scan track history: %w", err)
		}
		snapshot, err := decodeTrackSnapshot(rawSnapshot)
		if err != nil {
			return nil, fmt.Errorf("decode track history: %w", err)
		}
		entry.Track = snapshot
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate track history: %w", err)
	}
	return result, nil
}

// RecommendationHistory exposes the same compact, server-owned recent tracks
// to Wave. It intentionally does not expose counts or timestamps to the
// ranking service: their role here is a bounded set of familiar taste seeds.
func (s *Store) RecommendationHistory(ctx context.Context, userID string, limit int) ([]music.Track, error) {
	entries, err := s.ListHistory(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	tracks := make([]music.Track, 0, len(entries))
	for _, entry := range entries {
		tracks = append(tracks, entry.Track.musicTrack())
	}
	return tracks, nil
}

func historyLockKey(scope string, values ...string) string {
	var builder strings.Builder
	builder.WriteString("mixora:track-history:")
	builder.WriteString(scope)
	for _, value := range values {
		fmt.Fprintf(&builder, ":%d:%s", len(value), value)
	}
	return builder.String()
}
