package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/jackc/pgx/v5"
)

const (
	maxPreferenceIdempotencyKey = 120
	maxPreferenceSource         = 40
	maxPreferenceTrackID        = 240
	maxPreferenceTitle          = 500
	maxPreferenceArtist         = 500
	maxPreferenceURL            = 2048
	maxPreferenceAccess         = 40
)

var (
	providerSourcePattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	ErrIdempotencyKeyConflict   = errors.New("idempotency key has already been used for a different track preference")
	ErrPreferenceStoreNoDatabase = errors.New("track preference store requires a database")
)

// Preference is the user's current desired relationship with a track. Neutral
// is kept explicitly to let asynchronous consumers retract a prior like or
// dislike instead of inferring state from an event log.
type Preference string

const (
	PreferenceLiked    Preference = "liked"
	PreferenceDisliked Preference = "disliked"
	PreferenceNeutral  Preference = "neutral"
)

// TrackSnapshot is the deliberately small, provider-verified representation
// retained with a preference. It is not hydrated from track_catalog: callers
// must pass a music.Track already resolved by a trusted server-side path.
type TrackSnapshot struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	Title     string  `json:"title"`
	Artist    string  `json:"artist"`
	ArtistID  string  `json:"artistId,omitempty"`
	Artwork   string  `json:"artwork,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	Explicit  bool    `json:"explicit"`
	Access    string  `json:"access,omitempty"`
	Permalink string  `json:"permalink,omitempty"`
}

func (s TrackSnapshot) musicTrack() music.Track {
	return music.Track{
		ID:        s.ID,
		Source:    s.Source,
		Title:     s.Title,
		Artist:    s.Artist,
		ArtistID:  s.ArtistID,
		Artwork:   s.Artwork,
		Duration:  s.Duration,
		Explicit:  s.Explicit,
		Access:    s.Access,
		Permalink: s.Permalink,
	}
}

// PreferenceInput is one idempotent desired-state write. Track must have been
// verified by the provider/search or recommendation path before it reaches the
// store; this package intentionally has no dependency on a global catalog.
type PreferenceInput struct {
	IdempotencyKey string      `json:"idempotency_key"`
	Preference     Preference  `json:"preference"`
	Track          music.Track `json:"track"`
}

// TrackPreference is a current preference or a replayed result. Revision grows
// only when the desired state or compact snapshot changes.
type TrackPreference struct {
	Track      TrackSnapshot `json:"track"`
	Preference Preference    `json:"preference"`
	Revision   int64         `json:"revision"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// NormalizePreferenceInput validates an input and strips the supplied track to
// the compact snapshot fields. Its output is safe to fingerprint and persist.
func NormalizePreferenceInput(input PreferenceInput) (PreferenceInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if err := validatePreferenceText("idempotency key", input.IdempotencyKey, maxPreferenceIdempotencyKey, true); err != nil {
		return PreferenceInput{}, err
	}

	preference, err := normalizePreference(input.Preference)
	if err != nil {
		return PreferenceInput{}, err
	}
	snapshot, err := NewTrackSnapshot(input.Track)
	if err != nil {
		return PreferenceInput{}, err
	}

	input.Preference = preference
	input.Track = snapshot.musicTrack()
	return input, nil
}

