package dialect

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type mockDialect struct{}

func (mockDialect) Name() string               { return "mock" }
func (mockDialect) Placeholder(int) string     { return "$?" }
func (mockDialect) QuoteIdent(s string) string { return "[" + s + "]" }

var registerSuffix atomic.Int64

// TestFor_known_dialects_resolve verifies the in-tree registry entries.
func TestFor_known_dialects_resolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "sqlite", want: "sqlite"},
		{name: "postgres", want: "postgres"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := For(tt.name)
			if err != nil {
				t.Fatalf("For(%q) error: %v", tt.name, err)
			}

			if got := d.Name(); got != tt.want {
				t.Fatalf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFor_unknown_wraps_sentinel verifies the unknown-name error shape.
func TestFor_unknown_wraps_sentinel(t *testing.T) {
	t.Parallel()

	_, err := For("no-such-dialect-xyz")
	if err == nil {
		t.Fatal("For(unknown) = nil error, want an error")
	}

	if !errors.Is(err, ErrUnknownDialect) {
		t.Fatalf("For(unknown) error = %v, want it to wrap ErrUnknownDialect", err)
	}

	if !strings.HasPrefix(err.Error(), "orm/dialect:") {
		t.Fatalf("For(unknown) error = %q, want orm/dialect: prefix", err)
	}
}

// TestRegister_rejects_empty_name verifies the empty-name error shape.
func TestRegister_rejects_empty_name(t *testing.T) {
	t.Parallel()

	err := Register("", func() Dialect { return mockDialect{} })
	if err == nil {
		t.Fatal("Register(empty) = nil error, want an error")
	}

	if !errors.Is(err, ErrEmptyName) {
		t.Fatalf("Register(empty) error = %v, want it to wrap ErrEmptyName", err)
	}

	if !strings.HasPrefix(err.Error(), "orm/dialect:") {
		t.Fatalf("Register(empty) error = %q, want orm/dialect: prefix", err)
	}
}

// TestRegister_rejects_nil_factory verifies the nil-factory error shape.
func TestRegister_rejects_nil_factory(t *testing.T) {
	t.Parallel()

	err := Register("nil-factory-dialect", nil)
	if err == nil {
		t.Fatal("Register(nil factory) = nil error, want an error")
	}

	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register(nil factory) error = %v, want it to wrap ErrNilFactory", err)
	}

	if !strings.HasPrefix(err.Error(), "orm/dialect:") {
		t.Fatalf("Register(nil factory) error = %q, want orm/dialect: prefix", err)
	}
}

// TestRegister_rejects_duplicate verifies the duplicate-name error shape.
func TestRegister_rejects_duplicate(t *testing.T) {
	t.Parallel()

	name := fmt.Sprintf("test-dup-%d", registerSuffix.Add(1))

	if err := Register(name, func() Dialect { return mockDialect{} }); err != nil {
		t.Fatalf("first Register(%q) error: %v", name, err)
	}

	err := Register(name, func() Dialect { return mockDialect{} })
	if err == nil {
		t.Fatalf("second Register(%q) = nil error, want an error", name)
	}

	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Register error = %v, want it to wrap ErrDuplicate", err)
	}

	if !strings.Contains(err.Error(), name) {
		t.Fatalf("second Register error = %q, want it to name %q", err, name)
	}
}

// TestRegister_then_For_resolves verifies a registered factory round-trips.
func TestRegister_then_For_resolves(t *testing.T) {
	t.Parallel()

	name := fmt.Sprintf("test-mock-%d", registerSuffix.Add(1))

	if err := Register(name, func() Dialect { return mockDialect{} }); err != nil {
		t.Fatalf("Register(%q) error: %v", name, err)
	}

	d, err := For(name)
	if err != nil {
		t.Fatalf("For(%q) error: %v", name, err)
	}

	if d.Name() != "mock" {
		t.Fatalf("For(%q).Name() = %q, want mock", name, d.Name())
	}
}

// TestFor_concurrent_reads verifies the RLock fast path is race-safe.
func TestFor_concurrent_reads(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for range 16 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for _, name := range []string{"sqlite", "postgres"} {
				d, err := For(name)
				if err != nil {
					t.Errorf("For(%q) error: %v", name, err)
					return
				}

				if d.Name() != name {
					t.Errorf("Name() = %q, want %q", d.Name(), name)
				}
			}
		}()
	}

	wg.Wait()
}
