package secretstest_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/adapters/secrets/env"
	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/secrets/secretstest"
)

var envConformanceSeq atomic.Int64

// TestConformanceEnv proves the kit passes against the read-only env adapter.
// Each subtest gets a fresh prefix/window into the process environment seeded
// with one key, so cases stay isolated. Not parallel: the factory seeds via
// t.Setenv, which forbids parallel ancestors.
func TestConformanceEnv(t *testing.T) {
	secretstest.Conformance(t, func(t *testing.T) secrets.Secrets {
		t.Helper()

		prefix := fmt.Sprintf("CONFTEST_%d_", envConformanceSeq.Add(1))
		t.Setenv(prefix+"SEEDED", "1")

		s, err := env.New(env.Options{Prefix: prefix})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close(t.Context()) })

		return s
	})
}

type mapStub struct {
	mu    sync.Mutex
	store map[string][]byte
}

func newMapStub() *mapStub {
	return &mapStub{store: map[string][]byte{}}
}

func (m *mapStub) Get(_ context.Context, name string) ([]byte, error) {
	if err := secrets.ValidateName(name); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	val, ok := m.store[name]
	if !ok {
		return nil, fmt.Errorf("stub: %w: %s", secrets.ErrNotFound, name)
	}

	out := make([]byte, len(val))
	copy(out, val)

	return out, nil
}

func (m *mapStub) Set(_ context.Context, name string, value []byte) error {
	if err := secrets.ValidateName(name); err != nil {
		return err
	}

	cp := make([]byte, len(value))
	copy(cp, value)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[name] = cp

	return nil
}

func (m *mapStub) Delete(_ context.Context, name string) error {
	if err := secrets.ValidateName(name); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.store[name]; !ok {
		return fmt.Errorf("stub: %w: %s", secrets.ErrNotFound, name)
	}

	delete(m.store, name)

	return nil
}

func (m *mapStub) List(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys := make([]string, 0, len(m.store))
	for k := range m.store {
		keys = append(keys, k)
	}

	return keys, nil
}

func (m *mapStub) Close(_ context.Context) error { return nil }

var _ secrets.Secrets = (*mapStub)(nil)

// TestConformanceWritable proves the kit passes against a writable stub,
// covering the Set/Get round-trip, List-contains, and Delete-then-NotFound
// branches the read-only env adapter skips via ErrNotSupported.
func TestConformanceWritable(t *testing.T) {
	t.Parallel()

	secretstest.Conformance(t, func(t *testing.T) secrets.Secrets {
		t.Helper()

		s := newMapStub()
		t.Cleanup(func() { _ = s.Close(t.Context()) })

		return s
	})
}
