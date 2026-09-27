package orm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenta-dev/zever/core/db"
)

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain", errors.New("boom"), false},
		{"wrapped sentinel", fmt.Errorf("transient: %w", ErrRetryable), true},
		{"bare sentinel", ErrRetryable, true},
		{"pg serialization 40001", &pgconn.PgError{Code: "40001"}, true},
		{"pg deadlock 40P01", &pgconn.PgError{Code: "40P01"}, true},
		{"pg other code", &pgconn.PgError{Code: "23505"}, false},
		{"pg code wrapped by db commit", fmt.Errorf("commit error: %w", &pgconn.PgError{Code: "40001"}), true},
		{"pg code wrapped by orm", fmt.Errorf("orm: Query.All: %w", &pgconn.PgError{Code: "40P01"}), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRetryable(tc.err); got != tc.want {
				t.Fatalf("IsRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRetryTxRetriesThenSucceeds(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	attempts := 0

	err := RetryTx(ctx, conn, RetryOptions{MaxAttempts: 3, Backoff: time.Millisecond}, func(ctx context.Context, tx db.Tx) error {
		attempts++
		if attempts < 2 {
			return fmt.Errorf("%w: transient failure %d", ErrRetryable, attempts)
		}

		return renameWidget(ctx, tx, "w1", "Retried")
	})
	if err != nil {
		t.Fatalf("RetryTx: %v", err)
	}

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}

	row, ok, err := From(widgets).Where(widgetID.Eq("w1")).First(ctx, conn)
	if err != nil || !ok || row.Name != "Retried" {
		t.Fatalf("row after retry = %+v, ok=%v, err=%v, want Retried", row, ok, err)
	}
}

func TestRetryTxNoRetryOnNonRetryable(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	sentinel := errors.New("boom")
	attempts := 0

	err := RetryTx(ctx, conn, RetryOptions{MaxAttempts: 5, Backoff: time.Millisecond}, func(_ context.Context, _ db.Tx) error {
		attempts++

		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on non-retryable)", attempts)
	}
}

func TestRetryTxExhausts(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	attempts := 0

	err := RetryTx(ctx, conn, RetryOptions{MaxAttempts: 3, Backoff: time.Millisecond}, func(_ context.Context, _ db.Tx) error {
		attempts++

		return fmt.Errorf("%w: always transient", ErrRetryable)
	})
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("err = %v, want it to wrap ErrRetryable", err)
	}
}

