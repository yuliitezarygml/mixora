package embedding

import (
	"context"
	"testing"
	"time"
)

type fakeRepository struct {
	pending []PendingItem
	saved   int
}

func (f *fakeRepository) Pending(context.Context, int) ([]PendingItem, error) {
	return f.pending, nil
}

func (f *fakeRepository) Save(_ context.Context, items []PendingItem, vectors [][]float32) error {
	if len(items) != len(vectors) {
		return context.Canceled
	}
	f.saved += len(items)
	return nil
}

func (f *fakeRepository) Fail(context.Context, []PendingItem, error) error { return nil }

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	result := make([][]float32, len(inputs))
	for index := range result {
		result[index] = make([]float32, 768)
	}
	return result, nil
}

func TestWorkerProcessesPendingBatch(t *testing.T) {
	repository := &fakeRepository{pending: []PendingItem{{Source: "test", ID: "1", Input: "title: one"}}}
	worker, err := NewWorker(repository, fakeEmbedder{}, WorkerOptions{BatchSize: 4, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := worker.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || repository.saved != 1 {
		t.Fatalf("processed=%d saved=%d", processed, repository.saved)
	}
}
