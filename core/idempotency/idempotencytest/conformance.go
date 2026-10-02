// Package idempotencytest provides the conformance kit third-party idempotency adapters run to prove backend parity.
package idempotencytest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// Conformance verifies factory-built stores implement the
// idempotency.Store contract: open/register round-trip, Begin miss
// reserves, Complete then replay returns the stored copy,
// in-flight Begin reports ErrInProgress, fingerprint mismatch reports
// ErrKeyMismatch even mid-flight, Forget is idempotent, invalid keys
// fail closed, and Close shuts down. Each subtest takes a fresh
// instance from factory so cases stay isolated. Tests never call
// time.Sleep and never touch the network.
//
// No-op adapters: none; every adapter must persist reservations.
// A stub that always reports miss would trivially satisfy the shape
// but break exactly-once execution, so no stub exemption exists.
func Conformance(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("ExecuteReplay", func(t *testing.T) { conformanceExecuteReplay(t, factory) })
	t.Run("InProgress", func(t *testing.T) { conformanceInProgress(t, factory) })
	t.Run("FingerprintMismatch", func(t *testing.T) { conformanceFingerprintMismatch(t, factory) })
	t.Run("Forget", func(t *testing.T) { conformanceForget(t, factory) })
	t.Run("InvalidKey", func(t *testing.T) { conformanceInvalidKey(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := idempotency.Open(idempotency.Adapter("conformance-missing-adapter"), idempotency.Options{}); !errors.Is(err, idempotency.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := idempotency.Adapter("conformance-probe-idempotency")

	if err := idempotency.Register(probe, nil); !errors.Is(err, idempotency.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(idempotency.Options) (idempotency.Store, error) {
		return nil, errors.New("idempotencytest: probe factory must not run")
	}

	_ = idempotency.Register(probe, stub)

	if err := idempotency.Register(probe, stub); !errors.Is(err, idempotency.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceExecuteReplay(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	out, err := s.Begin(ctx, "exec-01", idempotency.BeginOptions{Fingerprint: []byte("req-v1")})
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	if out.Replay || out.Result != nil {
		t.Fatalf("Begin() = %+v, want miss {false nil}", out)
	}

	result := []byte("result-v1")
	if err = s.Complete(ctx, "exec-01", []byte("req-v1"), result); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	result[0] = 'X'

	replay, err := s.Begin(ctx, "exec-01", idempotency.BeginOptions{Fingerprint: []byte("req-v1")})
	if err != nil {
		t.Fatalf("Begin(replay) error = %v", err)
	}

	if !replay.Replay {
		t.Fatal("Begin(replay) Replay = false, want true")
	}

	if string(replay.Result) != "result-v1" {
		t.Fatalf("Begin(replay) Result = %q, want stored copy", replay.Result)
	}

	replay.Result[0] = 'Y'

	again, err := s.Begin(ctx, "exec-01", idempotency.BeginOptions{Fingerprint: []byte("req-v1")})
	if err != nil {
		t.Fatalf("Begin(replay) error = %v", err)
	}

	if string(again.Result) != "result-v1" {
		t.Errorf("Begin(replay) Result = %q, want stored copy (returned copy)", again.Result)
	}
}

func conformanceInProgress(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if _, err := s.Begin(ctx, "flight-01", idempotency.BeginOptions{}); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	if _, err := s.Begin(ctx, "flight-01", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrInProgress) {
		t.Fatalf("Begin(in-flight) err = %v, want ErrInProgress", err)
	}
}

func conformanceFingerprintMismatch(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	// Mismatch wins even mid-flight: fingerprint is checked first.
	if _, err := s.Begin(ctx, "fp-01", idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	if _, err := s.Begin(ctx, "fp-01", idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Begin(mid-flight mismatch) err = %v, want ErrKeyMismatch", err)
	}

	if err := s.Complete(ctx, "fp-01", []byte("a"), []byte("done")); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if _, err := s.Begin(ctx, "fp-01", idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Begin(replay mismatch) err = %v, want ErrKeyMismatch", err)
	}

	if err := s.Complete(ctx, "fp-01", []byte("b"), []byte("other")); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Errorf("Complete(mismatch) err = %v, want ErrKeyMismatch", err)
	}
}

func conformanceForget(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if err := s.Forget(ctx, "never-seen"); err != nil {
		t.Fatalf("Forget(missing) error = %v, want nil", err)
	}

	if _, err := s.Begin(ctx, "drop-01", idempotency.BeginOptions{}); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	if err := s.Forget(ctx, "drop-01"); err != nil {
		t.Fatalf("Forget() error = %v", err)
	}

	out, err := s.Begin(ctx, "drop-01", idempotency.BeginOptions{})
	if err != nil {
		t.Fatalf("Begin(after forget) error = %v", err)
	}

	if out.Replay {
		t.Error("Begin(after forget) Replay = true, want false (fresh reservation)")
	}

	if err := s.Forget(ctx, "drop-01"); err != nil {
		t.Errorf("Forget(again) error = %v, want nil", err)
	}
}

func conformanceInvalidKey(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if _, err := s.Begin(ctx, "", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrInvalidKey) {
		t.Errorf("Begin(empty) err = %v, want ErrInvalidKey", err)
	}

	if err := s.Complete(ctx, "", nil, []byte("r")); !errors.Is(err, idempotency.ErrInvalidKey) {
		t.Errorf("Complete(empty) err = %v, want ErrInvalidKey", err)
	}

	if err := s.Forget(ctx, ""); !errors.Is(err, idempotency.ErrInvalidKey) {
		t.Errorf("Forget(empty) err = %v, want ErrInvalidKey", err)
	}

	if err := idempotency.ValidateKey(""); !errors.Is(err, idempotency.ErrInvalidKey) {
		t.Errorf("ValidateKey(empty) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if _, err := s.Begin(ctx, "kit-key", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrClosed) {
		t.Errorf("Begin() err = %v, want ErrClosed", err)
	}

	if err := s.Complete(ctx, "kit-key", nil, []byte("r")); !errors.Is(err, idempotency.ErrClosed) {
		t.Errorf("Complete() err = %v, want ErrClosed", err)
	}

	if err := s.Forget(ctx, "kit-key"); !errors.Is(err, idempotency.ErrClosed) {
		t.Errorf("Forget() err = %v, want ErrClosed", err)
	}
}
