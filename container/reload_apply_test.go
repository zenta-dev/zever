package container

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/ratelimit"
)

func TestApplyReloadLimiterFailureKeepsLogLevel(t *testing.T) {
	logger := NewReloadableLogger(&stubLogger{min: log.LevelInfo})
	initial := &countingLimiter{}
	limiter := NewReloadableLimiter(initial)
	boom := errors.New("boom")

	err := ApplyReload(logger,
		func() (log.Logger, error) { return &stubLogger{min: log.LevelDebug}, nil },
		limiter,
		func() (ratelimit.Limiter, error) { return nil, boom },
	)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapping boom", err)
	}

	if logger.Enabled(log.LevelDebug) {
		t.Error("log level changed despite failed reload")
	}

	if limiter.current.Load().limiter != ratelimit.Limiter(initial) {
		t.Error("limiter changed despite failed reload")
	}
}

func TestApplyReloadLogFailureKeepsLimiter(t *testing.T) {
	logger := NewReloadableLogger(&stubLogger{min: log.LevelInfo})
	initial := &countingLimiter{}
	limiter := NewReloadableLimiter(initial)
	opened := false

	err := ApplyReload(logger,
		func() (log.Logger, error) { return nil, errors.New("bad") },
		limiter,
		func() (ratelimit.Limiter, error) { opened = true; return &countingLimiter{}, nil },
	)
	if err == nil {
		t.Fatal("want error")
	}

	if opened {
		t.Error("limiter opened after log failure")
	}
}

func TestApplyReloadSuccessSwapsBoth(t *testing.T) {
	logger := NewReloadableLogger(&stubLogger{min: log.LevelInfo})
	limiter := NewReloadableLimiter(&countingLimiter{})
	next := &countingLimiter{}

	err := ApplyReload(logger,
		func() (log.Logger, error) { return &stubLogger{min: log.LevelDebug}, nil },
		limiter,
		func() (ratelimit.Limiter, error) { return next, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	if !logger.Enabled(log.LevelDebug) || limiter.current.Load().limiter != ratelimit.Limiter(next) {
		t.Error("reload did not swap both")
	}

	// Rate limiting disabled: nil limiter is fine.
	if err := ApplyReload(logger, func() (log.Logger, error) { return &stubLogger{}, nil }, nil, nil); err != nil {
		t.Fatal(err)
	}
}
