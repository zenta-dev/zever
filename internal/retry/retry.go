package retry

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// Policy configures exponential backoff with jitter. Both NextDelay (a
// single delay computation, for a caller-owned retry or poll loop) and Do (a
// full bounded retry loop) are built on it.
type Policy struct {
	// BaseDelay is the delay before the first retry (attempt 1). Non-positive
	// values are treated as zero delay.
	BaseDelay time.Duration
	// MaxDelay caps the computed delay, before jitter is applied. Zero or
	// negative means unbounded.
	MaxDelay time.Duration
	// Multiplier scales BaseDelay per attempt: delay = BaseDelay *
	// Multiplier^(attempt-1). Values <= 1 disable growth (delay stays at
	// BaseDelay every attempt, still capped by MaxDelay and still jittered).
	Multiplier float64
	// Jitter is the +/- fraction of the computed delay to randomize, e.g. 0.2
	// means the returned delay is within [delay*0.8, delay*1.2]. Zero
	// disables jitter. Values are clamped to [0, 1].
	Jitter float64
	// MaxAttempts is the total number of attempts Do makes, including the
	// first (not a retry count). Zero or negative means 1 (no retries). It
	// is meaningless to NextDelay, which has no notion of exhaustion -- a
	// caller that needs an unbounded poll-wait loop should call NextDelay
	// directly and ignore MaxAttempts entirely.
	MaxAttempts int
}

// NextDelay returns the backoff delay before attempt (1-based: the delay
// before the first retry is NextDelay(1)). It computes BaseDelay *
// Multiplier^(attempt-1), caps it at MaxDelay, then applies jitter as a
// random +/-Jitter fraction of that capped value. attempt values below 1 are
// treated as 1.
//
// NextDelay is also the poll-wait primitive: a caller with no fixed retry
// count (e.g. an idle-queue poll loop) can call NextDelay(attempt) with an
// ever-increasing attempt counter, ignoring Do and MaxAttempts entirely.
func (p Policy) NextDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	base := p.BaseDelay
	if base < 0 {
		base = 0
	}

	mult := p.Multiplier
	if mult <= 1 {
		mult = 1
	}

	delay := base
	if mult > 1 && base > 0 {
		// Cap the exponent so the multiplication below cannot overflow
		// time.Duration (int64 nanoseconds); anything past this shift is
		// already far beyond any realistic MaxDelay.
		const maxShift = 62

		shift := attempt - 1
		if shift > maxShift {
			shift = maxShift
		}

		scaled := float64(base) * pow(mult, shift)
		if scaled > float64(maxDuration) {
			delay = maxDuration
		} else {
			delay = time.Duration(scaled)
		}
	}

	if p.MaxDelay > 0 && delay > p.MaxDelay {
		delay = p.MaxDelay
	}

	return applyJitter(delay, p.Jitter)
}

// maxDuration is the largest representable time.Duration.
const maxDuration = time.Duration(1<<63 - 1)

// pow computes base^exp for a non-negative integer exponent.
func pow(base float64, exp int) float64 {
	result := 1.0
	for i := 0; i < exp; i++ {
		result *= base
	}

	return result
}

// applyJitter randomizes d by +/- the fraction frac (clamped to [0, 1]) of
// d, returning a value in [d*(1-frac), d*(1+frac)]. Non-positive d or frac
// returns d unchanged (clamped to non-negative).
func applyJitter(d time.Duration, frac float64) time.Duration {
	if d <= 0 {
		return 0
	}

	if frac <= 0 {
		return d
	}

	if frac > 1 {
		frac = 1
	}

	//nolint:gosec // G404: math/rand suffices for retry jitter, which is not security-sensitive.
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// offset is a uniform draw from [-frac*d, +frac*d].
	span := frac * float64(d)
	offset := (r.Float64()*2 - 1) * span

	result := float64(d) + offset
	if result < 0 {
		return 0
	}

	return time.Duration(result)
}

// Do calls fn, retrying on error up to p.MaxAttempts times total (including
// the first call), waiting p.NextDelay(attempt) between attempts via a
// ctx-aware timer. It returns nil on the first success.
//
// If ctx is cancelled -- either before an attempt or during the inter-attempt
// sleep -- Do returns ctx.Err() immediately without making a further
// attempt. If every attempt fails, Do returns an error wrapping both the
// attempt count and the last error, still unwrappable to that last error via
// errors.Is/errors.As.
func Do(ctx context.Context, p Policy, fn func(ctx context.Context) error) error {
	attempts := p.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if attempt == attempts {
			break
		}

		if err := sleepCtx(ctx, p.NextDelay(attempt)); err != nil {
			return err
		}
	}

	return fmt.Errorf("retry: exhausted %d attempts: %w", attempts, lastErr)
}

// sleepCtx waits d or until ctx is done, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		if err := ctx.Err(); err != nil {
			return err
		}

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
