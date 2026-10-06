package ratelimittest

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/ratelimit"
)

// stubLimiter is a scriptable token-bucket ratelimit.Limiter double
// enforcing the documented contract by default. Override fields drive
// each conformance failure branch.
type stubLimiter struct {
	mu              sync.Mutex
	name            string
	burst           float64
	remaining       map[string]float64
	allowErr        error
	denyOnFirst     bool
	neverDeny       bool
	overAllowed     bool
	retryAfter      time.Duration
	denyRemaining   float64
	otherErr        error
	otherDenied     bool
	resetErr        error
	resetNoRestore  bool
	resetMissingErr error
	invalidBypass   bool
	closeErr        error
	closeAgainErr   error
	allowAfterClose bool
	resetAfterClose bool
	closed          bool
	closes          int
	charges         int
	denied          bool
}

func healthyStubLimiter() *stubLimiter {
	return &stubLimiter{
		name:          "stub",
		burst:         3,
		remaining:     make(map[string]float64),
		retryAfter:    time.Second,
		denyRemaining: 0,
	}
}

func (s *stubLimiter) Name() string { return s.name }

func (s *stubLimiter) Allow(_ context.Context, key string, tokens float64) (ratelimit.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.allowErr != nil {
		return ratelimit.Decision{}, s.allowErr
	}

	if s.closed && !s.allowAfterClose {
		return ratelimit.Decision{}, ratelimit.ErrClosed
	}

	if !s.invalidBypass {
		if key == "" || len(key) > ratelimit.MaxKeyLen {
			return ratelimit.Decision{}, ratelimit.ErrInvalidKey
		}

		if math.IsNaN(tokens) || math.IsInf(tokens, 0) || tokens <= 0 {
			return ratelimit.Decision{}, ratelimit.ErrInvalidCost
		}
	}

	if key == "kit-other" {
		if s.otherErr != nil {
			return ratelimit.Decision{}, s.otherErr
		}

		if s.otherDenied {
			return ratelimit.Decision{Allowed: false, RetryAfter: s.retryAfter, Remaining: s.denyRemaining}, nil
		}

		return ratelimit.Decision{Allowed: true, Remaining: s.burst}, nil
	}

	if s.neverDeny {
		return ratelimit.Decision{Allowed: true, Remaining: s.burst}, nil
	}

	s.charges++

	if s.overAllowed && s.denied {
		return ratelimit.Decision{Allowed: true, Remaining: s.burst}, nil
	}

	if s.denyOnFirst && s.charges == 1 {
		s.denied = true

		return ratelimit.Decision{Allowed: false, RetryAfter: s.retryAfter, Remaining: s.denyRemaining}, nil
	}

	left, ok := s.remaining[key]
	if !ok {
		left = s.burst
	}

	if left < tokens {
		s.denied = true

		return ratelimit.Decision{Allowed: false, RetryAfter: s.retryAfter, Remaining: s.denyRemaining}, nil
	}

	left -= tokens
	s.remaining[key] = left

	return ratelimit.Decision{Allowed: true, Remaining: left}, nil
}

func (s *stubLimiter) Reset(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed && !s.resetAfterClose {
		return ratelimit.ErrClosed
	}

	if s.resetMissingErr != nil && key == "never-seen" {
		return s.resetMissingErr
	}

	if s.resetErr != nil {
		return s.resetErr
	}

	if !s.invalidBypass && key == "" {
		return ratelimit.ErrInvalidKey
	}

	if !s.resetNoRestore {
		delete(s.remaining, key)
	}

	return nil
}

func (s *stubLimiter) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closes++
	if s.closes > 1 {
		return s.closeAgainErr
	}

	if s.closeErr != nil {
		return s.closeErr
	}

	s.closed = true

	return nil
}

func mustPass(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("check err = %v, want nil", err)
	}
}

func TestCheckOpenRegister_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkOpenRegister())
}

func TestCheckAllowDeny_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkAllowDeny(t.Context(), healthyStubLimiter()))
}

