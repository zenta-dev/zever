package ratelimit

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"
)

var freshSeq atomic.Int64

type stubLimiter struct{}

func (stubLimiter) Allow(_ context.Context, _ string, _ float64) (Decision, error) {
	return Decision{Allowed: true}, nil
}

func (stubLimiter) Reset(_ context.Context, _ string) error { return nil }

func (stubLimiter) Close() error { return nil }

func (stubLimiter) Name() string { return "stub" }

func TestValidateCost_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		tokens  float64
		burst   float64
		wantErr bool
	}{
		{"ok", 1, 5, false},
		{"ok_fractional", 0.5, 1, false},
		{"zero_tokens", 0, 5, true},
		{"negative_tokens", -1, 5, true},
		{"nan_tokens", math.NaN(), 5, true},
		{"inf_tokens", math.Inf(1), 5, true},
		{"neginf_tokens", math.Inf(-1), 5, true},
		{"zero_burst", 1, 0, true},
		{"negative_burst", 1, -2, true},
		{"nan_burst", 1, math.NaN(), true},
		{"inf_burst", 1, math.Inf(1), true},
		{"tokens_above_burst_capped_not_error", 10, 5, false},
	}
	for _, c := range cases {
		err := ValidateCost(c.tokens, c.burst)
		if c.wantErr && !errors.Is(err, ErrInvalidCost) {
			t.Errorf("%s: ValidateCost(%v,%v) err = %v, want ErrInvalidCost", c.name, c.tokens, c.burst, err)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%s: ValidateCost(%v,%v) err = %v, want nil", c.name, c.tokens, c.burst, err)
		}
	}
}

func TestDecision_fields(t *testing.T) {
	t.Parallel()
	d := Decision{Allowed: false, RetryAfter: time.Second, Remaining: 0}
	if d.Allowed {
		t.Error("Allowed = true, want false")
	}
	if d.RetryAfter != time.Second {
		t.Errorf("RetryAfter = %v want 1s", d.RetryAfter)
	}
}
