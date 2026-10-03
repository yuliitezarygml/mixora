// Package recommendationlock serializes external recommendation publication
// across independent durable workers. PostgreSQL session advisory locks make
// the lease safe across API processes without introducing another service.
package recommendationlock

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const advisoryLockID int64 = 0x4d49584f52415245 // "MIXORARE"

type Lease struct {
	connection *pgxpool.Conn
}

// TryAcquire returns acquired=false when another worker is already publishing
// recommendation feedback. The caller can leave durable work pending and retry
// on its normal polling interval.
func TryAcquire(ctx context.Context, db *pgxpool.Pool) (*Lease, bool, error) {
	if db == nil {
		return nil, false, fmt.Errorf("recommendation publication database is required")
	}
	connection, err := db.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire recommendation publication connection: %w", err)
	}
	var acquired bool
	if err := connection.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, advisoryLockID).Scan(&acquired); err != nil {
		connection.Release()
		return nil, false, fmt.Errorf("acquire recommendation publication lease: %w", err)
	}
	if !acquired {
		connection.Release()
		return nil, false, nil
	}
	return &Lease{connection: connection}, true, nil
}

func (l *Lease) Connection() *pgxpool.Conn {
	if l == nil {
		return nil
	}
	return l.connection
}

func (l *Lease) Release() {
	if l == nil || l.connection == nil {
		return
	}
	unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = l.connection.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, advisoryLockID)
	l.connection.Release()
	l.connection = nil
}
