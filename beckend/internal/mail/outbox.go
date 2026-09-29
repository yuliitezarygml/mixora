package mail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Message struct {
	Kind     string
	To       string
	Subject  string
	TextBody string
	HTMLBody string
}

func (m Message) validate() error {
	if strings.TrimSpace(m.Kind) == "" {
		return errors.New("mail kind is required")
	}
	if strings.TrimSpace(m.To) == "" || strings.ContainsAny(m.To, "\r\n") {
		return errors.New("mail recipient is invalid")
	}
	if strings.TrimSpace(m.Subject) == "" || strings.ContainsAny(m.Subject, "\r\n") {
		return errors.New("mail subject is invalid")
	}
	if m.TextBody == "" {
		return errors.New("mail text body is required")
	}
	return nil
}

type Outbox struct {
	pool *pgxpool.Pool
}

func NewOutbox(pool *pgxpool.Pool) (*Outbox, error) {
	if pool == nil {
		return nil, fmt.Errorf("mail outbox requires a database pool")
	}
	return &Outbox{pool: pool}, nil
}

// Enqueue persists a message before it is sent. Callers can safely return from
// an HTTP request once this succeeds; the worker retries transient SMTP errors.
func (o *Outbox) Enqueue(ctx context.Context, message Message) (int64, error) {
	if err := message.validate(); err != nil {
		return 0, err
	}
	var id int64
	err := o.pool.QueryRow(ctx, `
		INSERT INTO outbox (kind, recipient, subject, text_body, html_body)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		message.Kind,
		message.To,
		message.Subject,
		message.TextBody,
		message.HTMLBody,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("enqueue email: %w", err)
	}
	return id, nil
}

type outboxItem struct {
	ID       int64
	Message  Message
	Attempts int
}

func (o *Outbox) claim(
	ctx context.Context,
	now time.Time,
	leaseUntil time.Time,
	maxAttempts int,
	batchSize int,
) ([]outboxItem, error) {
	rows, err := o.pool.Query(ctx, `
		WITH picked AS (
			SELECT id
			FROM outbox
			WHERE delivered_at IS NULL
			  AND available_at <= $1
			  AND attempts < $3
			ORDER BY available_at, id
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox AS queued
		SET attempts = queued.attempts + 1,
		    available_at = $2
		FROM picked
		WHERE queued.id = picked.id
		RETURNING queued.id, queued.kind, queued.recipient, queued.subject,
		          queued.text_body, queued.html_body, queued.attempts`,
		now,
		leaseUntil,
		maxAttempts,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("claim outbox messages: %w", err)
	}
	defer rows.Close()

	items := make([]outboxItem, 0, batchSize)
	for rows.Next() {
		var item outboxItem
		if err := rows.Scan(
			&item.ID,
			&item.Message.Kind,
			&item.Message.To,
			&item.Message.Subject,
			&item.Message.TextBody,
			&item.Message.HTMLBody,
			&item.Attempts,
		); err != nil {
			return nil, fmt.Errorf("scan outbox message: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read outbox messages: %w", err)
	}
	return items, nil
}

func (o *Outbox) markDelivered(ctx context.Context, id int64, deliveredAt time.Time) error {
	_, err := o.pool.Exec(ctx, `
		UPDATE outbox
		SET delivered_at = $2, last_error = NULL
		WHERE id = $1 AND delivered_at IS NULL`, id, deliveredAt,
	)
	if err != nil {
		return fmt.Errorf("mark email %d delivered: %w", id, err)
	}
	return nil
}

func (o *Outbox) markFailed(
	ctx context.Context,
	id int64,
	retryAt time.Time,
	deliveryError error,
) error {
	lastError := deliveryError.Error()
	if len(lastError) > 2000 {
		lastError = lastError[:2000]
	}
	_, err := o.pool.Exec(ctx, `
		UPDATE outbox
		SET available_at = $2, last_error = $3
		WHERE id = $1 AND delivered_at IS NULL`, id, retryAt, lastError,
	)
	if err != nil {
		return fmt.Errorf("record failure for email %d: %w", id, err)
	}
	return nil
}
