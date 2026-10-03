package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/jackc/pgx/v5"
)

const (
	maxPlaylistName        = 120
	maxPlaylistDescription = 2000
	maxPlaylistTracks      = 500
)

var (
	playlistIDPattern              = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	ErrPlaylistNotFound            = errors.New("playlist was not found")
	ErrPlaylistIdempotencyConflict = errors.New("playlist idempotency key has already been used for a different mutation")
	ErrPlaylistStoreNoDatabase     = errors.New("playlist store requires a database")
)

// Playlist is an account-owned, ordered collection. Tracks remain compact
// snapshots because the music engine, rather than Mixora, owns full provider
// metadata and playback URLs.
type Playlist struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Tracks      []TrackSnapshot `json:"tracks"`
	Pinned      bool            `json:"pinned"`
	Liked       bool            `json:"liked"`
	Revision    int64           `json:"revision"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// PlaylistInput replaces the visible state of one account playlist. A full
// ordered list makes retrying an offline drag, add, or removal deterministic;
// it does not turn the old library JSON snapshot back into the authority.
type PlaylistInput struct {
	IdempotencyKey string        `json:"idempotency_key"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Tracks         []music.Track `json:"tracks"`
	Pinned         bool          `json:"pinned"`
	Liked          bool          `json:"liked"`
}

type PlaylistDeleteInput struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type PlaylistDeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func NormalizePlaylistDeleteInput(input PlaylistDeleteInput) (PlaylistDeleteInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if err := validatePreferenceText("idempotency key", input.IdempotencyKey, maxPreferenceIdempotencyKey, true); err != nil {
		return PlaylistDeleteInput{}, err
	}
	return input, nil
}

// NormalizePlaylistID restricts client-generated identifiers to canonical
// UUID text. IDs are client-generated only so an offline create can retain
// stable links and queued mutations before its first server round trip.
func NormalizePlaylistID(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !playlistIDPattern.MatchString(value) {
		return "", errors.New("playlist id must be a UUID")
	}
	return value, nil
}

