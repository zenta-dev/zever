package flagtest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/flag"
)

// stubFlag is a scriptable flag.Flag double. Zero value returns
// fallbacks with nil errors (the conformance contract); per-method
// error/override fields drive each failure branch.
type stubFlag struct {
	mu sync.Mutex

	boolVal   bool
	boolErr   error
	stringVal string
	stringErr error
	intVal    int
	intErr    error
	jsonErr   error
	closeErr  error
	closes    int
	closeAg   error
}

func healthyStubFlag() *stubFlag {
	return &stubFlag{boolVal: true, stringVal: "fallback", intVal: 42}
}

func (s *stubFlag) Bool(_ context.Context, key string, fallback bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.boolErr != nil {
		return false, s.boolErr
	}
	if key == "" || len(key) > 256 {
		return fallback, flag.InvalidKeyError{KeyLen: len(key)}
	}
	return s.boolVal, nil
}

func (s *stubFlag) String(_ context.Context, key string, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stringErr != nil {
		return "", s.stringErr
	}
	if key == "" {
		return "", flag.InvalidKeyError{KeyLen: 0}
	}
	return s.stringVal, nil
}

func (s *stubFlag) Int(_ context.Context, key string, _ int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.intErr != nil {
		return 0, s.intErr
	}
	if key == "" {
		return 0, flag.InvalidKeyError{KeyLen: 0}
	}
	return s.intVal, nil
}

func (s *stubFlag) JSON(_ context.Context, key string, _ any, _ any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.jsonErr != nil {
		return s.jsonErr
	}
	if key == "" {
		return flag.InvalidKeyError{KeyLen: 0}
	}
	return nil
}

func (s *stubFlag) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closes++
	if s.closes > 1 {
		return s.closeAg
	}
	return s.closeErr
}

func mustPass(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("check err = %v, want nil", err)
	}
}

func TestCheckOpenRegisterSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkOpenRegister())
}

func TestCheckFallbackSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkFallback(t.Context(), healthyStubFlag()))
}

func TestCheckFallbackFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("flagtest: boom")

	cases := []struct {
		name     string
		mutate   func(*stubFlag)
		contains string
	}{
		{"bool error", func(s *stubFlag) { s.boolErr = boom }, "Bool(missing)"},
		{"bool wrong value", func(s *stubFlag) { s.boolVal = false }, "Bool(missing)"},
		{"string error", func(s *stubFlag) { s.stringErr = boom }, "String(missing)"},
		{"string wrong value", func(s *stubFlag) { s.stringVal = "other" }, "String(missing)"},
		{"int error", func(s *stubFlag) { s.intErr = boom }, "Int(missing)"},
		{"int wrong value", func(s *stubFlag) { s.intVal = 7 }, "Int(missing)"},
		{"json error", func(s *stubFlag) { s.jsonErr = boom }, "JSON(missing)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubFlag()
			tc.mutate(stub)

			err := checkFallback(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkFallback() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkFallback() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckFallbackBoolErrorWraps(t *testing.T) {
	t.Parallel()

	boom := errors.New("flagtest: boom")
	stub := healthyStubFlag()
	stub.boolErr = boom

	err := checkFallback(t.Context(), stub)
	if err == nil || !strings.Contains(err.Error(), "Bool(missing)") {
		t.Fatalf("checkFallback() = %v, want Bool error", err)
	}
}

func TestCheckInvalidKeySuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkInvalidKey(t.Context(), healthyStubFlag()))
}

func TestCheckInvalidKeyFailures(t *testing.T) {
	t.Parallel()

	// permissiveFlag never rejects keys, so every branch fails and joins.
	err := checkInvalidKey(t.Context(), &permissiveFlag{})
	if err == nil {
		t.Fatal("checkInvalidKey(permissive) = nil, want joined errors")
	}

	// Note: the ValidateKey(empty) branch calls the real flag.ValidateKey,
	// which always rejects ""; the permissive double cannot break it, so
	// only the five stub-driven branches join here.
	for _, want := range []string{"Bool(empty)", "String(empty)", "Int(empty)", "JSON(empty)", "Bool(long)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("checkInvalidKey() = %v, want containing %q", err, want)
		}
	}
}

// permissiveFlag accepts every key, violating the invalid-key contract.
type permissiveFlag struct{}

func (permissiveFlag) Bool(_ context.Context, _ string, fb bool) (bool, error) {
	return fb, nil
}

func (permissiveFlag) String(_ context.Context, _ string, fb string) (string, error) {
	return fb, nil
}

func (permissiveFlag) Int(_ context.Context, _ string, fb int) (int, error) {
	return fb, nil
}

func (permissiveFlag) JSON(_ context.Context, _ string, _ any, _ any) error { return nil }

func (permissiveFlag) Close() error { return nil }

func TestCheckCloseSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkClose(healthyStubFlag()))
}

func TestCheckCloseFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("flagtest: boom")

	t.Run("first close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubFlag()
		stub.closeErr = boom

		if err := checkClose(stub); !errors.Is(err, boom) {
			t.Fatalf("checkClose() = %v, want wrap of boom", err)
		}
	})

	t.Run("second close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubFlag()
		stub.closeAg = boom

		if err := checkClose(stub); err == nil {
			t.Fatal("checkClose() = nil, want second-close error")
		} else if !strings.Contains(err.Error(), "Close() second") {
			t.Fatalf("checkClose() = %v, want second-close error", err)
		}
	})
}

func TestCheckConcurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := context.Background()

			if err := checkFallback(ctx, healthyStubFlag()); err != nil {
				t.Errorf("checkFallback() = %v, want nil", err)
			}

			if err := checkInvalidKey(ctx, healthyStubFlag()); err != nil {
				t.Errorf("checkInvalidKey() = %v, want nil", err)
			}

			if err := checkClose(healthyStubFlag()); err != nil {
				t.Errorf("checkClose() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
