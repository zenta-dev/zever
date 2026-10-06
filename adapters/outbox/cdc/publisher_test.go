package cdc

import (
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestSetPublisher_attachesLatestAndIgnoresNil(t *testing.T) {
	t.Parallel()

	s := mustStore(t, mustOptions())

	if got := s.relayPublisher(); got != nil {
		t.Fatalf("relayPublisher() = %v, want nil before attach", got)
	}

	first := &stubPublisher{}
	s.SetPublisher(first)

	if got := s.relayPublisher(); got != first {
		t.Errorf("relayPublisher() = %v, want the attached publisher", got)
	}

	second := &stubPublisher{}
	s.SetPublisher(second)

	if got := s.relayPublisher(); got != second {
		t.Error("relayPublisher() did not return the most recent attach")
	}

	s.SetPublisher(nil)

	if got := s.relayPublisher(); got != second {
		t.Error("nil SetPublisher detached the publisher")
	}
}

// The publisher check runs before the replication dial, so an unwired store
// fails fast and offline instead of reporting a connection error. mustOptions
// points at an unroutable DSN on purpose: reaching the dial would prove the
// check did not run first.
func TestSetPublisher_startRejectsBeforeDialing(t *testing.T) {
	t.Parallel()

	s := mustStore(t, mustOptions())

	err := s.Start(t.Context())
	if err == nil {
		t.Fatal("Start() = nil, want error")
	}

	if !strings.Contains(err.Error(), "publisher") {
		t.Errorf("Start() error = %q, want it to name the missing publisher", err)
	}

	if strings.Contains(err.Error(), "connect") {
		t.Errorf("Start() error = %q, want the publisher check to run before the dial", err)
	}
}

// Concurrent attach + read exercises the race detector on the publisher field.
func TestSetPublisher_concurrentWithRelayRead(t *testing.T) {
	t.Parallel()

	pub := &stubPublisher{}
	s := mustStore(t, mustOptions())

	const workers = 8

	var wg sync.WaitGroup

	for range workers {
		wg.Add(2)

		go func() {
			defer wg.Done()

			s.SetPublisher(pub)
		}()

		go func() {
			defer wg.Done()

			_ = s.relayPublisher()
		}()
	}

	wg.Wait()
}

// mustOptions is the minimum valid configuration New accepts. New opens no
// connection, so no postgres server is needed.
func mustOptions() Options {
	return Options{Options: outbox.Options{DSN: "postgres://localhost/db"}}
}

// mustStore opens a store over o and closes it with the test.
func mustStore(t *testing.T, o Options) *store {
	t.Helper()

	s, err := New(o)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	store, ok := s.(*store)
	if !ok {
		t.Fatalf("New() returned %T, want *store", s)
	}

	t.Cleanup(func() { _ = s.Close() })

	return store
}
