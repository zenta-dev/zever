package memory_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/adapters/outbox/memory"
	"github.com/zenta-dev/zever/core/outbox"
)

func TestRecordCancelledContext(t *testing.T) {
	t.Parallel()

	s, err := memory.New(memory.Options{Publisher: outbox.PublisherFunc(func(context.Context, outbox.Message) error { return nil })})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = s.Record(ctx, nil, outbox.Message{ID: "1", Topic: "t"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Record(cancelled) = %v, want context.Canceled", err)
	}
}

func TestRecordPublisherError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")

	s, err := memory.New(memory.Options{Publisher: outbox.PublisherFunc(func(context.Context, outbox.Message) error {
		return sentinel
	})})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = s.Record(t.Context(), nil, outbox.Message{ID: "1", Topic: "t"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Record() = %v, want sentinel", err)
	}
}

func TestRecordStampsCreatedAt(t *testing.T) {
	t.Parallel()

	var got outbox.Message

	s, err := memory.New(memory.Options{Publisher: outbox.PublisherFunc(func(_ context.Context, m outbox.Message) error {
		got = m
		return nil
	})})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := s.Record(t.Context(), nil, outbox.Message{ID: "1", Topic: "t"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want stamped")
	}
}

func TestCloseIdempotent(t *testing.T) {
	t.Parallel()

	s, err := memory.New(memory.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}

func TestStartIdempotent(t *testing.T) {
	t.Parallel()

	s, err := memory.New(memory.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := s.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := s.Start(t.Context()); err != nil {
		t.Errorf("Start() second error = %v, want nil", err)
	}
}

func TestConcurrentRecords(t *testing.T) {
	t.Parallel()

	var count atomic.Int64

	s, err := memory.New(memory.Options{Publisher: outbox.PublisherFunc(func(context.Context, outbox.Message) error {
		count.Add(1)
		return nil
	})})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	const (
		workers = 8
		each    = 25
	)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for j := 0; j < each; j++ {
				if err := s.Record(t.Context(), nil, outbox.Message{ID: "evt", Topic: "t"}); err != nil {
					t.Errorf("Record() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	if n := count.Load(); n != workers*each {
		t.Errorf("published = %d, want %d", n, workers*each)
	}
}
