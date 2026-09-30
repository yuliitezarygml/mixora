package embedding

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Embedder interface {
	Embed(context.Context, []string) ([][]float32, error)
}

type Repository interface {
	Pending(context.Context, int) ([]PendingItem, error)
	Save(context.Context, []PendingItem, [][]float32) error
	Fail(context.Context, []PendingItem, error) error
}

type WorkerOptions struct {
	BatchSize int
	Interval  time.Duration
	OnError   func(error)
}

type Worker struct {
	repository Repository
	embedder   Embedder
	options    WorkerOptions
}

func DefaultWorkerOptions() WorkerOptions {
	return WorkerOptions{BatchSize: 16, Interval: 15 * time.Second}
}

func NewWorker(repository Repository, embedder Embedder, options WorkerOptions) (*Worker, error) {
	if repository == nil || embedder == nil {
		return nil, errors.New("embedding repository and embedder are required")
	}
	if options.BatchSize <= 0 {
		return nil, errors.New("embedding batch size must be positive")
	}
	if options.Interval <= 0 {
		return nil, errors.New("embedding interval must be positive")
	}
	return &Worker{repository: repository, embedder: embedder, options: options}, nil
}

func (w *Worker) ProcessOnce(ctx context.Context) (int, error) {
	items, err := w.repository.Pending(ctx, w.options.BatchSize)
	if err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	inputs := make([]string, len(items))
	for index := range items {
		inputs[index] = items[index].Input
	}
	vectors, err := w.embedder.Embed(ctx, inputs)
	if err != nil {
		_ = w.repository.Fail(ctx, items, err)
		return 0, fmt.Errorf("embed track metadata: %w", err)
	}
	if err := w.repository.Save(ctx, items, vectors); err != nil {
		_ = w.repository.Fail(ctx, items, err)
		return 0, err
	}
	return len(items), nil
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.options.Interval)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOnce(ctx); err != nil && w.options.OnError != nil {
			w.options.OnError(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