// TestRetryTxRollsBackBetweenAttempts proves each retry re-runs the WHOLE
// transaction: the write of the failed attempt is rolled back, so the final
// state reflects only the successful attempt.
func TestRetryTxRollsBackBetweenAttempts(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	attempts := 0

	err := RetryTx(ctx, conn, RetryOptions{MaxAttempts: 2, Backoff: time.Millisecond}, func(ctx context.Context, tx db.Tx) error {
		attempts++

		if err := renameWidget(ctx, tx, "w2", fmt.Sprintf("attempt%d", attempts)); err != nil {
			return err
		}

		if attempts < 2 {
			return fmt.Errorf("%w: transient after write", ErrRetryable)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("RetryTx: %v", err)
	}

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}

	row, ok, err := From(widgets).Where(widgetID.Eq("w2")).First(ctx, conn)
	if err != nil || !ok || row.Name != "attempt2" {
		t.Fatalf("row = %+v, ok=%v, err=%v, want attempt2 (attempt1 write rolled back)", row, ok, err)
	}
}

// TestRetryTxInsideOuterTx proves RetryTx degrades to savepoints inside an
// existing transaction (via WithNestedTx) and still retries.
func TestRetryTxInsideOuterTx(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	err := WithNestedTx(ctx, conn, func(outerCtx context.Context, outerTx db.Tx) error {
		attempts := 0

		return RetryTx(outerCtx, conn, RetryOptions{MaxAttempts: 3, Backoff: time.Millisecond}, func(_ context.Context, innerTx db.Tx) error {
			attempts++
			if innerTx != outerTx {
				t.Fatalf("RetryTx inside an outer tx did not degrade to savepoint")
			}

			if attempts < 2 {
				return fmt.Errorf("%w: transient", ErrRetryable)
			}

			return nil
		})
	})
	if err != nil {
		t.Fatalf("RetryTx: %v", err)
	}
}

func TestRetryTxDefaultMaxAttempts(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	attempts := 0

	err := RetryTx(ctx, conn, RetryOptions{Backoff: time.Millisecond}, func(_ context.Context, _ db.Tx) error {
		attempts++

		return fmt.Errorf("%w: always transient", ErrRetryable)
	})
	if err == nil {
		t.Fatalf("want error after exhausting default attempts")
	}

	if attempts != 3 {
		t.Fatalf("attempts = %d, want default 3", attempts)
	}
}

func TestRetryTxOnRetryCallback(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var retried []int
	attempts := 0

	err := RetryTx(ctx, conn, RetryOptions{
		MaxAttempts: 3,
		Backoff:     time.Millisecond,
		OnRetry: func(attempt int, _ error) {
			retried = append(retried, attempt)
		},
	}, func(_ context.Context, _ db.Tx) error {
		attempts++
		if attempts < 2 {
			return fmt.Errorf("%w: transient", ErrRetryable)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("RetryTx: %v", err)
	}

	if len(retried) != 1 || retried[0] != 0 {
		t.Fatalf("OnRetry calls = %v, want [0]", retried)
	}
}

func TestRetryTxStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_, conn := newWidgetsDB(t)

	attempts := 0

	// The fn cancels the context and returns a retryable error; the retry
	// backoff must observe the cancellation and abort instead of sleeping.
	err := RetryTx(ctx, conn, RetryOptions{MaxAttempts: 5, Backoff: 10 * time.Second}, func(_ context.Context, _ db.Tx) error {
		attempts++
		if attempts == 1 {
			cancel()
		}

		return fmt.Errorf("%w: transient", ErrRetryable)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled (backoff aborted by cancellation)", err)
	}

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no second attempt after cancellation)", attempts)
	}
}

// recordingTransactor wraps a db.DB purely to capture the *db.TxOptions
// passed to BeginTx; every other method is the embedded (unused) DB.
type recordingTransactor struct {
	db.DB
	mu       sync.Mutex
	lastOpts *db.TxOptions
}

func (r *recordingTransactor) BeginTx(_ context.Context, opts *db.TxOptions) (db.Tx, error) {
	r.mu.Lock()
	r.lastOpts = opts
	r.mu.Unlock()

	return nil, errors.New("begin fails")
}

// TestRetryTxForwardsTxOptions proves RetryOptions.Tx reaches db.WithTx,
// so a caller can run attempts at e.g. db.Serializable isolation.
func TestRetryTxForwardsTxOptions(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rec := &recordingTransactor{DB: conn}

	opts := &db.TxOptions{Isolation: db.Serializable, ReadOnly: true}

	err := RetryTx(ctx, rec, RetryOptions{Tx: opts}, func(_ context.Context, _ db.Tx) error { return nil })
	if err == nil {
		t.Fatalf("want error from BeginTx, got nil")
	}

	rec.mu.Lock()
	got := rec.lastOpts
	rec.mu.Unlock()

	if got != opts || got.Isolation != db.Serializable || !got.ReadOnly {
		t.Fatalf("BeginTx opts = %+v, want %+v", got, opts)
	}
}

// TestSleepCtx pins the backoff sleeper: non-positive waits return
// immediately, a cancelled context aborts the wait, and a normal wait
// elapses.
func TestSleepCtx(t *testing.T) {
	if err := sleepCtx(t.Context(), 0); err != nil {
		t.Fatalf("sleepCtx(0) = %v, want nil", err)
	}

	if err := sleepCtx(t.Context(), -time.Second); err != nil {
		t.Fatalf("sleepCtx(-1s) = %v, want nil", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("sleepCtx(cancelled) = %v, want context.Canceled", err)
	}

	start := time.Now()
	if err := sleepCtx(t.Context(), 5*time.Millisecond); err != nil {
		t.Fatalf("sleepCtx(5ms) = %v, want nil", err)
	}

	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("sleepCtx(5ms) returned early")
	}
}
