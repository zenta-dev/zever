package retry

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// Policy configures exponential (or linear) backoff with jitter. Both
// NextDelay (a single delay computation, for a caller-owned retry or poll
// loop) and Do (a full bounded retry loop) are built on it.
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
	// Ignored when Linear is true.
	Multiplier float64
	// Linear switches NextDelay from exponential growth to linear growth:
	// delay = BaseDelay * attempt, still capped at MaxDelay and still
	// jittered. Multiplier is ignored when this is true. The zero value
	// (false) is the existing exponential behavior.
	Linear bool
	// Jitter is the fraction of the computed delay to randomize, e.g. 0.2
	// means up to 20% of the delay. How it is applied depends on JitterMode.
	// Zero disables jitter. Values are clamped to [0, 1].
	Jitter float64
	// JitterMode selects how Jitter is applied to the computed delay. The
	// zero value is JitterSymmetric, the existing +/- behavior.
	JitterMode JitterMode
	// JitterMax is the upper bound of a flat, delay-independent jitter
	// draw, used only when JitterMode is JitterFlat. It is a plain duration,
	// not a fraction: the returned delay is delay + rand[0, JitterMax]. Zero
	// means no jitter is added.
	JitterMax time.Duration
	// MaxAttempts is the total number of attempts Do makes, including the
	// first (not a retry count). Zero or negative means 1 (no retries). It
	// is meaningless to NextDelay, which has no notion of exhaustion -- a
	// caller that needs an unbounded poll-wait loop should call NextDelay
	// directly and ignore MaxAttempts entirely.
	MaxAttempts int
	// OnRetry, if non-nil, is called before each inter-attempt sleep that Do
	// performs, with the 1-based attempt number that just failed (matching
	// Do's internal attempt numbering: the first call is attempt 1) and the
	// error that triggered the retry. It is not called before the first
	// attempt, and not called after the final attempt (whether that attempt
	// succeeds or exhausts MaxAttempts) since no further sleep follows it.
	// It has no effect on NextDelay, which is not attempt-aware in this way.
	OnRetry func(attempt int, err error)
}

// JitterMode selects how Policy.Jitter (and, for JitterFlat, Policy.JitterMax)
// is applied to a computed delay.
type JitterMode int

const (
	// JitterSymmetric randomizes the delay by +/- the Jitter fraction,
	// returning a value in [delay*(1-Jitter), delay*(1+Jitter)]. This is the
	// zero value and the package's original jitter behavior.
	JitterSymmetric JitterMode = iota
	// JitterAdditive adds a one-sided random amount to the delay, returning
	// a value in [delay, delay+delay*Jitter]. The delay is never reduced.
	JitterAdditive
	// JitterFlat adds a one-sided random amount drawn from [0, JitterMax],
	// independent of the delay's own magnitude, returning a value in
	// [delay, delay+JitterMax]. Jitter is ignored in this mode.
	JitterFlat
)

// NextDelay returns the backoff delay before attempt (1-based: the delay
// before the first retry is NextDelay(1)). attempt values below 1 are
// treated as 1.
//
// If Linear is false (the default), it computes BaseDelay *
// Multiplier^(attempt-1). If Linear is true, it computes BaseDelay *
// attempt instead, ignoring Multiplier. Either way the result is capped at
// MaxDelay, then jitter is applied per Jitter/JitterMode/JitterMax.
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

	var delay time.Duration
	if p.Linear {
		delay = linearDelay(base, attempt)
	} else {
		delay = exponentialDelay(base, p.Multiplier, attempt)
	}

	if p.MaxDelay > 0 && delay > p.MaxDelay {
		delay = p.MaxDelay
	}

	return applyJitter(delay, p.Jitter, p.JitterMode, p.JitterMax)
}

// exponentialDelay computes base * mult^(attempt-1), the package's original
// backoff formula. Values <= 1 for mult disable growth.
func exponentialDelay(base time.Duration, mult float64, attempt int) time.Duration {
	if mult <= 1 {
		mult = 1
	}

	if mult <= 1 || base <= 0 {
		return base
	}

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
		return maxDuration
	}

	return time.Duration(scaled)
}

// linearDelay computes base * attempt, capping the multiplication so it
// cannot overflow time.Duration (int64 nanoseconds).
func linearDelay(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		return 0
	}

	scaled := float64(base) * float64(attempt)
	if scaled > float64(maxDuration) {
		return maxDuration
	}

	return time.Duration(scaled)
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

// applyJitter randomizes d according to mode. Non-positive d returns 0.
//
// JitterSymmetric (the zero value) randomizes d by +/- the fraction frac
// (clamped to [0, 1]) of d, returning a value in [d*(1-frac), d*(1+frac)].
// Non-positive frac returns d unchanged.
//
// JitterAdditive adds a uniform draw from [0, frac*d] to d, never
// subtracting, returning a value in [d, d+d*frac]. Non-positive frac returns
// d unchanged.
//
// JitterFlat adds a uniform draw from [0, max] to d, independent of d's own
// magnitude, returning a value in [d, d+max]. Non-positive max returns d
// unchanged.
func applyJitter(d time.Duration, frac float64, mode JitterMode, maxFlat time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}

	if frac > 1 {
		frac = 1
	}

	switch mode {
	case JitterAdditive:
		if frac <= 0 {
			return d
		}

		return d + randDuration(frac*float64(d))
	case JitterFlat:
		if maxFlat <= 0 {
			return d
		}

		return d + randDuration(float64(maxFlat))
	case JitterSymmetric:
		fallthrough
	default:
		if frac <= 0 {
			return d
		}

		// offset is a uniform draw from [-frac*d, +frac*d].
		span := frac * float64(d)
		offset := (randFloat64()*2 - 1) * span

		result := float64(d) + offset
		if result < 0 {
			return 0
		}

		return time.Duration(result)
	}
}

// randDuration returns a uniform random duration in [0, span].
func randDuration(span float64) time.Duration {
	if span <= 0 {
		return 0
	}

	return time.Duration(randFloat64() * span)
}

// randFloat64 returns a uniform random float64 in [0, 1). math/rand/v2's
// top-level functions are safe for concurrent use without manual seeding --
// unlike a fresh rand.New(rand.NewSource(time.Now().UnixNano())) per call,
// which both allocates a PRNG on every jitter computation and can hand
// concurrent callers landing in the same nanosecond identical seeds (and
// therefore identical "random" delays), defeating the point of jitter.
func randFloat64() float64 {
	//nolint:gosec // G404: math/rand/v2 suffices for retry jitter, which is not security-sensitive.
	return rand.Float64()
}

// Do calls fn, retrying on error up to p.MaxAttempts times total (including
// the first call), waiting p.NextDelay(attempt) between attempts via a
// ctx-aware timer. It returns nil on the first success.
//
// If p.OnRetry is non-nil, it is called with the 1-based attempt number that
// just failed and its error immediately before each inter-attempt sleep --
// not before the first attempt, and not after the final attempt (a failure
// that exhausts MaxAttempts is not followed by a sleep or an OnRetry call).
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

		if p.OnRetry != nil {
			p.OnRetry(attempt, err)
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
