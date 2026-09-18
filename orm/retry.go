package orm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/retry"
)

// ErrRetryable marks an error as a transient transaction failure worth
// retrying the WHOLE transaction over. RetryTx treats it (directly or
// wrapped anywhere in the error chain) as retryable, as does IsRetryable;
// wrapping a driver/dialect error with it is how a caller retries failures
// this helper does not recognize on its own -- the dialect-agnostic escape
// hatch. It is a sentinel for errors.Is/errors.Is-based testing, so wrap
// it with %w, never build its text.
var ErrRetryable = errors.New("orm: retryable transaction error")

// IsRetryable reports whether err (or any error it wraps) is a retryable
// serialization/deadlock failure: it either wraps orm.ErrRetryable, or it
// is a Postgres error carrying SQLSTATE 40001 (serialization_failure) or
// 40P01 (deadlock_detected) -- the two SQLSTATE codes the Postgres adapter
// surfaces (via pgx) for the abort-then-retry class of failures. Other
// drivers' transient failures are recognized by wrapping ErrRetryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrRetryable) {
		return true
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01":
			return true
		}
	}

	return false
}

// RetryOptions configures RetryTx.
type RetryOptions struct {
	// MaxAttempts is the total number of transaction attempts, including
	// the first. Zero means 3 (1 initial attempt + 2 retries).
	MaxAttempts int
	// Backoff is the base delay between attempts; the wait before attempt
	// n is Backoff * n (linear backoff). Zero means 10ms. The wait is
	// context-aware and is skipped for a cancelled ctx.
	Backoff time.Duration
	// Tx carries per-attempt transaction options (e.g. an isolation level
	// like db.Serializable) forwarded to db.WithTx. May be nil.
	Tx *db.TxOptions
	// OnRetry, when non-nil, is called between attempts with the 0-based
	// attempt index of the failed attempt and the retryable error that
	// triggered the retry.
	OnRetry func(attempt int, err error)
}

// RetryTx runs fn inside a transaction and, when the transaction fails
// with a retryable error (see IsRetryable), re-runs the WHOLE transaction
// from scratch -- a new BeginTx each attempt -- up to opts.MaxAttempts
// times. It is the serialization-retry workhorse for SERIALIZABLE /
// REPEATABLE READ workloads: a serialization or deadlock failure aborts
// the entire transaction, so only a full re-run (not a savepoint) can make
// progress.
//
// Each attempt reuses the transaction plumbing: db.WithTx when fn's
// context carries no transaction yet (a real BeginTx/Commit/Rollback per
// attempt), or WithNestedTx when RetryTx is called from inside an outer
// transaction (a SAVEPOINT per attempt). Note that a genuine Postgres
// serialization failure aborts the enclosing transaction too, so nested
// retries only progress for ErrRetryable-driven (non-Postgres-abort)
// failures.
//
// A non-retryable error is returned unwrapped after exactly one attempt.
// When attempts are exhausted, the last retryable error is returned wrapped
// in an "orm: RetryTx" tag, still testable with errors.Is.
//
// The outer attempt loop and error shaping here are intentionally
// hand-rolled rather than built on retry.Do: retry.Do retries any non-nil
// error unconditionally and has no way to short-circuit on a non-retryable
// error without the exhaustion wrap, which this function's contract
// requires (see IsRetryable above). Only the per-attempt delay is delegated
// to retry.Policy (Linear: true), reproducing the original
// backoff*time.Duration(attempt+1) linear formula value-for-value.
func RetryTx(ctx context.Context, exec db.DB, opts RetryOptions, fn func(ctx context.Context, tx db.Tx) error) error {
	attempts := opts.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}

	backoff := opts.Backoff
	if backoff <= 0 {
		backoff = 10 * time.Millisecond
	}

	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		err := runTxAttempt(ctx, exec, opts, fn)
		if err == nil {
			return nil
		}

		if !IsRetryable(err) {
			return err
		}

		lastErr = err

		if attempt < attempts-1 {
			if opts.OnRetry != nil {
				opts.OnRetry(attempt, err)
			}

			delay := retry.Policy{BaseDelay: backoff, Linear: true}.NextDelay(attempt + 1)
			if err := sleepCtx(ctx, delay); err != nil {
				return fmt.Errorf("orm: RetryTx: %w", err)
			}
		}
	}

	return fmt.Errorf("orm: RetryTx: exhausted %d attempts: %w", attempts, lastErr)
}

// runTxAttempt runs one fn attempt inside a transaction, reusing the same
// plumbing WithNestedTx uses: a real db.WithTx transaction when ctx carries
// no transaction yet, a SAVEPOINT (WithNestedTx) when it already does.
func runTxAttempt(ctx context.Context, exec db.DB, opts RetryOptions, fn func(ctx context.Context, tx db.Tx) error) error {
	if _, ok := db.TxFromContext(ctx); ok {
		return WithNestedTx(ctx, exec, fn)
	}

	return db.WithTx(ctx, exec, opts.Tx, fn)
}

// sleepCtx waits d or until ctx is done, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
