package inproc_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/adapters/resilience/inproc"
	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/shared/retry"
)

func mustOpenEdge(t *testing.T, opts resilience.Options) resilience.Manager {
	t.Helper()

	m, err := inproc.New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = m.Close() })

	return m
}

func TestNewInvalidBulkhead(t *testing.T) {
	t.Parallel()

	_, err := inproc.New(resilience.Options{Bulkhead: resilience.BulkheadOptions{MaxConcurrent: -1}})
	if !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("New(invalid bulkhead) error = %v, want ErrInvalidOptions", err)
	}
}

func TestGuardAfterManagerClose(t *testing.T) {
	t.Parallel()

	m, err := inproc.New(resilience.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := m.Guard("dep"); !errors.Is(err, resilience.ErrClosed) {
		t.Fatalf("Guard(after close) error = %v, want ErrClosed", err)
	}
}

func TestExecuteCancelledContext(t *testing.T) {
	t.Parallel()

	m := mustOpenEdge(t, resilience.Options{})

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := g.Execute(ctx, func(context.Context) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute(canceled) error = %v, want context.Canceled", err)
	}
}

func TestExecuteRetryExhausted(t *testing.T) {
	t.Parallel()

	m := mustOpenEdge(t, resilience.Options{Retry: retry.Policy{MaxAttempts: 2}})

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	sentinel := errors.New("extras-edge: boom")

	err = g.Execute(t.Context(), func(context.Context) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Execute(exhausted) error = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "retry exhausted") {
		t.Fatalf("Execute(exhausted) error = %q, want retry exhausted mention", err.Error())
	}
}

func TestConcurrentExecute(t *testing.T) {
	t.Parallel()

	m := mustOpenEdge(t, resilience.Options{})
	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	const n = 16

	var wg sync.WaitGroup

	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()

			if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
				t.Errorf("Execute() error = %v", err)
			}
		}()
	}

	wg.Wait()
}

func TestConcurrentGuardSameName(t *testing.T) {
	t.Parallel()

	m := mustOpenEdge(t, resilience.Options{})

	const n = 16

	guards := make([]resilience.Guard, n)

	var wg sync.WaitGroup

	wg.Add(n)

	for i := range n {
		go func() {
			defer wg.Done()

			g, err := m.Guard("dep")
			if err != nil {
				t.Errorf("Guard() error = %v", err)
				return
			}

			guards[i] = g
		}()
	}

	wg.Wait()

	for i := 1; i < n; i++ {
		if guards[i] == nil || guards[0] == nil {
			t.Fatalf("guard[%d] is nil after concurrent Guard", i)
		}

		if guards[i] != guards[0] {
			t.Fatalf("guard[%d] != guard[0], want cached Guard", i)
		}
	}
}

func TestDoubleRegisterThenOpen(t *testing.T) {
	t.Parallel()

	inproc.Register()
	inproc.Register()

	m, err := resilience.Open(resilience.Memory, resilience.Options{})
	if err != nil {
		t.Fatalf("Open(Memory) error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
