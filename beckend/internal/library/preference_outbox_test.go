package library

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPreferenceOutboxWorkerPublishesCurrentStateAndMarksDelivered(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 1, 10, 0, 0, 0, time.UTC)
	item := testPreferenceOutboxItem(PreferenceNeutral, 7, 1)
	backend := &fakePreferenceOutboxBackend{items: []preferenceOutboxItem{item}}
	publisher := &fakeTrackPreferencePublisher{}
	options := DefaultPreferenceOutboxOptions()
	worker, err := newPreferenceOutboxWorker(backend, publisher, options)
	if err != nil {
		t.Fatalf("newPreferenceOutboxWorker() error = %v", err)
	}
	worker.now = func() time.Time { return now }

	delivered, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if delivered != 1 {
		t.Fatalf("delivered = %d, want 1", delivered)
	}
	if backend.claimBatch != options.BatchSize || !backend.claimNow.Equal(now) || !backend.claimLease.Equal(now.Add(options.Lease)) {
		t.Fatalf("claim arguments = batch=%d now=%s lease=%s", backend.claimBatch, backend.claimNow, backend.claimLease)
	}
	if len(publisher.publications) != 1 {
		t.Fatalf("publications = %#v", publisher.publications)
	}
	publication := publisher.publications[0]
	if publication.UserID != "00000000-0000-0000-0000-000000000042" || publication.Preference.Preference != PreferenceNeutral || publication.Preference.Revision != 7 {
		t.Fatalf("publication = %#v", publication)
	}
	if publication.Preference.Track.Source != "soundcloud" || publication.Preference.Track.ID != "42" {
		t.Fatalf("published track = %#v", publication.Preference.Track)
	}
	if len(backend.delivered) != 1 || backend.delivered[0].item.Revision != 7 || !backend.delivered[0].at.Equal(now) {
		t.Fatalf("delivered marks = %#v", backend.delivered)
	}
	if len(backend.failed) != 0 {
		t.Fatalf("failure marks = %#v", backend.failed)
	}
	if backend.requeueCalls != 1 || !backend.requeueAt.Equal(now) {
		t.Fatalf("catalog requeue = calls=%d at=%s", backend.requeueCalls, backend.requeueAt)
	}
}

func TestPreferenceOutboxWorkerRetriesPublisherFailureWithBoundedBackoff(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 1, 11, 0, 0, 0, time.UTC)
	backend := &fakePreferenceOutboxBackend{items: []preferenceOutboxItem{
		testPreferenceOutboxItem(PreferenceLiked, 3, 3),
	}}
	publisher := &fakeTrackPreferencePublisher{err: errors.New("gorse is unavailable")}
	options := DefaultPreferenceOutboxOptions()
	options.InitialBackoff = time.Second
	options.MaxBackoff = 4 * time.Second
	worker, err := newPreferenceOutboxWorker(backend, publisher, options)
	if err != nil {
		t.Fatalf("newPreferenceOutboxWorker() error = %v", err)
	}
	worker.now = func() time.Time { return now }

	delivered, err := worker.RunOnce(context.Background())
	if delivered != 0 {
		t.Fatalf("delivered = %d, want 0", delivered)
	}
	if err == nil || !strings.Contains(err.Error(), "gorse is unavailable") {
		t.Fatalf("RunOnce() error = %v, want publisher error", err)
	}
	if len(backend.failed) != 1 {
		t.Fatalf("failure marks = %#v", backend.failed)
	}
	failure := backend.failed[0]
	if !failure.retryAt.Equal(now.Add(4 * time.Second)) {
		t.Fatalf("retry at = %s, want %s", failure.retryAt, now.Add(4*time.Second))
	}
	if !errors.Is(failure.cause, publisher.err) {
		t.Fatalf("failure cause = %v, want %v", failure.cause, publisher.err)
	}
	if len(backend.delivered) != 0 {
		t.Fatalf("delivered marks = %#v", backend.delivered)
	}
}