// NewTrackSnapshot canonicalizes a provider track reference and extracts only
// data required to render or asynchronously project a preference.
func NewTrackSnapshot(track music.Track) (TrackSnapshot, error) {
	track.Source = strings.ToLower(strings.TrimSpace(track.Source))
	track.ID = strings.TrimSpace(track.ID)
	track.ArtistID = strings.TrimSpace(track.ArtistID)
	track = music.CanonicalTrack(track)

	snapshot := TrackSnapshot{
		ID:        strings.TrimSpace(track.ID),
		Source:    strings.ToLower(strings.TrimSpace(track.Source)),
		Title:     strings.TrimSpace(track.Title),
		Artist:    strings.TrimSpace(track.Artist),
		ArtistID:  strings.TrimSpace(track.ArtistID),
		Artwork:   strings.TrimSpace(track.Artwork),
		Duration:  track.Duration,
		Explicit:  track.Explicit,
		Access:    strings.ToLower(strings.TrimSpace(track.Access)),
		Permalink: strings.TrimSpace(track.Permalink),
	}

	if err := validatePreferenceText("track source", snapshot.Source, maxPreferenceSource, true); err != nil {
		return TrackSnapshot{}, err
	}
	if !providerSourcePattern.MatchString(snapshot.Source) {
		return TrackSnapshot{}, fmt.Errorf("track source contains unsupported characters")
	}
	if err := validatePreferenceText("track id", snapshot.ID, maxPreferenceTrackID, true); err != nil {
		return TrackSnapshot{}, err
	}
	if err := validatePreferenceText("track title", snapshot.Title, maxPreferenceTitle, true); err != nil {
		return TrackSnapshot{}, err
	}
	if err := validatePreferenceText("track artist", snapshot.Artist, maxPreferenceArtist, true); err != nil {
		return TrackSnapshot{}, err
	}
	if err := validatePreferenceText("track artist id", snapshot.ArtistID, maxPreferenceTrackID, false); err != nil {
		return TrackSnapshot{}, err
	}
	if err := validatePreferenceText("track artwork", snapshot.Artwork, maxPreferenceURL, false); err != nil {
		return TrackSnapshot{}, err
	}
	if err := validatePreferenceText("track permalink", snapshot.Permalink, maxPreferenceURL, false); err != nil {
		return TrackSnapshot{}, err
	}
	if err := validatePreferenceText("track access", snapshot.Access, maxPreferenceAccess, false); err != nil {
		return TrackSnapshot{}, err
	}
	if math.IsNaN(snapshot.Duration) || math.IsInf(snapshot.Duration, 0) || snapshot.Duration < 0 {
		return TrackSnapshot{}, fmt.Errorf("track duration must be a finite non-negative number")
	}
	return snapshot, nil
}

func normalizePreference(preference Preference) (Preference, error) {
	preference = Preference(strings.ToLower(strings.TrimSpace(string(preference))))
	switch preference {
	case PreferenceLiked, PreferenceDisliked, PreferenceNeutral:
		return preference, nil
	default:
		return "", fmt.Errorf("unsupported track preference %q", preference)
	}
}

func validatePreferenceText(field, value string, max int, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(value) > max {
		return fmt.Errorf("%s is too long", field)
	}
	if value != "" && (!utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0) {
		return fmt.Errorf("%s contains invalid characters", field)
	}
	return nil
}

// PreferenceRequestFingerprint identifies the desired state and compact track,
// but deliberately excludes its idempotency key. Call it with a normalized
// input; SetTrackPreference always does so before storing a request record.
func PreferenceRequestFingerprint(input PreferenceInput) []byte {
	snapshot, _ := NewTrackSnapshot(input.Track)
	body, _ := json.Marshal(struct {
		Preference Preference   `json:"preference"`
		Track      TrackSnapshot `json:"track"`
	}{Preference: input.Preference, Track: snapshot})
	sum := sha256.Sum256(body)
	return append([]byte(nil), sum[:]...)
}

