package resilience_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

var freshSeq atomic.Int64

type stubGuard struct {
	name string
	err  error
}

func (s *stubGuard) Execute(ctx context.Context, fn func(context.Context) error) error {
	if s.err != nil {
		return s.err
	}

	return fn(ctx)
}

func (s *stubGuard) State() resilience.State { return resilience.StateClosed }

func (s *stubGuard) Name() string { return s.name }

func (s *stubGuard) Close() error { return nil }

type stubManager struct {
	g resilience.Guard
}

func (m stubManager) Guard(name string) (resilience.Guard, error) {
	if m.g != nil {
		return m.g, nil
	}

	return &stubGuard{name: name}, nil
}

func (m stubManager) Close() error { return nil }

func TestDo_returnsValue(t *testing.T) {
	t.Parallel()

	g := &stubGuard{name: "dep"}

	got, err := resilience.Do(t.Context(), g, func(context.Context) (int, error) { return 42, nil })
	if err != nil {
		t.Fatalf("Do err = %v", err)
	}

	if got != 42 {
		t.Fatalf("Do = %d, want 42", got)
	}
}

func TestDo_propagatesError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	g := &stubGuard{name: "dep", err: sentinel}

	got, err := resilience.Do(t.Context(), g, func(context.Context) (int, error) { return 7, nil })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do err = %v, want sentinel", err)
	}

	if got != 0 {
		t.Fatalf("Do value = %d, want zero on error", got)
	}
}