func TestPreferenceOutboxWorkerDefersCorruptSnapshotWithoutPublishing(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	item := testPreferenceOutboxItem(PreferenceDisliked, 1, 1)
	item.TrackSnapshot = json.RawMessage(`{"source":"soundcloud","id":"99","title":"Song","artist":"Artist"}`)
	backend := &fakePreferenceOutboxBackend{items: []preferenceOutboxItem{item}}
	publisher := &fakeTrackPreferencePublisher{}
	options := DefaultPreferenceOutboxOptions()
	options.InitialBackoff = time.Second
	options.MaxBackoff = time.Second
	worker, err := newPreferenceOutboxWorker(backend, publisher, options)
	if err != nil {
		t.Fatalf("newPreferenceOutboxWorker() error = %v", err)
	}
	worker.now = func() time.Time { return now }

	_, err = worker.RunOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not match key") {
		t.Fatalf("RunOnce() error = %v, want snapshot key error", err)
	}
	if len(publisher.publications) != 0 {
		t.Fatalf("publisher was called with %#v", publisher.publications)
	}
	if len(backend.failed) != 1 || !backend.failed[0].retryAt.Equal(now.Add(time.Second)) {
		t.Fatalf("failure marks = %#v", backend.failed)
	}
}

func TestPreferenceOutboxWorkerSkipsAClaimReplacedByNewerState(t *testing.T) {
	t.Parallel()
	item := testPreferenceOutboxItem(PreferenceLiked, 4, 1)
	backend := &fakePreferenceOutboxBackend{
		items:      []preferenceOutboxItem{item},
		current:    false,
		currentSet: true,
	}
	publisher := &fakeTrackPreferencePublisher{}
	worker, err := newPreferenceOutboxWorker(backend, publisher, DefaultPreferenceOutboxOptions())
	if err != nil {
		t.Fatalf("newPreferenceOutboxWorker() error = %v", err)
	}

	delivered, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if delivered != 0 || len(publisher.publications) != 0 {
		t.Fatalf("stale item was published: delivered=%d publications=%#v", delivered, publisher.publications)
	}
	if len(backend.checks) != 1 || len(backend.delivered) != 0 || len(backend.failed) != 0 {
		t.Fatalf("stale item bookkeeping = checks=%#v delivered=%#v failed=%#v", backend.checks, backend.delivered, backend.failed)
	}
}

func TestPreferenceOutboxWorkerMarksUnverifiedTrackForCatalogReconciliation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 5, 13, 0, 0, 0, time.UTC)
	item := testPreferenceOutboxItem(PreferenceLiked, 1, 1)
	backend := &fakePreferenceOutboxBackend{items: []preferenceOutboxItem{item}}
	publisher := &fakeTrackPreferencePublisher{err: ErrPreferenceTrackUnverified}
	worker, err := newPreferenceOutboxWorker(backend, publisher, DefaultPreferenceOutboxOptions())
	if err != nil {
		t.Fatalf("newPreferenceOutboxWorker() error = %v", err)
	}
	worker.now = func() time.Time { return now }

	delivered, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if delivered != 0 {
		t.Fatalf("delivered = %d, want 0", delivered)
	}
	if len(backend.unverified) != 1 || backend.unverified[0].item.TrackID != "42" || !backend.unverified[0].at.Equal(now) {
		t.Fatalf("catalog-unverified marks = %#v", backend.unverified)
	}
	if len(backend.failed) != 0 || len(backend.delivered) != 0 {
		t.Fatalf("unverified bookkeeping = failed=%#v delivered=%#v", backend.failed, backend.delivered)
	}
}

