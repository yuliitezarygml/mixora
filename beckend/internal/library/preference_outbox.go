package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/recommendationlock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxPreferenceOutboxErrorLength = 1000

var (
	ErrPreferenceOutboxLeaseLost = errors.New("track preference outbox lease is no longer held")
	// ErrPreferenceTrackUnverified tells the outbox that the product state is
	// valid but its source:id has not yet been observed by the trusted catalog.
	// It is terminal for this attempt, not a transient Gorse failure.
	ErrPreferenceTrackUnverified = errors.New("track preference is not verified by the server catalog")
	errStalePreferenceOutboxItem = errors.New("track preference outbox item is no longer current")
)

// TrackPreferencePublication is the immutable desired state that a downstream
// recommender must make visible. Revision is monotonic for one user/track key.
// A publisher must tolerate duplicate calls: a worker can lose its lease after
// a successful remote call but before it records delivery locally.
type TrackPreferencePublication struct {
	UserID     string
	Preference TrackPreference
}

// TrackPreferencePublisher is deliberately independent of Gorse. The adapter
// that is added later can turn liked/disliked/neutral into the corresponding
// Gorse upsert/delete operations without giving this durable worker a network
// dependency.
type TrackPreferencePublisher interface {
	PublishTrackPreference(context.Context, TrackPreferencePublication) error
}

// PreferenceOutboxOptions controls local delivery cadence. Failed messages are
// never dropped: InitialBackoff is doubled per attempt and capped at MaxBackoff.
type PreferenceOutboxOptions struct {
	BatchSize      int
	PollInterval   time.Duration
	Lease          time.Duration
	PublishTimeout time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	OnError        func(error)
}

func DefaultPreferenceOutboxOptions() PreferenceOutboxOptions {
	return PreferenceOutboxOptions{
		BatchSize:      25,
		PollInterval:   2 * time.Second,
		Lease:          30 * time.Second,
		PublishTimeout: 10 * time.Second,
		InitialBackoff: 5 * time.Second,
		MaxBackoff:     5 * time.Minute,
	}
}

// PreferenceOutboxWorker delivers the latest coalesced desired state for each
// user/track pair. It does not own the preference API and is safe to run in
// more than one API process: claims use FOR UPDATE SKIP LOCKED plus a lease.
type PreferenceOutboxWorker struct {
	outbox    preferenceOutboxBackend
	publisher TrackPreferencePublisher
	options   PreferenceOutboxOptions
	now       func() time.Time
	db        *pgxpool.Pool
}

type preferenceOutboxBackend interface {
	requeueCatalogVerifiedPreferenceOutbox(context.Context, time.Time) error
	claimPreferenceOutbox(context.Context, time.Time, time.Time, int) ([]preferenceOutboxItem, error)
	isCurrentPreferenceOutbox(context.Context, preferenceOutboxItem) (bool, error)
	markPreferenceOutboxDelivered(context.Context, preferenceOutboxItem, time.Time) error
	markPreferenceOutboxCatalogUnverified(context.Context, preferenceOutboxItem, time.Time) error
	markPreferenceOutboxFailed(context.Context, preferenceOutboxItem, time.Time, error) error
}

type preferenceOutboxStore struct {
	db *pgxpool.Pool
}

type preferenceOutboxItem struct {
	UserID            string
	TrackSource       string
	TrackID           string
	DesiredPreference Preference
	TrackSnapshot     json.RawMessage
	Revision          int64
	Attempts          int
	ClaimedUntil      time.Time
	UpdatedAt         time.Time
}

// NewPreferenceOutboxWorker makes the PostgreSQL-backed worker. The publisher
// is the only extension point required to connect a recommender implementation.
func NewPreferenceOutboxWorker(db *pgxpool.Pool, publisher TrackPreferencePublisher, options PreferenceOutboxOptions) (*PreferenceOutboxWorker, error) {
	if db == nil {
		return nil, errors.New("track preference outbox worker requires a database pool")
	}
	worker, err := newPreferenceOutboxWorker(&preferenceOutboxStore{db: db}, publisher, options)
	if err != nil {
		return nil, err
	}
	worker.db = db
	return worker, nil
}

