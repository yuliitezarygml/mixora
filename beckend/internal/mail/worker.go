package mail

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type WorkerOptions struct {
	BatchSize    int
	MaxAttempts  int
	PollInterval time.Duration
	Lease        time.Duration
	OnError      func(error)
}

func DefaultWorkerOptions() WorkerOptions {
	return WorkerOptions{
		BatchSize:    10,
		MaxAttempts:  8,
		PollInterval: 2 * time.Second,
		Lease:        2 * time.Minute,
	}
}

type Worker struct {
	outbox  *Outbox
	sender  Sender
	options WorkerOptions
	now     func() time.Time
}

func NewWorker(outbox *Outbox, sender Sender, options WorkerOptions) (*Worker, error) {
	if outbox == nil {
		return nil, fmt.Errorf("mail worker requires an outbox")
	}
	if sender == nil {
		return nil, fmt.Errorf("mail worker requires a sender")
	}
	if options.BatchSize <= 0 || options.BatchSize > 100 {
		return nil, fmt.Errorf("mail worker batch size must be between 1 and 100")
	}
	if options.MaxAttempts <= 0 || options.MaxAttempts > 20 {
		return nil, fmt.Errorf("mail worker max attempts must be between 1 and 20")
	}
	if options.PollInterval <= 0 {
		return nil, fmt.Errorf("mail worker poll interval must be positive")
	}
	if options.Lease < 30*time.Second {
		return nil, fmt.Errorf("mail worker lease must be at least 30 seconds")
	}
	if options.OnError == nil {
		options.OnError = func(error) {}
	}
	return &Worker{outbox: outbox, sender: sender, options: options, now: time.Now}, nil
}

// RunOnce claims and attempts one batch. Claimed rows receive a lease, so more
// than one worker process can run without normally sending the same message.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	now := w.now().UTC()
	items, err := w.outbox.claim(
		ctx,
		now,
		now.Add(w.options.Lease),
		w.options.MaxAttempts,
		w.options.BatchSize,
	)
	if err != nil {
		return 0, err
	}

	delivered := 0
	batchErrors := make([]error, 0)
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return delivered, errors.Join(append(batchErrors, err)...)
		}
		if err := w.sender.Send(ctx, item.Message); err != nil {
			recordError := w.outbox.markFailed(
				ctx,
				item.ID,
				w.now().UTC().Add(retryDelay(item.Attempts)),
				err,
			)
			batchErrors = append(batchErrors, fmt.Errorf("deliver email %d: %w", item.ID, err))
			if recordError != nil {
				batchErrors = append(batchErrors, recordError)
			}
			continue
		}
		if err := w.outbox.markDelivered(ctx, item.ID, w.now().UTC()); err != nil {
			batchErrors = append(batchErrors, err)
			continue
		}
		delivered++
	}
	return delivered, errors.Join(batchErrors...)
}

func (w *Worker) Run(ctx context.Context) error {
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

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 7 {
		attempt = 7
	}
	return time.Duration(1<<(attempt-1)) * time.Minute
}
