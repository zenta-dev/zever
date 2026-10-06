package inproc

import (
	"context"
	"errors"

	"github.com/sony/gobreaker/v2"

	"github.com/zenta-dev/zever/core/resilience"
)

// breakerSettings translates resilience breaker options into gobreaker
// settings. Cancellation errors are excluded so caller cancellation never
// counts against the circuit; nil errors are always successful.
func breakerSettings(name string, b resilience.BreakerOptions, onStateChange func(string, resilience.State, resilience.State)) gobreaker.Settings {
	return gobreaker.Settings{
		Name:         name,
		MaxRequests:  b.MaxRequests,
		Interval:     b.Interval,
		BucketPeriod: b.BucketPeriod,
		Timeout:      b.Timeout,
		ReadyToTrip:  readyToTrip(b),
		OnStateChange: func(_ string, from, to gobreaker.State) {
			if onStateChange != nil {
				onStateChange(name, stateFromBreaker(from), stateFromBreaker(to))
			}
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}

			if b.IsSuccessful != nil {
				return b.IsSuccessful(err)
			}

			return false
		},
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		},
	}
}

// readyToTrip builds the trip predicate from the ratio and consecutive-failure
// thresholds. Either threshold alone can open the breaker.
func readyToTrip(b resilience.BreakerOptions) func(gobreaker.Counts) bool {
	return func(c gobreaker.Counts) bool {
		if b.ConsecutiveFailures > 0 && int64(c.ConsecutiveFailures) >= int64(b.ConsecutiveFailures) {
			return true
		}

		if b.MinRequests > 0 && int64(c.Requests) >= int64(b.MinRequests) && b.FailureRatio > 0 {
			return float64(c.TotalFailures)/float64(c.Requests) >= b.FailureRatio
		}

		return false
	}
}

// stateFromBreaker maps a gobreaker state onto the battery State.
func stateFromBreaker(s gobreaker.State) resilience.State {
	switch s {
	case gobreaker.StateClosed:
		return resilience.StateClosed
	case gobreaker.StateOpen:
		return resilience.StateOpen
	case gobreaker.StateHalfOpen:
		return resilience.StateHalfOpen
	default:
		return resilience.StateClosed
	}
}

// mapBreakerError maps gobreaker sentinels onto the battery sentinels so
// callers can match with errors.Is. Other errors pass through unchanged.
func mapBreakerError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gobreaker.ErrOpenState):
		return resilience.ErrOpenState
	case errors.Is(err, gobreaker.ErrTooManyRequests):
		return resilience.ErrTooManyRequests
	default:
		return err
	}
}
