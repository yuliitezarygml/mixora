package events

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		event   Event
		wantErr bool
	}{
		{"play", Event{Key: "event-1", Type: "play", Source: "soundcloud", TrackID: "12"}, false},
		{"search", Event{Key: "event-2", Type: "search", Context: json.RawMessage(`{"q":"jazz"}`)}, false},
		{"missing key", Event{Type: "play", Source: "soundcloud", TrackID: "12"}, true},
		{"missing track", Event{Key: "event-3", Type: "complete", Source: "soundcloud"}, true},
		{"unknown", Event{Key: "event-4", Type: "opened_player", Source: "soundcloud", TrackID: "12"}, true},
		{"invalid context", Event{Key: "event-5", Type: "play", Source: "soundcloud", TrackID: "12", Context: json.RawMessage(`{`)}, true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Validate(test.event); (got != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", got, test.wantErr)
			}
		})
	}
}

func TestDeduplicatesWaveEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		event Event
		want  bool
	}{
		{"wave seek", Event{Type: "seek", SessionID: "session-1"}, true},
		{"wave like", Event{Type: "like", SessionID: "session-1"}, true},
		{"ordinary play", Event{Type: "play"}, false},
		{"search is never a wave feedback signal", Event{Type: "search", SessionID: "session-1"}, false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := deduplicatesWaveEvent(test.event); got != test.want {
				t.Fatalf("deduplicatesWaveEvent(%+v) = %v, want %v", test.event, got, test.want)
			}
		})
	}
}

func TestAddSerializesIdempotencyAndWaveReceiptBeforeWriting(t *testing.T) {
	t.Parallel()
	results := &fakeEventBatchResults{}
	tx := &fakeEventTx{results: results}
	store := &Store{db: &fakeEventTransactionStarter{tx: tx}}
	input := []Event{
		{Key: "same-key", Type: "like", Source: "soundcloud", TrackID: "42", SessionID: "wave-1"},
		{Key: "ordinary-key", Type: "play", Source: "soundcloud", TrackID: "43"},
		{Key: "same-key", Type: "like", Source: "soundcloud", TrackID: "42", SessionID: "wave-1"},
	}

	if err := store.Add(context.Background(), "user-1", input); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	wantLocks := eventWriteLockKeys("user-1", input)
	if !reflect.DeepEqual(tx.lockKeys, wantLocks) {
		t.Fatalf("advisory locks = %#v, want %#v", tx.lockKeys, wantLocks)
	}
	if tx.batch == nil || tx.batch.Len() != len(input) {
		t.Fatalf("saved batch = %#v, want %d queries", tx.batch, len(input))
	}
	query := tx.batch.QueuedQueries[0].SQL
	if !strings.Contains(query, "existing_receipt") || !strings.Contains(query, "RETURNING 1") {
		t.Fatalf("wave event query does not gate the receipt: %s", query)
	}
	if strings.Contains(query, "ON CONFLICT (user_id, event_key)") {
		t.Fatalf("journal conflict must abort the transaction instead of leaving an orphan receipt: %s", query)
	}
	if !reflect.DeepEqual(tx.calls, []string{"send_batch", "commit", "rollback"}) {
		t.Fatalf("transaction calls = %#v", tx.calls)
	}
}

func TestAddRollsBackWaveReceiptWhenJournalWriteFails(t *testing.T) {
	t.Parallel()
	results := &fakeEventBatchResults{failAt: 1}
	tx := &fakeEventTx{results: results}
	store := &Store{db: &fakeEventTransactionStarter{tx: tx}}

	err := store.Add(context.Background(), "user-1", []Event{
		{Key: "event-key", Type: "like", Source: "soundcloud", TrackID: "42", SessionID: "wave-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "save listening event") {
		t.Fatalf("Add() error = %v, want journal write error", err)
	}
	if tx.committed {
		t.Fatal("transaction committed after a journal write error")
	}
	if !reflect.DeepEqual(tx.calls, []string{"send_batch", "rollback"}) {
		t.Fatalf("transaction calls = %#v, want rollback without commit", tx.calls)
	}
}

func TestEventWriteLockKeysAreUnambiguousAndDeterministic(t *testing.T) {
	t.Parallel()
	first := eventWriteLockKeys("ab", []Event{{Key: "c", Type: "like", Source: "soundcloud", TrackID: "42", SessionID: "wave"}})
	second := eventWriteLockKeys("a", []Event{{Key: "bc", Type: "like", Source: "soundcloud", TrackID: "42", SessionID: "wave"}})
	if reflect.DeepEqual(first, second) {
		t.Fatalf("ambiguous lock tuples: %#v == %#v", first, second)
	}

	input := []Event{
		{Key: "z", Type: "play", Source: "soundcloud", TrackID: "1"},
		{Key: "a", Type: "like", Source: "soundcloud", TrackID: "2", SessionID: "wave"},
	}
	forward := eventWriteLockKeys("user", input)
	reversed := eventWriteLockKeys("user", []Event{input[1], input[0]})
	if !reflect.DeepEqual(forward, reversed) {
		t.Fatalf("lock order depends on batch order: %#v != %#v", forward, reversed)
	}
}

type fakeEventTransactionStarter struct {
	tx pgx.Tx
}

func (s *fakeEventTransactionStarter) Begin(context.Context) (pgx.Tx, error) {
	return s.tx, nil
}

type fakeEventTx struct {
	pgx.Tx
	lockKeys  []string
	batch     *pgx.Batch
	results   pgx.BatchResults
	calls     []string
	committed bool
}

func (tx *fakeEventTx) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "pg_advisory_xact_lock") {
		if len(arguments) != 1 {
			return pgconn.CommandTag{}, errors.New("unexpected advisory lock arguments")
		}
		lockKey, ok := arguments[0].(string)
		if !ok {
			return pgconn.CommandTag{}, errors.New("advisory lock key is not text")
		}
		tx.lockKeys = append(tx.lockKeys, lockKey)
	}
	return pgconn.NewCommandTag("SELECT 1"), nil
}

func (tx *fakeEventTx) SendBatch(_ context.Context, batch *pgx.Batch) pgx.BatchResults {
	tx.calls = append(tx.calls, "send_batch")
	tx.batch = batch
	return tx.results
}

func (tx *fakeEventTx) Commit(context.Context) error {
	tx.calls = append(tx.calls, "commit")
	tx.committed = true
	return nil
}

func (tx *fakeEventTx) Rollback(context.Context) error {
	tx.calls = append(tx.calls, "rollback")
	return nil
}

type fakeEventBatchResults struct {
	pgx.BatchResults
	execCount int
	failAt    int
}

func (r *fakeEventBatchResults) Exec() (pgconn.CommandTag, error) {
	r.execCount++
	if r.failAt == r.execCount {
		return pgconn.CommandTag{}, errors.New("journal insert failed")
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (*fakeEventBatchResults) Close() error {
	return nil
}