func TestCheckAllowDeny_failures(t *testing.T) {
	t.Parallel()

	boom := errors.New("ratelimittest: boom")

	cases := []struct {
		name     string
		mutate   func(*stubLimiter)
		contains string
	}{
		{"empty name", func(s *stubLimiter) { s.name = "" }, "Name() is empty"},
		{"allow error", func(s *stubLimiter) { s.allowErr = boom }, "Allow() error"},
		{"never denies", func(s *stubLimiter) { s.neverDeny = true }, "never denied after 8192"},
		{"deny on first", func(s *stubLimiter) { s.denyOnFirst = true }, "denied on a fresh bucket"},
		{"zero retry after", func(s *stubLimiter) { s.retryAfter = 0 }, "RetryAfter"},
		{"negative remaining", func(s *stubLimiter) { s.denyRemaining = -1 }, "Remaining"},
		{"other error", func(s *stubLimiter) { s.otherErr = boom }, "Allow(other) error"},
		{"other denied", func(s *stubLimiter) { s.otherDenied = true }, "per-key isolation"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubLimiter()
			tc.mutate(stub)

			err := checkAllowDeny(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkAllowDeny() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkAllowDeny() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckAllowDeny_allowErrorWraps(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("ratelimittest: boom")
	stub := healthyStubLimiter()
	stub.allowErr = sentinel

	if err := checkAllowDeny(t.Context(), stub); !errors.Is(err, sentinel) {
		t.Fatalf("checkAllowDeny() = %v, want wrap of sentinel", err)
	}
}

func TestCheckReset_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkReset(t.Context(), healthyStubLimiter()))
}

func TestCheckReset_failures(t *testing.T) {
	t.Parallel()

	boom := errors.New("ratelimittest: boom")

	cases := []struct {
		name     string
		mutate   func(*stubLimiter)
		contains string
	}{
		{"allow error", func(s *stubLimiter) { s.allowErr = boom }, "Allow() error"},
		{"never denies", func(s *stubLimiter) { s.neverDeny = true }, "never denied"},
		{"over allowed", func(s *stubLimiter) { s.overAllowed = true }, "Allow(over)"},
		{"reset error", func(s *stubLimiter) { s.resetErr = boom }, "Reset() error"},
		{"no restore", func(s *stubLimiter) { s.resetNoRestore = true }, "Allow(after reset)"},
		{"missing error", func(s *stubLimiter) { s.resetMissingErr = boom }, "Reset(missing)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubLimiter()
			tc.mutate(stub)

			err := checkReset(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkReset() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkReset() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckInvalidInput_reportsAll(t *testing.T) {
	t.Parallel()

	stub := healthyStubLimiter()
	stub.invalidBypass = true

	err := checkInvalidInput(t.Context(), stub)
	if err == nil {
		t.Fatal("checkInvalidInput(bypass stub) = nil, want joined errors")
	}

	for _, want := range []string{"Allow(empty key)", "Allow(long key)", "Allow(tokens=0)", "Allow(tokens=-1)", "Reset(empty)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("checkInvalidInput() = %v, want containing %q", err, want)
		}
	}
}

func TestCheckInvalidInput_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkInvalidInput(t.Context(), healthyStubLimiter()))
}

func TestCheckClose_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkClose(t.Context(), healthyStubLimiter()))
}

func TestCheckClose_failures(t *testing.T) {
	t.Parallel()

	boom := errors.New("ratelimittest: boom")

	t.Run("first close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubLimiter()
		stub.closeErr = boom

		if err := checkClose(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkClose() = %v, want wrap of boom", err)
		}
	})

	t.Run("second close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubLimiter()
		stub.closeAgainErr = boom

		err := checkClose(t.Context(), stub)
		if err == nil {
			t.Fatal("checkClose() = nil, want second-close error")
		}
		if !strings.Contains(err.Error(), "Close() second") {
			t.Fatalf("checkClose() = %v, want second-close error", err)
		}
	})

	t.Run("allow after close succeeds", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubLimiter()
		stub.allowAfterClose = true

		if err := checkClose(t.Context(), stub); err == nil {
			t.Fatal("checkClose() = nil, want ErrClosed from Allow")
		} else if !strings.Contains(err.Error(), "want ErrClosed") {
			t.Fatalf("checkClose() = %v, want ErrClosed", err)
		}
	})

	t.Run("reset after close succeeds", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubLimiter()
		stub.resetAfterClose = true

		if err := checkClose(t.Context(), stub); err == nil {
			t.Fatal("checkClose() = nil, want ErrClosed from Reset")
		} else if !strings.Contains(err.Error(), "want ErrClosed") {
			t.Fatalf("checkClose() = %v, want ErrClosed", err)
		}
	})
}

func TestCheck_concurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := context.Background()

			if err := checkAllowDeny(ctx, healthyStubLimiter()); err != nil {
				t.Errorf("checkAllowDeny() = %v, want nil", err)
			}

			if err := checkReset(ctx, healthyStubLimiter()); err != nil {
				t.Errorf("checkReset() = %v, want nil", err)
			}

			if err := checkClose(ctx, healthyStubLimiter()); err != nil {
				t.Errorf("checkClose() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
