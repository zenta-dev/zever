package resilience_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

func TestParseAdapter(t *testing.T) {
	t.Parallel()

	if _, err := resilience.ParseAdapter(""); !errors.Is(err, resilience.ErrInvalidAdapter) {
		t.Fatalf("ParseAdapter(empty) err = %v, want ErrInvalidAdapter", err)
	}

	got, err := resilience.ParseAdapter("custom")
	if err != nil {
		t.Fatalf("ParseAdapter(custom) err = %v", err)
	}

	if got != resilience.Adapter("custom") {
		t.Fatalf("ParseAdapter(custom) = %v, want custom", got)
	}
}

func TestAdapter_String(t *testing.T) {
	t.Parallel()

	if got := resilience.Memory.String(); got != "memory" {
		t.Errorf("Memory.String() = %q, want memory", got)
	}

	if got := resilience.Redis.String(); got != "redis" {
		t.Errorf("Redis.String() = %q, want redis", got)
	}

	if got := resilience.Adapter("").String(); got != "unknown" {
		t.Errorf("empty String() = %q, want unknown", got)
	}
}

func TestTypedErrorStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"duplicate", (resilience.DuplicateError{Adapter: resilience.Redis}).Error(), "resilience: duplicate adapter: redis"},
		{"unknown", (resilience.UnknownAdapterError{Adapter: resilience.Memory}).Error(), "resilience: unknown adapter: memory (forgotten import?)"},
		{"invalid_adapter", (resilience.InvalidAdapterError{Adapter: "bogus"}).Error(), `resilience: invalid adapter: "bogus"`},
		{"invalid_options", (resilience.InvalidOptionsError{Reason: "timeout must be >= 0"}).Error(), "resilience: invalid options: timeout must be >= 0"},
	}

	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s Error() = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestState_constants(t *testing.T) {
	t.Parallel()

	if resilience.StateClosed != "closed" || resilience.StateOpen != "open" || resilience.StateHalfOpen != "half-open" {
		t.Fatalf("state constants = %q, %q, %q", resilience.StateClosed, resilience.StateOpen, resilience.StateHalfOpen)
	}
}

func TestOpen_invalidOptions(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := resilience.Register(a, func(resilience.Options) (resilience.Manager, error) { return stubManager{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	_, err := resilience.Open(a, resilience.Options{Timeout: -1})
	if !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("Open(invalid) err = %v, want ErrInvalidOptions", err)
	}
}

func TestRegister_Open_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16

	var wg sync.WaitGroup

	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()

			a := freshAdapter()
			if err := resilience.Register(a, func(resilience.Options) (resilience.Manager, error) { return stubManager{}, nil }); err != nil {
				t.Errorf("Register(%v) err = %v", a, err)
				return
			}

			m, err := resilience.Open(a, resilience.Options{})
			if err != nil {
				t.Errorf("Open(%v) err = %v", a, err)
				return
			}

			g, err := m.Guard("dep")
			if err != nil {
				t.Errorf("Guard(%v) err = %v", a, err)
				return
			}

			if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
				t.Errorf("Execute(%v) err = %v", a, err)
			}
		}()
	}

	wg.Wait()
}