func newPreferenceOutboxWorker(outbox preferenceOutboxBackend, publisher TrackPreferencePublisher, options PreferenceOutboxOptions) (*PreferenceOutboxWorker, error) {
	if outbox == nil {
		return nil, errors.New("track preference outbox worker requires an outbox")
	}
	if publisher == nil {
		return nil, errors.New("track preference outbox worker requires a publisher")
	}
	if options.BatchSize <= 0 || options.BatchSize > 1000 {
		return nil, errors.New("track preference outbox batch size must be between 1 and 1000")
	}
	if options.PollInterval <= 0 {
		return nil, errors.New("track preference outbox poll interval must be positive")
	}
	if options.Lease <= 0 {
		return nil, errors.New("track preference outbox lease must be positive")
	}
	if options.PublishTimeout <= 0 {
		return nil, errors.New("track preference outbox publish timeout must be positive")
	}
	if options.Lease < options.PublishTimeout {
		return nil, errors.New("track preference outbox lease must not be shorter than publish timeout")
	}
	if options.InitialBackoff <= 0 || options.MaxBackoff < options.InitialBackoff {
		return nil, errors.New("track preference outbox backoff bounds are invalid")
	}
	if options.OnError == nil {
		options.OnError = func(error) {}
	}
	return &PreferenceOutboxWorker{outbox: outbox, publisher: publisher, options: options, now: time.Now}, nil
}

// RunOnce claims one batch and returns how many current desired states were
// durably marked delivered. A failed publication remains available for retry.
func (w *PreferenceOutboxWorker) RunOnce(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var publicationLease *recommendationlock.Lease
	if w.db != nil {
		lease, acquired, err := recommendationlock.TryAcquire(ctx, w.db)
		if err != nil {
			return 0, err
		}
		if !acquired {
			return 0, nil
		}
		publicationLease = lease
		defer publicationLease.Release()
	}
	now := w.now().UTC()
	if err := w.outbox.requeueCatalogVerifiedPreferenceOutbox(ctx, now); err != nil {
		return 0, fmt.Errorf("requeue catalog-verified track preferences: %w", err)
	}
	items, err := w.outbox.claimPreferenceOutbox(ctx, now, now.Add(w.options.Lease), w.options.BatchSize)
	if err != nil {
		return 0, err
	}

	delivered := 0
	batchErrors := make([]error, 0)
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return delivered, errors.Join(append(batchErrors, err)...)
		}
		publishErr := w.publishCurrent(ctx, item, publicationLease)
		if errors.Is(publishErr, errStalePreferenceOutboxItem) {
			// A newer preference atomically reset this outbox row. It owns the
			// next delivery attempt, so never retry or mark the stale claim.
			continue
		}
		if errors.Is(publishErr, ErrPreferenceTrackUnverified) {
			if err := w.outbox.markPreferenceOutboxCatalogUnverified(ctx, item, w.now().UTC()); err != nil {
				batchErrors = append(batchErrors, fmt.Errorf("mark unverified track preference %s/%s: %w", item.TrackSource, item.TrackID, err))
			}
			continue
		}
		if publishErr != nil {
			retryAt := w.now().UTC().Add(preferenceOutboxRetryDelay(item.Attempts, w.options.InitialBackoff, w.options.MaxBackoff))
			if retryErr := w.outbox.markPreferenceOutboxFailed(ctx, item, retryAt, publishErr); retryErr != nil {
				batchErrors = append(batchErrors, fmt.Errorf("defer track preference %s/%s: %w", item.TrackSource, item.TrackID, retryErr))
			}
			batchErrors = append(batchErrors, fmt.Errorf("publish track preference %s/%s: %w", item.TrackSource, item.TrackID, publishErr))
			continue
		}
		if err := w.outbox.markPreferenceOutboxDelivered(ctx, item, w.now().UTC()); err != nil {
			batchErrors = append(batchErrors, fmt.Errorf("mark track preference %s/%s delivered: %w", item.TrackSource, item.TrackID, err))
			continue
		}
		delivered++
	}
	return delivered, errors.Join(batchErrors...)
}

// publishCurrent keeps an old claimed outbox row from being sent after a
// second device has already committed a newer desired state. SetTrackPreference
// uses the same per-track advisory lock transactionally. Holding its session
// counterpart across the external call makes the visible Gorse order match
// the committed preference order; the current-row check covers a write that
// won the race before this worker got the lock.
func (w *PreferenceOutboxWorker) publishCurrent(ctx context.Context, item preferenceOutboxItem, lease *recommendationlock.Lease) error {
	var release func()
	if lease != nil {
		lockCtx, cancel := context.WithTimeout(ctx, w.options.PublishTimeout)
		defer cancel()
		unlock, err := lockPreferencePublication(lockCtx, lease.Connection(), item)
		if err != nil {
			return err
		}
		release = unlock
		defer release()
	}

	var current bool
	var err error
	if lease != nil {
		// Use the already-acquired publication connection while holding the
		// per-track lock. This avoids waiting for another pool connection
		// while concurrent writes are themselves waiting for that lock.
		current, err = currentPreferenceOutbox(ctx, lease.Connection(), item)
	} else {
		current, err = w.outbox.isCurrentPreferenceOutbox(ctx, item)
	}
	if err != nil {
		return fmt.Errorf("check current track preference outbox: %w", err)
	}
	if !current {
		return errStalePreferenceOutboxItem
	}

	publication, err := item.publication()
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, w.options.PublishTimeout)
	defer cancel()
	return w.publisher.PublishTrackPreference(publishCtx, publication)
}