// SetTrackPreference atomically stores one desired state, its compact snapshot,
// an idempotency receipt, and a coalesced durable outbox record.
func (s *Store) SetTrackPreference(ctx context.Context, userID string, input PreferenceInput) (TrackPreference, error) {
	if s == nil || s.db == nil {
		return TrackPreference{}, ErrPreferenceStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return TrackPreference{}, err
	}
	normalized, err := NormalizePreferenceInput(input)
	if err != nil {
		return TrackPreference{}, err
	}
	snapshot, err := NewTrackSnapshot(normalized.Track)
	if err != nil {
		return TrackPreference{}, err
	}
	requestHash := PreferenceRequestFingerprint(normalized)
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return TrackPreference{}, fmt.Errorf("encode track preference snapshot: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TrackPreference{}, fmt.Errorf("begin track preference save: %w", err)
	}
	defer tx.Rollback(ctx)

	// Serialize both a repeated idempotency key and the current desired state
	// for one user/track. SELECT ... FOR UPDATE cannot lock an absent row, so
	// the second lock prevents two devices from concurrently inserting the same
	// preference row with different request keys.
	idempotencyLockKey := preferenceLockKey("idempotency", userID, normalized.IdempotencyKey)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, idempotencyLockKey); err != nil {
		return TrackPreference{}, fmt.Errorf("lock track preference idempotency key: %w", err)
	}
	trackLockKey := preferenceLockKey("track", userID, snapshot.Source, snapshot.ID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, trackLockKey); err != nil {
		return TrackPreference{}, fmt.Errorf("lock current track preference: %w", err)
	}

	if replay, found, err := findPreferenceReplay(ctx, tx, userID, normalized.IdempotencyKey, requestHash); err != nil {
		return TrackPreference{}, err
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return TrackPreference{}, fmt.Errorf("commit track preference replay: %w", err)
		}
		return replay, nil
	}

	preference, changed, err := upsertCurrentPreference(ctx, tx, userID, normalized.Preference, snapshot, snapshotJSON)
	if err != nil {
		return TrackPreference{}, err
	}
	if err := savePreferenceOutbox(ctx, tx, userID, preference, snapshotJSON, changed); err != nil {
		return TrackPreference{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_track_preference_idempotency(
			user_id, idempotency_key, request_hash, track_source, track_id,
			preference, track_snapshot, revision, preference_updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, userID, normalized.IdempotencyKey, requestHash, preference.Track.Source, preference.Track.ID,
		preference.Preference, snapshotJSON, preference.Revision, preference.UpdatedAt); err != nil {
		return TrackPreference{}, fmt.Errorf("save track preference idempotency receipt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TrackPreference{}, fmt.Errorf("commit track preference save: %w", err)
	}
	return preference, nil
}

func findPreferenceReplay(ctx context.Context, tx pgx.Tx, userID, key string, requestHash []byte) (TrackPreference, bool, error) {
	var storedHash, snapshotJSON []byte
	var storedPreference string
	var replay TrackPreference
	err := tx.QueryRow(ctx, `
		SELECT request_hash, preference, track_snapshot, revision, preference_updated_at
		FROM user_track_preference_idempotency
		WHERE user_id=$1 AND idempotency_key=$2
	`, userID, key).Scan(&storedHash, &storedPreference, &snapshotJSON, &replay.Revision, &replay.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TrackPreference{}, false, nil
	}
	if err != nil {
		return TrackPreference{}, false, fmt.Errorf("read track preference idempotency receipt: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash) {
		return TrackPreference{}, false, ErrIdempotencyKeyConflict
	}
	snapshot, err := decodeTrackSnapshot(snapshotJSON)
	if err != nil {
		return TrackPreference{}, false, fmt.Errorf("decode replayed track preference: %w", err)
	}
	replay.Track = snapshot
	replay.Preference = Preference(storedPreference)
	return replay, true, nil
}

func upsertCurrentPreference(ctx context.Context, tx pgx.Tx, userID string, desired Preference, snapshot TrackSnapshot, snapshotJSON []byte) (TrackPreference, bool, error) {
	var currentPreference string
	var currentSnapshotJSON []byte
	var current TrackPreference
	err := tx.QueryRow(ctx, `
		SELECT preference, track_snapshot, revision, updated_at
		FROM user_track_preferences
		WHERE user_id=$1 AND track_source=$2 AND track_id=$3
		FOR UPDATE
	`, userID, snapshot.Source, snapshot.ID).Scan(&currentPreference, &currentSnapshotJSON, &current.Revision, &current.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		result := TrackPreference{Track: snapshot, Preference: desired}
		err = tx.QueryRow(ctx, `
			INSERT INTO user_track_preferences(
				user_id, track_source, track_id, preference, track_snapshot, revision
			) VALUES ($1,$2,$3,$4,$5,1)
			RETURNING revision, updated_at
		`, userID, snapshot.Source, snapshot.ID, desired, snapshotJSON).Scan(&result.Revision, &result.UpdatedAt)
		if err != nil {
			return TrackPreference{}, false, fmt.Errorf("insert current track preference: %w", err)
		}
		return result, true, nil
	}
	if err != nil {
		return TrackPreference{}, false, fmt.Errorf("lock current track preference: %w", err)
	}
	currentSnapshot, err := decodeTrackSnapshot(currentSnapshotJSON)
	if err != nil {
		return TrackPreference{}, false, fmt.Errorf("decode current track preference: %w", err)
	}
	current.Track = currentSnapshot
	current.Preference = Preference(currentPreference)
	if current.Preference == desired && current.Track == snapshot {
		return current, false, nil
	}

	result := TrackPreference{Track: snapshot, Preference: desired}
	err = tx.QueryRow(ctx, `
		UPDATE user_track_preferences
		SET preference=$4,
			track_snapshot=$5,
			revision=revision + 1,
			updated_at=now()
		WHERE user_id=$1 AND track_source=$2 AND track_id=$3
		RETURNING revision, updated_at
	`, userID, snapshot.Source, snapshot.ID, desired, snapshotJSON).Scan(&result.Revision, &result.UpdatedAt)
	if err != nil {
		return TrackPreference{}, false, fmt.Errorf("update current track preference: %w", err)
	}
	return result, true, nil
}

func savePreferenceOutbox(ctx context.Context, tx pgx.Tx, userID string, preference TrackPreference, snapshotJSON []byte, reset bool) error {
	if reset {
		_, err := tx.Exec(ctx, `
			INSERT INTO user_track_preference_outbox(
				user_id, track_source, track_id, desired_preference, track_snapshot, revision
			) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (user_id, track_source, track_id) DO UPDATE
			SET desired_preference=EXCLUDED.desired_preference,
				track_snapshot=EXCLUDED.track_snapshot,
				revision=EXCLUDED.revision,
				available_at=now(),
				claimed_until=NULL,
				delivered_at=NULL,
				last_error=NULL,
				updated_at=now()
		`, userID, preference.Track.Source, preference.Track.ID, preference.Preference, snapshotJSON, preference.Revision)
		if err != nil {
			return fmt.Errorf("upsert track preference outbox: %w", err)
		}
		return nil
	}

	// A legacy/imported current row might lack an outbox record. Create one
	// without reopening a record that an eventual worker has already delivered.
	_, err := tx.Exec(ctx, `
		INSERT INTO user_track_preference_outbox(
			user_id, track_source, track_id, desired_preference, track_snapshot, revision
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (user_id, track_source, track_id) DO NOTHING
	`, userID, preference.Track.Source, preference.Track.ID, preference.Preference, snapshotJSON, preference.Revision)
	if err != nil {
		return fmt.Errorf("ensure track preference outbox: %w", err)
	}
	return nil
}

// ListTrackPreferences returns every current state, including neutral records,
// ordered by most recently changed preference first.
func (s *Store) ListTrackPreferences(ctx context.Context, userID string) ([]TrackPreference, error) {
	if s == nil || s.db == nil {
		return nil, ErrPreferenceStoreNoDatabase
	}
	userID = strings.TrimSpace(userID)
	if err := validatePreferenceText("user id", userID, 128, true); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT preference, track_snapshot, revision, updated_at
		FROM user_track_preferences
		WHERE user_id=$1
		ORDER BY updated_at DESC, track_source, track_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list current track preferences: %w", err)
	}
	defer rows.Close()

	preferences := make([]TrackPreference, 0)
	for rows.Next() {
		var rawSnapshot []byte
		var storedPreference string
		var preference TrackPreference
		if err := rows.Scan(&storedPreference, &rawSnapshot, &preference.Revision, &preference.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan current track preference: %w", err)
		}
		snapshot, err := decodeTrackSnapshot(rawSnapshot)
		if err != nil {
			return nil, fmt.Errorf("decode current track preference: %w", err)
		}
		preference.Track = snapshot
		preference.Preference = Preference(storedPreference)
		preferences = append(preferences, preference)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current track preferences: %w", err)
	}
	return preferences, nil
}

func decodeTrackSnapshot(raw []byte) (TrackSnapshot, error) {
	var snapshot TrackSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return TrackSnapshot{}, err
	}
	normalized, err := NewTrackSnapshot(snapshot.musicTrack())
	if err != nil {
		return TrackSnapshot{}, err
	}
	if normalized != snapshot {
		return TrackSnapshot{}, fmt.Errorf("stored snapshot is not canonical")
	}
	return snapshot, nil
}

// preferenceLockKey is length-prefixed so different field boundaries cannot
// accidentally describe the same advisory-lock input. PostgreSQL hash
// collisions only serialize unrelated writes; they never merge their data.
func preferenceLockKey(scope string, values ...string) string {
	var builder strings.Builder
	builder.WriteString("mixora:track-preference:")
	builder.WriteString(scope)
	for _, value := range values {
		builder.WriteByte(':')
		builder.WriteString(fmt.Sprintf("%d:", len(value)))
		builder.WriteString(value)
	}
	return builder.String()
}
