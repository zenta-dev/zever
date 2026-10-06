package cdc

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestCloseIdempotent(t *testing.T) {
	t.Parallel()

	s := &store{}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() first error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() second error = %v, want nil (idempotent)", err)
	}
}

func TestStartWithoutPublisher(t *testing.T) {
	t.Parallel()

	s := &store{dsn: "postgres://localhost/db"}

	err := s.Start(t.Context())
	if err == nil {
		t.Fatal("Start() = nil error, want publisher error")
	}

	if !strings.Contains(err.Error(), "publisher") {
		t.Errorf("Start() error = %v, want containing %q", err, "publisher")
	}

	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Errorf("Start() error = %v, want wrapping ErrInvalidOptions", err)
	}
}

func TestStartAfterClose(t *testing.T) {
	t.Parallel()

	s := &store{dsn: "postgres://localhost/db"}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Start(t.Context()); !errors.Is(err, ErrClosed) {
		t.Errorf("Start() after Close error = %v, want ErrClosed", err)
	}
}

func TestRecordNilTx(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix}

	err := s.Record(t.Context(), nil, outbox.Message{ID: "evt-1", Topic: "orders"})
	if !errors.Is(err, outbox.ErrTxRequired) {
		t.Fatalf("Record(nil tx) error = %v, want ErrTxRequired", err)
	}
}