// NormalizePlaylistInput validates user-owned metadata and reduces every
// track to the same provider-neutral snapshot used by preferences and history.
func NormalizePlaylistInput(input PlaylistInput) (PlaylistInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if err := validatePreferenceText("idempotency key", input.IdempotencyKey, maxPreferenceIdempotencyKey, true); err != nil {
		return PlaylistInput{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if err := validatePreferenceText("playlist name", input.Name, maxPlaylistName, true); err != nil {
		return PlaylistInput{}, err
	}
	input.Description = strings.TrimSpace(input.Description)
	if err := validatePreferenceText("playlist description", input.Description, maxPlaylistDescription, false); err != nil {
		return PlaylistInput{}, err
	}
	if len(input.Tracks) > maxPlaylistTracks {
		return PlaylistInput{}, fmt.Errorf("playlist may contain at most %d tracks", maxPlaylistTracks)
	}
	tracks := make([]music.Track, 0, len(input.Tracks))
	seen := make(map[string]struct{}, len(input.Tracks))
	for _, track := range input.Tracks {
		snapshot, err := NewTrackSnapshot(track)
		if err != nil {
			return PlaylistInput{}, err
		}
		key := snapshot.Source + ":" + snapshot.ID
		if _, exists := seen[key]; exists {
			return PlaylistInput{}, fmt.Errorf("playlist contains duplicate track %s", key)
		}
		seen[key] = struct{}{}
		tracks = append(tracks, snapshot.musicTrack())
	}
	input.Tracks = tracks
	return input, nil
}

// PlaylistRequestFingerprint excludes the idempotency key but includes the
// addressed playlist and complete desired order. Reusing a key for any other
// edit is rejected rather than silently changing a listener's playlist.
func PlaylistRequestFingerprint(playlistID string, input PlaylistInput) []byte {
	snapshots := playlistSnapshots(input.Tracks)
	body, _ := json.Marshal(struct {
		Operation   string          `json:"operation"`
		PlaylistID  string          `json:"playlist_id"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Tracks      []TrackSnapshot `json:"tracks"`
		Pinned      bool            `json:"pinned"`
		Liked       bool            `json:"liked"`
	}{
		Operation:   "replace",
		PlaylistID:  playlistID,
		Name:        input.Name,
		Description: input.Description,
		Tracks:      snapshots,
		Pinned:      input.Pinned,
		Liked:       input.Liked,
	})
	sum := sha256.Sum256(body)
	return append([]byte(nil), sum[:]...)
}

func PlaylistDeleteRequestFingerprint(playlistID string) []byte {
	body, _ := json.Marshal(struct {
		Operation  string `json:"operation"`
		PlaylistID string `json:"playlist_id"`
	}{Operation: "delete", PlaylistID: playlistID})
	sum := sha256.Sum256(body)
	return append([]byte(nil), sum[:]...)
}

// ReplacePlaylist atomically creates or replaces an account playlist and its
// complete order. The narrow mutation is intentionally idempotent so a client
// can safely retry an offline queue entry without duplicating tracks.
func (s *Store) ReplacePlaylist(ctx context.Context, userID, playlistID string, input PlaylistInput) (Playlist, error) {
	if s == nil || s.db == nil {
		return Playlist{}, ErrPlaylistStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return Playlist{}, err
	}
	playlistID, err := NormalizePlaylistID(playlistID)
	if err != nil {
		return Playlist{}, err
	}
	normalized, err := NormalizePlaylistInput(input)
	if err != nil {
		return Playlist{}, err
	}
	snapshots := playlistSnapshots(normalized.Tracks)
	requestHash := PlaylistRequestFingerprint(playlistID, normalized)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Playlist{}, fmt.Errorf("begin playlist replacement: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, lockKey := range []string{
		playlistLockKey("idempotency", userID, normalized.IdempotencyKey),
		playlistLockKey("playlist", userID, playlistID),
	} {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
			return Playlist{}, fmt.Errorf("lock playlist mutation: %w", err)
		}
	}

	if replay, found, err := findPlaylistReceipt(ctx, tx, userID, normalized.IdempotencyKey, requestHash); err != nil {
		return Playlist{}, err
	} else if found {
		var playlist Playlist
		if err := json.Unmarshal(replay, &playlist); err != nil {
			return Playlist{}, fmt.Errorf("decode replayed playlist: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return Playlist{}, fmt.Errorf("commit playlist replay: %w", err)
		}
		return playlist, nil
	}

	current, found, err := loadPlaylist(ctx, tx, userID, playlistID, true)
	if err != nil {
		return Playlist{}, err
	}
	var result Playlist
	if !found {
		result, err = insertPlaylist(ctx, tx, userID, playlistID, normalized, snapshots)
	} else if playlistContentEqual(current, normalized, snapshots) {
		result = current
	} else {
		result, err = replacePlaylistContent(ctx, tx, current, normalized, snapshots)
	}
	if err != nil {
		return Playlist{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return Playlist{}, fmt.Errorf("encode playlist receipt: %w", err)
	}
	if err := savePlaylistReceipt(ctx, tx, userID, normalized.IdempotencyKey, requestHash, "replace", playlistID, encoded); err != nil {
		return Playlist{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Playlist{}, fmt.Errorf("commit playlist replacement: %w", err)
	}
	return result, nil
}

// DeletePlaylist removes one account playlist. Its receipt deliberately stays
// after the row is gone, so a timed-out delete can be retried safely.
func (s *Store) DeletePlaylist(ctx context.Context, userID, playlistID string, input PlaylistDeleteInput) (PlaylistDeleteResult, error) {
	if s == nil || s.db == nil {
		return PlaylistDeleteResult{}, ErrPlaylistStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return PlaylistDeleteResult{}, err
	}
	playlistID, err := NormalizePlaylistID(playlistID)
	if err != nil {
		return PlaylistDeleteResult{}, err
	}
	input, err = NormalizePlaylistDeleteInput(input)
	if err != nil {
		return PlaylistDeleteResult{}, err
	}
	requestHash := PlaylistDeleteRequestFingerprint(playlistID)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PlaylistDeleteResult{}, fmt.Errorf("begin playlist deletion: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, lockKey := range []string{
		playlistLockKey("idempotency", userID, input.IdempotencyKey),
		playlistLockKey("playlist", userID, playlistID),
	} {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
			return PlaylistDeleteResult{}, fmt.Errorf("lock playlist deletion: %w", err)
		}
	}
	if replay, found, err := findPlaylistReceipt(ctx, tx, userID, input.IdempotencyKey, requestHash); err != nil {
		return PlaylistDeleteResult{}, err
	} else if found {
		var result PlaylistDeleteResult
		if err := json.Unmarshal(replay, &result); err != nil {
			return PlaylistDeleteResult{}, fmt.Errorf("decode replayed playlist delete: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return PlaylistDeleteResult{}, fmt.Errorf("commit playlist delete replay: %w", err)
		}
		return result, nil
	}

	if _, err := tx.Exec(ctx, `DELETE FROM user_playlists WHERE id=$1 AND user_id=$2`, playlistID, userID); err != nil {
		return PlaylistDeleteResult{}, fmt.Errorf("delete playlist: %w", err)
	}
	result := PlaylistDeleteResult{ID: playlistID, Deleted: true}
	encoded, err := json.Marshal(result)
	if err != nil {
		return PlaylistDeleteResult{}, fmt.Errorf("encode playlist delete receipt: %w", err)
	}
	if err := savePlaylistReceipt(ctx, tx, userID, input.IdempotencyKey, requestHash, "delete", playlistID, encoded); err != nil {
		return PlaylistDeleteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PlaylistDeleteResult{}, fmt.Errorf("commit playlist deletion: %w", err)
	}
	return result, nil
}

func (s *Store) ListPlaylists(ctx context.Context, userID string) ([]Playlist, error) {
	if s == nil || s.db == nil {
		return nil, ErrPlaylistStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text
		FROM user_playlists
		WHERE user_id=$1
		ORDER BY updated_at DESC, id
		LIMIT 50
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list playlists: %w", err)
	}
	defer rows.Close()
	playlists := make([]Playlist, 0)
	for rows.Next() {
		var playlistID string
		if err := rows.Scan(&playlistID); err != nil {
			return nil, fmt.Errorf("scan playlist id: %w", err)
		}
		playlist, found, err := loadPlaylist(ctx, s.db, userID, playlistID, false)
		if err != nil {
			return nil, err
		}
		if found {
			playlists = append(playlists, playlist)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate playlists: %w", err)
	}
	return playlists, nil
}

func (s *Store) GetPlaylist(ctx context.Context, userID, playlistID string) (Playlist, error) {
	if s == nil || s.db == nil {
		return Playlist{}, ErrPlaylistStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return Playlist{}, err
	}
	playlistID, err := NormalizePlaylistID(playlistID)
	if err != nil {
		return Playlist{}, err
	}
	playlist, found, err := loadPlaylist(ctx, s.db, userID, playlistID, false)
	if err != nil {
		return Playlist{}, err
	}
	if !found {
		return Playlist{}, ErrPlaylistNotFound
	}
	return playlist, nil
}

type playlistQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadPlaylist(ctx context.Context, q playlistQueryer, userID, playlistID string, lock bool) (Playlist, bool, error) {
	query := `
		SELECT id::text, name, description, pinned, liked, revision, created_at, updated_at
		FROM user_playlists
		WHERE id=$1 AND user_id=$2
	`
	if lock {
		query += " FOR UPDATE"
	}
	var playlist Playlist
	err := q.QueryRow(ctx, query, playlistID, userID).Scan(
		&playlist.ID, &playlist.Name, &playlist.Description, &playlist.Pinned,
		&playlist.Liked, &playlist.Revision, &playlist.CreatedAt, &playlist.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Playlist{}, false, nil
	}
	if err != nil {
		return Playlist{}, false, fmt.Errorf("load playlist: %w", err)
	}
	rows, err := q.Query(ctx, `
		SELECT track_snapshot
		FROM user_playlist_tracks
		WHERE playlist_id=$1
		ORDER BY position
	`, playlist.ID)
	if err != nil {
		return Playlist{}, false, fmt.Errorf("list playlist tracks: %w", err)
	}
	defer rows.Close()
	playlist.Tracks = make([]TrackSnapshot, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return Playlist{}, false, fmt.Errorf("scan playlist track: %w", err)
		}
		snapshot, err := decodeTrackSnapshot(raw)
		if err != nil {
			return Playlist{}, false, fmt.Errorf("decode playlist track: %w", err)
		}
		playlist.Tracks = append(playlist.Tracks, snapshot)
	}
	if err := rows.Err(); err != nil {
		return Playlist{}, false, fmt.Errorf("iterate playlist tracks: %w", err)
	}
	return playlist, true, nil
}

func insertPlaylist(ctx context.Context, tx pgx.Tx, userID, playlistID string, input PlaylistInput, snapshots []TrackSnapshot) (Playlist, error) {
	var playlist Playlist
	err := tx.QueryRow(ctx, `
		INSERT INTO user_playlists(id, user_id, name, description, pinned, liked)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id::text, revision, created_at, updated_at
	`, playlistID, userID, input.Name, input.Description, input.Pinned, input.Liked).Scan(
		&playlist.ID, &playlist.Revision, &playlist.CreatedAt, &playlist.UpdatedAt,
	)
	if err != nil {
		return Playlist{}, fmt.Errorf("create playlist: %w", err)
	}
	playlist.Name = input.Name
	playlist.Description = input.Description
	playlist.Pinned = input.Pinned
	playlist.Liked = input.Liked
	playlist.Tracks = append([]TrackSnapshot(nil), snapshots...)
	if err := insertPlaylistTracks(ctx, tx, playlist.ID, snapshots); err != nil {
		return Playlist{}, err
	}
	return playlist, nil
}

func replacePlaylistContent(ctx context.Context, tx pgx.Tx, current Playlist, input PlaylistInput, snapshots []TrackSnapshot) (Playlist, error) {
	var playlist Playlist
	err := tx.QueryRow(ctx, `
		UPDATE user_playlists
		SET name=$2,
			description=$3,
			pinned=$4,
			liked=$5,
			revision=revision + 1,
			updated_at=now()
		WHERE id=$1
		RETURNING id::text, revision, created_at, updated_at
	`, current.ID, input.Name, input.Description, input.Pinned, input.Liked).Scan(
		&playlist.ID, &playlist.Revision, &playlist.CreatedAt, &playlist.UpdatedAt,
	)
	if err != nil {
		return Playlist{}, fmt.Errorf("update playlist: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_playlist_tracks WHERE playlist_id=$1`, current.ID); err != nil {
		return Playlist{}, fmt.Errorf("replace playlist tracks: %w", err)
	}
	if err := insertPlaylistTracks(ctx, tx, current.ID, snapshots); err != nil {
		return Playlist{}, err
	}
	playlist.Name = input.Name
	playlist.Description = input.Description
	playlist.Pinned = input.Pinned
	playlist.Liked = input.Liked
	playlist.Tracks = append([]TrackSnapshot(nil), snapshots...)
	return playlist, nil
}

func insertPlaylistTracks(ctx context.Context, tx pgx.Tx, playlistID string, snapshots []TrackSnapshot) error {
	for index, snapshot := range snapshots {
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("encode playlist track: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_playlist_tracks(
				playlist_id, track_source, track_id, track_snapshot, position
			) VALUES ($1,$2,$3,$4,$5)
		`, playlistID, snapshot.Source, snapshot.ID, raw, index+1); err != nil {
			return fmt.Errorf("insert playlist track: %w", err)
		}
	}
	return nil
}

func playlistContentEqual(current Playlist, input PlaylistInput, snapshots []TrackSnapshot) bool {
	if current.Name != input.Name || current.Description != input.Description || current.Pinned != input.Pinned || current.Liked != input.Liked || len(current.Tracks) != len(snapshots) {
		return false
	}
	for index := range snapshots {
		if current.Tracks[index] != snapshots[index] {
			return false
		}
	}
	return true
}

func playlistSnapshots(tracks []music.Track) []TrackSnapshot {
	snapshots := make([]TrackSnapshot, 0, len(tracks))
	for _, track := range tracks {
		snapshot, err := NewTrackSnapshot(track)
		if err != nil {
			// Callers use this only after NormalizePlaylistInput. Returning an
			// empty snapshot would make corruption obvious in the subsequent
			// validation or database constraint rather than silently broadening
			// the accepted playlist contract.
			return nil
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func findPlaylistReceipt(ctx context.Context, tx pgx.Tx, userID, key string, requestHash []byte) ([]byte, bool, error) {
	var storedHash, result []byte
	err := tx.QueryRow(ctx, `
		SELECT request_hash, result
		FROM user_playlist_idempotency
		WHERE user_id=$1 AND idempotency_key=$2
	`, userID, key).Scan(&storedHash, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read playlist idempotency receipt: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash) {
		return nil, false, ErrPlaylistIdempotencyConflict
	}
	return result, true, nil
}

func savePlaylistReceipt(ctx context.Context, tx pgx.Tx, userID, key string, requestHash []byte, operation, playlistID string, result []byte) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_playlist_idempotency(
			user_id, idempotency_key, request_hash, operation, playlist_id, result
		) VALUES ($1,$2,$3,$4,$5,$6)
	`, userID, key, requestHash, operation, playlistID, result); err != nil {
		return fmt.Errorf("save playlist idempotency receipt: %w", err)
	}
	return nil
}

func playlistLockKey(scope string, values ...string) string {
	var builder strings.Builder
	builder.WriteString("mixora:playlist:")
	builder.WriteString(scope)
	for _, value := range values {
		fmt.Fprintf(&builder, ":%d:%s", len(value), value)
	}
	return builder.String()
}
