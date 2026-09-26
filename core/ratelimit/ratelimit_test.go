package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var freshSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", 1000+int(freshSeq.Add(1))))
}

type stubLimiter struct{}

func (stubLimiter) Allow(_ context.Context, _ string, _ float64) (Decision, error) {
	return Decision{Allowed: true}, nil
}

func (stubLimiter) Reset(_ context.Context, _ string) error { return nil }

func (stubLimiter) Close() error { return nil }

func (stubLimiter) Name() string { return "stub" }

func TestRegister_nilFactory_returnsErrNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Limiter, error) { return stubLimiter{}, nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	if err := Register(a, ok); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Register err = %v, want ErrDuplicate", err)
	}
	var de *DuplicateError
	if err := Register(a, ok); !errors.As(err, &de) {
		t.Fatalf("dup err %T is not *DuplicateError", err)
	} else if de.Adapter != a {
		t.Fatalf("dup carried adapter = %v want %v", de.Adapter, a)
	}
}

func TestOpen_unknownAdapter_returnsUnknownError(t *testing.T) {
	a := Adapter("test-9999")
	_, err := Open(a, Options{Rate: 1, Burst: 1})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("carried adapter = %v want %v", ue.Adapter, a)
	}
}

func TestOpen_factoryError_wrappedWithAdapter(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (Limiter, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{Rate: 1, Burst: 1})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "ratelimit: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "ratelimit: open")
	}
}

func TestOpen_success_returnsLimiter(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Limiter, error) { return stubLimiter{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	l, err := Open(a, Options{Rate: 1, Burst: 1})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	d, err := l.Allow(t.Context(), "k", 1)
	if err != nil {
		t.Fatalf("Allow err = %v", err)
	}
	if !d.Allowed {
		t.Fatalf("Allow = %+v, want allowed", d)
	}
	if err := l.Reset(t.Context(), "k"); err != nil {
		t.Fatalf("Reset err = %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

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
