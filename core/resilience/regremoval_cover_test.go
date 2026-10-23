package resilience

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers every typed error left uncovered when the
// adapter registry was removed: Register/Open used to exercise them via
// duplicate/unknown/invalid adapter paths, so direct unit tests now pin the
// message constructors, sentinel unwrapping, and As round-trips.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSub  string
		asTarget any
	}{
		{"duplicate", DuplicateError{Adapter: "breaker"}, ErrDuplicateAdapter, "breaker", &DuplicateError{}},
		{"unknown", UnknownAdapterError{Adapter: "breaker"}, ErrUnknownAdapter, "breaker", &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, "bogus", &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad timeout"}, ErrInvalidOptions, "bad timeout", &InvalidOptionsError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "resilience: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "resilience: ")
			}

			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tc.wantSub)
			}

			if !errors.Is(tc.err, tc.sentinel) {
				t.Errorf("errors.Is(err, sentinel) = false (err = %v)", tc.err)
			}

			if !errors.As(tc.err, tc.asTarget) {
				t.Errorf("errors.As(%T) = false", tc.err)
			}
		})
	}
}

// TestRegRemovalValidateBreakerBranches covers the breaker Validate
// branches left without registry-driven fixtures: every negative
// breaker field must fail while the breaker is enabled.
func TestRegRemovalValidateBreakerBranches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Options)
		want   string
	}{
		{"min_requests", func(o *Options) { o.Breaker.MinRequests = -1 }, "min_requests"},
		{"consecutive", func(o *Options) { o.Breaker.ConsecutiveFailures = -1 }, "consecutive_failures"},
		{"interval", func(o *Options) { o.Breaker.Interval = -1 }, "interval"},
		{"bucket", func(o *Options) { o.Breaker.BucketPeriod = -1 }, "bucket_period"},
		{"timeout", func(o *Options) { o.Breaker.Timeout = -1 }, "timeout"},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := Default()
			if !opts.Breaker.Enabled {
				t.Fatal("Default() Breaker.Enabled = false, want true")
			}

			tc.mutate(&opts)

			err := opts.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want %q violation", tc.want)
			}

			if !errors.Is(err, ErrInvalidOptions) {
				t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %q, want substring %q", err, tc.want)
			}
		})
	}
}

// TestRegRemovalValidateRedisPrefix covers the Redis prefix Validate
// branch: a prefix with non-token characters must fail closed.
func TestRegRemovalValidateRedisPrefix(t *testing.T) {
	t.Parallel()

	opts := Default()
	opts.Redis.Prefix = "has space"

	err := opts.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want prefix violation")
	}

	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
	}
}
