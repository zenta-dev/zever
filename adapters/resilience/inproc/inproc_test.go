package inproc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/resilience/inproc"
	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/core/resilience/resiliencetest"
)

func openManager(t *testing.T, opts resilience.Options) resilience.Manager {
	t.Helper()

	m, err := inproc.New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = m.Close() })

	return m
}

// TestInprocConformance proves the in-process adapter honors the resilience
// contract via the shared conformance kit.
func TestInprocConformance(t *testing.T) {
	resiliencetest.Conformance(t, openManager)
}

func TestRegister_thenOpen(t *testing.T) {
	t.Parallel()

	inproc.Register()

	m, err := resilience.Open(resilience.Memory, resilience.Options{})
	if err != nil {
		t.Fatalf("Open(Memory) error = %v", err)
	}
	defer func() { _ = m.Close() }()

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()

	_, err := inproc.New(resilience.Options{Timeout: -1})
	if !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("New(invalid) error = %v, want ErrInvalidOptions", err)
	}
}

func TestGuard_emptyName(t *testing.T) {
	t.Parallel()

	m := openManager(t, resilience.Options{})

	if _, err := m.Guard(""); !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("Guard(empty) error = %v, want ErrInvalidOptions", err)
	}
}

func TestGuard_sameNameCached(t *testing.T) {
	t.Parallel()

	m := openManager(t, resilience.Options{})

	first, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	second, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard(same) error = %v", err)
	}

	if first != second {
		t.Error("Guard(same) returned a different Guard, want the cached one")
	}

	if first.Name() != "dep" {
		t.Errorf("Name() = %q, want dep", first.Name())
	}
}

func TestOnStateChange(t *testing.T) {
	t.Parallel()

	var (
		froms []resilience.State
		tos   []resilience.State
	)

	m := openManager(t, resilience.Options{
		Breaker: resilience.BreakerOptions{
			Enabled:      true,
			MinRequests:  1,
			FailureRatio: 0.1,
			Timeout:      time.Second,
		},
		OnStateChange: func(_ string, from, to resilience.State) {
			froms = append(froms, from)
			tos = append(tos, to)
		},
	})

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return errors.New("boom") }); err == nil {
		t.Fatal("Execute(fail) error = nil, want failure")
	}

	if len(tos) != 1 {
		t.Fatalf("state changes = %d, want 1", len(tos))
	}

	if froms[0] != resilience.StateClosed || tos[0] != resilience.StateOpen {
		t.Fatalf("transition = %s->%s, want closed->open", froms[0], tos[0])
	}
}

func TestGuard_stateNoBreaker_closed(t *testing.T) {
	t.Parallel()

	m := openManager(t, resilience.Options{})

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if g.State() != resilience.StateClosed {
		t.Errorf("State() = %s, want closed", g.State())
	}
}