func lockPreferencePublication(ctx context.Context, connection *pgxpool.Conn, item preferenceOutboxItem) (func(), error) {
	if connection == nil {
		return nil, errors.New("track preference publication connection is unavailable")
	}
	lockKey := preferenceLockKey("track", item.UserID, item.TrackSource, item.TrackID)
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return nil, fmt.Errorf("lock track preference publication: %w", err)
	}
	return func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = connection.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey)
	}, nil
}

// Run keeps polling after individual publication failures. OnError is called
// once per failed batch and can be connected to structured logging by main.
func (w *PreferenceOutboxWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		if _, err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.options.OnError(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (item preferenceOutboxItem) publication() (TrackPreferencePublication, error) {
	userID := strings.TrimSpace(item.UserID)
	if userID == "" {
		return TrackPreferencePublication{}, errors.New("outbox user ID is empty")
	}
	preference, err := normalizePreference(item.DesiredPreference)
	if err != nil {
		return TrackPreferencePublication{}, err
	}
	snapshot, err := decodeTrackSnapshot(item.TrackSnapshot)
	if err != nil {
		return TrackPreferencePublication{}, fmt.Errorf("decode outbox track snapshot: %w", err)
	}
	if snapshot.Source != item.TrackSource || snapshot.ID != item.TrackID {
		return TrackPreferencePublication{}, errors.New("outbox track snapshot does not match key")
	}
	if item.Revision <= 0 {
		return TrackPreferencePublication{}, errors.New("outbox revision must be positive")
	}
	return TrackPreferencePublication{
		UserID: userID,
		Preference: TrackPreference{
			Track:      snapshot,
			Preference: preference,
			Revision:   item.Revision,
			UpdatedAt:  item.UpdatedAt,
		},
	}, nil
}

func preferenceOutboxRetryDelay(attempt int, initial, maximum time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := initial
	for retry := 1; retry < attempt && delay < maximum; retry++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func (o *preferenceOutboxStore) claimPreferenceOutbox(ctx context.Context, now, leaseUntil time.Time, batchSize int) ([]preferenceOutboxItem, error) {
	rows, err := o.db.Query(ctx, `
		WITH picked AS (
			SELECT user_id, track_source, track_id
			FROM user_track_preference_outbox
			WHERE delivered_at IS NULL
			  AND available_at <= $1
			  AND (claimed_until IS NULL OR claimed_until <= $1)
			ORDER BY available_at, user_id, track_source, track_id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		UPDATE user_track_preference_outbox AS queued
		SET attempts = queued.attempts + 1,
		    claimed_until = $2
		FROM picked
		WHERE queued.user_id = picked.user_id
		  AND queued.track_source = picked.track_source
		  AND queued.track_id = picked.track_id
		RETURNING queued.user_id::text, queued.track_source, queued.track_id,
		          queued.desired_preference, queued.track_snapshot, queued.revision,
		          queued.attempts, queued.claimed_until, queued.updated_at
	`, now, leaseUntil, batchSize)
	if err != nil {
		return nil, fmt.Errorf("claim track preference outbox: %w", err)
	}
	defer rows.Close()

	items := make([]preferenceOutboxItem, 0, batchSize)
	for rows.Next() {
		var item preferenceOutboxItem
		if err := rows.Scan(
			&item.UserID,
			&item.TrackSource,
			&item.TrackID,
			&item.DesiredPreference,
			&item.TrackSnapshot,
			&item.Revision,
			&item.Attempts,
			&item.ClaimedUntil,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan track preference outbox: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read track preference outbox: %w", err)
	}
	return items, nil
}

// requeueCatalogVerifiedPreferenceOutbox restores only states that were
// terminal-skipped because their source:id was not in the catalog yet. A
// normal completed delivery is never reopened, so frequently observed tracks
// do not create repeated Gorse mutations.
func (o *preferenceOutboxStore) requeueCatalogVerifiedPreferenceOutbox(ctx context.Context, now time.Time) error {
	_, err := o.db.Exec(ctx, `
		UPDATE user_track_preference_outbox AS queued
		SET delivered_at=NULL,
			catalog_unverified_at=NULL,
			available_at=$1,
			claimed_until=NULL,
			attempts=0,
			last_error=NULL,
			updated_at=$1
		WHERE queued.catalog_unverified_at IS NOT NULL
		  AND EXISTS (
			SELECT 1
			FROM track_catalog AS catalog
			WHERE (catalog.track_source = queued.track_source AND catalog.track_id = queued.track_id)
			   OR (
				queued.track_source = 'soundcloud'
				AND LOWER(catalog.track_source) = 'soundcloud'
				AND catalog.track_id ~ '(^|:|/)[0-9]+$'
				AND substring(catalog.track_id FROM '([0-9]+)$') = queued.track_id
			   )
		  )
	`, now)
	if err != nil {
		return fmt.Errorf("requeue catalog-verified track preferences: %w", err)
	}
	return nil
}

func (o *preferenceOutboxStore) isCurrentPreferenceOutbox(ctx context.Context, item preferenceOutboxItem) (bool, error) {
	return currentPreferenceOutbox(ctx, o.db, item)
}

type preferenceOutboxRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func currentPreferenceOutbox(ctx context.Context, query preferenceOutboxRowQuerier, item preferenceOutboxItem) (bool, error) {
	var current bool
	err := query.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM user_track_preference_outbox
			WHERE user_id=$1
			  AND track_source=$2
			  AND track_id=$3
			  AND revision=$4
			  AND delivered_at IS NULL
			  AND claimed_until=$5
		)
	`, item.UserID, item.TrackSource, item.TrackID, item.Revision, item.ClaimedUntil).Scan(&current)
	if err != nil {
		return false, fmt.Errorf("check current track preference outbox: %w", err)
	}
	return current, nil
}

func (o *preferenceOutboxStore) markPreferenceOutboxDelivered(ctx context.Context, item preferenceOutboxItem, deliveredAt time.Time) error {
	result, err := o.db.Exec(ctx, `
		UPDATE user_track_preference_outbox
		SET delivered_at=$6, catalog_unverified_at=NULL, claimed_until=NULL, last_error=NULL, updated_at=$6
		WHERE user_id=$1
		  AND track_source=$2
		  AND track_id=$3
		  AND revision=$4
		  AND delivered_at IS NULL
		  AND claimed_until=$5
	`, item.UserID, item.TrackSource, item.TrackID, item.Revision, item.ClaimedUntil, deliveredAt)
	if err != nil {
		return fmt.Errorf("mark track preference outbox delivered: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrPreferenceOutboxLeaseLost
	}
	return nil
}

func (o *preferenceOutboxStore) markPreferenceOutboxCatalogUnverified(ctx context.Context, item preferenceOutboxItem, at time.Time) error {
	result, err := o.db.Exec(ctx, `
		UPDATE user_track_preference_outbox
		SET delivered_at=$6,
			catalog_unverified_at=$6,
			claimed_until=NULL,
			last_error='track has not yet been observed in the server catalog',
			updated_at=$6
		WHERE user_id=$1
		  AND track_source=$2
		  AND track_id=$3
		  AND revision=$4
		  AND delivered_at IS NULL
		  AND claimed_until=$5
	`, item.UserID, item.TrackSource, item.TrackID, item.Revision, item.ClaimedUntil, at)
	if err != nil {
		return fmt.Errorf("mark track preference catalog-unverified: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrPreferenceOutboxLeaseLost
	}
	return nil
}

func (o *preferenceOutboxStore) markPreferenceOutboxFailed(ctx context.Context, item preferenceOutboxItem, retryAt time.Time, cause error) error {
	message := strings.TrimSpace(cause.Error())
	if len(message) > maxPreferenceOutboxErrorLength {
		message = message[:maxPreferenceOutboxErrorLength]
	}
	result, err := o.db.Exec(ctx, `
		UPDATE user_track_preference_outbox
		SET available_at=$6, claimed_until=NULL, last_error=$7, updated_at=now()
		WHERE user_id=$1
		  AND track_source=$2
		  AND track_id=$3
		  AND revision=$4
		  AND delivered_at IS NULL
		  AND claimed_until=$5
	`, item.UserID, item.TrackSource, item.TrackID, item.Revision, item.ClaimedUntil, retryAt, message)
	if err != nil {
		return fmt.Errorf("record track preference outbox failure: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrPreferenceOutboxLeaseLost
	}
	return nil
}
