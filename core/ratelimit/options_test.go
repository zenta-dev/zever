package ratelimit

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestOptions_zeroRate_isInvalid(t *testing.T) {
	t.Parallel()
	err := Options{Rate: 0, Burst: 5}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Rate 0 err = %v, want ErrInvalidOptions", err)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
}

func TestOptions_zeroBurst_isInvalid(t *testing.T) {
	t.Parallel()
	if err := (Options{Rate: 10, Burst: 0}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Burst 0 err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_badRates_areInvalid(t *testing.T) {
	t.Parallel()
	for _, r := range []float64{-1, -0.5, math.Inf(1), math.Inf(-1), math.NaN()} {
		o := Options{Rate: r, Burst: 5}
		if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("Rate %v err = %v, want ErrInvalidOptions", r, err)
		}
	}
}

func TestOptions_negativeBurst_isInvalid(t *testing.T) {
	t.Parallel()
	if err := (Options{Rate: 10, Burst: -1}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Burst -1 err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_negativeTTLs_areInvalid(t *testing.T) {
	t.Parallel()
	if err := (Options{Rate: 10, Burst: 5, IdleTTL: -time.Second}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("IdleTTL -1s err = %v, want ErrInvalidOptions", err)
	}
	if err := (Options{Rate: 10, Burst: 5, SweepInterval: -time.Second}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("SweepInterval -1s err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_zeroTTLs_areValid(t *testing.T) {
	t.Parallel()
	o := Options{Rate: 10, Burst: 5}
	if err := o.Validate(); err != nil {
		t.Fatalf("zero TTLs Validate err = %v, want nil", err)
	}
	if o.idleTTL() != DefaultIdleTTL {
		t.Errorf("idleTTL() = %v want %v", o.idleTTL(), DefaultIdleTTL)
	}
	if o.sweepInterval() != DefaultSweepInterval {
		t.Errorf("sweepInterval() = %v want %v", o.sweepInterval(), DefaultSweepInterval)
	}
}

func TestOptions_badRedisAddr_isInvalid(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{
		"redis://localhost:6379",
		"http://localhost:6379",
		"localhost",
		"localhost:0",
		"localhost:99999",
		"localhost:notaport",
		":6379",
		"/tmp/redis.sock",
		"localhost:6379/db?x=1",
	} {
		o := Options{Rate: 10, Burst: 5, Redis: RedisOptions{Addr: addr}}
		if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("Addr %q err = %v, want ErrInvalidOptions", addr, err)
		}
	}
}

func TestOptions_goodRedisAddr_isValid(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"", "localhost:6379", "127.0.0.1:6380", "redis:6379", "[::1]:6379"} {
		o := Options{Rate: 10, Burst: 5, Redis: RedisOptions{Addr: addr}}
		if err := o.Validate(); err != nil {
			t.Errorf("Addr %q err = %v, want nil", addr, err)
		}
	}
}

func TestOptions_oversizePrefix_isInvalid(t *testing.T) {
	t.Parallel()
	o := Options{Rate: 10, Burst: 5, Redis: RedisOptions{Prefix: strings.Repeat("a", 65)}}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("65-char prefix err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_badPrefixChars_isInvalid(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"has space", "slash/x", "colon:bad", "semi;bad"} {
		o := Options{Rate: 10, Burst: 5, Redis: RedisOptions{Prefix: p}}
		if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("Prefix %q err = %v, want ErrInvalidOptions", p, err)
		}
	}
}

func TestOptions_minimal_valid(t *testing.T) {
	t.Parallel()
	o := Options{Rate: 10, Burst: 5}
	if err := o.Validate(); err != nil {
		t.Fatalf("minimal {Rate:10,Burst:5} Validate err = %v, want nil", err)
	}
}

func TestOptions_goodPrefix_isValid(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"", "zever-rl", "prod_v1.2", strings.Repeat("a", 64)} {
		o := Options{Rate: 10, Burst: 5, Redis: RedisOptions{Prefix: p}}
		if err := o.Validate(); err != nil {
			t.Errorf("Prefix %q err = %v, want nil", p, err)
		}
	}
}

func TestOptions_idleSweep_defaults(t *testing.T) {
	t.Parallel()
	o := Options{Rate: 10, Burst: 5, IdleTTL: time.Minute, SweepInterval: 30 * time.Second}
	if o.idleTTL() != time.Minute {
		t.Errorf("idleTTL() = %v want 1m", o.idleTTL())
	}
	if o.sweepInterval() != 30*time.Second {
		t.Errorf("sweepInterval() = %v want 30s", o.sweepInterval())
	}
}
