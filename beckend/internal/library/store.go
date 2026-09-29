package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Empty = json.RawMessage(`{"likes":[],"dislikes":[],"history":[],"playlists":[],"artists":[],"searches":[],"savedPlaylists":[],"albums":[],"episodes":[],"listens":[],"pins":[]}`)

type Snapshot struct {
	Payload json.RawMessage
	Version int64
}

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Get(ctx context.Context, userID string) (Snapshot, error) {
	var payload []byte
	var version int64
	err := s.db.QueryRow(ctx, `
		SELECT payload, version FROM user_libraries WHERE user_id=$1
	`, userID).Scan(&payload, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{Payload: append(json.RawMessage(nil), Empty...), Version: 0}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("get library: %w", err)
	}
	return Snapshot{Payload: json.RawMessage(payload), Version: version}, nil
}

func (s *Store) Put(ctx context.Context, userID string, payload json.RawMessage) (Snapshot, error) {
	if !json.Valid(payload) {
		return Snapshot{}, fmt.Errorf("library payload is not valid JSON")
	}
	var saved []byte
	var version int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO user_libraries(user_id, payload, version)
		VALUES ($1, $2, 1)
		ON CONFLICT (user_id) DO UPDATE
		SET payload=EXCLUDED.payload,
		    version=user_libraries.version + 1,
		    updated_at=now()
		RETURNING payload, version
	`, userID, []byte(payload)).Scan(&saved, &version)
	if err != nil {
		return Snapshot{}, fmt.Errorf("save library: %w", err)
	}
	return Snapshot{Payload: json.RawMessage(saved), Version: version}, nil
}