func TestPreferenceOutboxRetryDelayIsExponentialAndBounded(t *testing.T) {
	t.Parallel()
	initial, maximum := time.Second, 8*time.Second
	if got := preferenceOutboxRetryDelay(1, initial, maximum); got != time.Second {
		t.Fatalf("attempt 1 delay = %s", got)
	}
	if got := preferenceOutboxRetryDelay(3, initial, maximum); got != 4*time.Second {
		t.Fatalf("attempt 3 delay = %s", got)
	}
	if got := preferenceOutboxRetryDelay(99, initial, maximum); got != maximum {
		t.Fatalf("large attempt delay = %s, want %s", got, maximum)
	}
}

func testPreferenceOutboxItem(preference Preference, revision int64, attempts int) preferenceOutboxItem {
	return preferenceOutboxItem{
		UserID:            "00000000-0000-0000-0000-000000000042",
		TrackSource:       "soundcloud",
		TrackID:           "42",
		DesiredPreference: preference,
		TrackSnapshot:     json.RawMessage(`{"source":"soundcloud","id":"42","title":"Song","artist":"Artist","access":"playable"}`),
		Revision:          revision,
		Attempts:          attempts,
		ClaimedUntil:      time.Date(2026, time.October, 1, 13, 0, 0, 0, time.UTC),
		UpdatedAt:         time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC),
	}
}

type fakeTrackPreferencePublisher struct {
	publications []TrackPreferencePublication
	err          error
}

func (p *fakeTrackPreferencePublisher) PublishTrackPreference(_ context.Context, publication TrackPreferencePublication) error {
	p.publications = append(p.publications, publication)
	return p.err
}

type fakePreferenceOutboxBackend struct {
	items        []preferenceOutboxItem
	claimErr     error
	current      bool
	currentSet   bool
	currentErr   error
	checks       []preferenceOutboxItem
	claimNow     time.Time
	claimLease   time.Time
	claimBatch   int
	requeueCalls int
	requeueAt    time.Time
	delivered    []fakePreferenceDelivery
	unverified   []fakePreferenceDelivery
	failed       []fakePreferenceFailure
}

func (f *fakePreferenceOutboxBackend) requeueCatalogVerifiedPreferenceOutbox(_ context.Context, now time.Time) error {
	f.requeueCalls++
	f.requeueAt = now
	return nil
}

type fakePreferenceDelivery struct {
	item preferenceOutboxItem
	at   time.Time
}

type fakePreferenceFailure struct {
	item    preferenceOutboxItem
	retryAt time.Time
	cause   error
}

func (f *fakePreferenceOutboxBackend) claimPreferenceOutbox(_ context.Context, now, lease time.Time, batch int) ([]preferenceOutboxItem, error) {
	f.claimNow, f.claimLease, f.claimBatch = now, lease, batch
	return append([]preferenceOutboxItem(nil), f.items...), f.claimErr
}

func (f *fakePreferenceOutboxBackend) isCurrentPreferenceOutbox(_ context.Context, item preferenceOutboxItem) (bool, error) {
	f.checks = append(f.checks, item)
	if f.currentErr != nil {
		return false, f.currentErr
	}
	if !f.currentSet {
		return true, nil
	}
	return f.current, nil
}

func (f *fakePreferenceOutboxBackend) markPreferenceOutboxDelivered(_ context.Context, item preferenceOutboxItem, at time.Time) error {
	f.delivered = append(f.delivered, fakePreferenceDelivery{item: item, at: at})
	return nil
}

func (f *fakePreferenceOutboxBackend) markPreferenceOutboxCatalogUnverified(_ context.Context, item preferenceOutboxItem, at time.Time) error {
	f.unverified = append(f.unverified, fakePreferenceDelivery{item: item, at: at})
	return nil
}

func (f *fakePreferenceOutboxBackend) markPreferenceOutboxFailed(_ context.Context, item preferenceOutboxItem, retryAt time.Time, cause error) error {
	f.failed = append(f.failed, fakePreferenceFailure{item: item, retryAt: retryAt, cause: cause})
	return nil
}
