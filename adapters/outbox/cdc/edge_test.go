package cdc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

func TestNewWhitespaceDSN(t *testing.T) {
	t.Parallel()

	_, err := New(Options{Options: outbox.Options{DSN: "   ", Slot: "s", Publication: "p", Prefix: "x"}})
	if err == nil {
		t.Fatal("New(whitespace dsn) = nil error, want error")
	}
}

func TestRecordCancelledContext(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix}
	tx := &fakeTx{}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := s.Record(ctx, tx, outbox.Message{ID: "evt-1", Topic: "orders"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Record(cancelled) = %v, want context.Canceled", err)
	}
}

func TestRecordExecError(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix}
	tx := &fakeTx{execErr: errors.New("boom")}

	err := s.Record(t.Context(), tx, outbox.Message{ID: "evt-1", Topic: "orders"})
	if err == nil {
		t.Fatal("Record() = nil error, want error")
	}

	if !strings.Contains(err.Error(), "cdc: record") {
		t.Errorf("err = %v, want containing %q", err, "cdc: record")
	}
}

func TestStatusZero(t *testing.T) {
	t.Parallel()

	if st := (&store{}).Status(); st != (outbox.Status{}) {
		t.Errorf("Status() = %+v, want zero", st)
	}
}

func TestQuoteLiteral(t *testing.T) {
	t.Parallel()

	if got := quoteLiteral("a'b"); got != "'a''b'" {
		t.Errorf("quoteLiteral() = %q, want 'a''b'", got)
	}
}

func TestSleepCtxZeroDuration(t *testing.T) {
	t.Parallel()

	if err := sleepCtx(t.Context(), 0); err != nil {
		t.Errorf("sleepCtx(0) = %v, want nil", err)
	}
}

func TestSleepCtxCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("sleepCtx(cancelled) = %v, want context.Canceled", err)
	}
}

func TestHandleMessageSleepCancelled(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, maxAttempts: 3, sleep: func(context.Context, time.Duration) error {
		return context.Canceled
	}}

	msg := outbox.Message{ID: "evt-1", Topic: "orders"}

	advanced, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), func(context.Context, outbox.Message) error {
		return errors.New("boom")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("handleMessage() = %v, want context.Canceled", err)
	}

	if advanced {
		t.Error("handleMessage() advanced = true, want false")
	}
}

func TestDecodeMessageEmpty(t *testing.T) {
	t.Parallel()

	if _, err := decodeMessage(nil); err == nil {
		t.Fatal("decodeMessage(nil) = nil error, want error")
	}
}
