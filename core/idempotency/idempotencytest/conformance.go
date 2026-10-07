// Package idempotencytest provides the conformance kit third-party idempotency adapters run to prove backend parity.
package idempotencytest

import (
	"context"
	"errors"
	"fmt"
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
// errf builds a non-wrapping descriptive error with Sprintf semantics.
// Check helpers use it (instead of fmt.Errorf with %w) for diagnostics
// where the formatted error may be nil: %w of a nil error prints
// "%!w(<nil>)", diverging from the historical Fatalf text, and errorlint
// forbids %v of an error in Errorf. Failure text stays byte-identical.
// It deliberately avoids fmt.Errorf so only real failures wrap.
func errf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)

	return errors.New(msg)
}

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

	if err := checkOpenRegister(); err != nil {
		t.Fatal(err)
	}
}

// checkOpenRegister proves the open/register round-trip against the
// shared registry. It returns a descriptive error on the first
// contract violation so unit tests can drive every branch.
func checkOpenRegister() error {
	if _, err := idempotency.Open(idempotency.Adapter("conformance-missing-adapter"), idempotency.Options{}); !errors.Is(err, idempotency.ErrUnknownAdapter) {
		return fmt.Errorf("idempotencytest: Open(missing) err = %w, want ErrUnknownAdapter", err)
	}

	probe := idempotency.Adapter("conformance-probe-idempotency")

	if err := idempotency.Register(probe, nil); !errors.Is(err, idempotency.ErrNilFactory) {
		return fmt.Errorf("idempotencytest: Register(nil) err = %w, want ErrNilFactory", err)
	}

	stub := func(idempotency.Options) (idempotency.Store, error) {
		return nil, errors.New("idempotencytest: probe factory must not run")
	}

	_ = idempotency.Register(probe, stub)

	if err := idempotency.Register(probe, stub); !errors.Is(err, idempotency.ErrDuplicate) {
		return fmt.Errorf("idempotencytest: Register(duplicate) err = %w, want ErrDuplicate", err)
	}

	return nil
}

func conformanceExecuteReplay(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	if err := checkExecuteReplay(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkExecuteReplay proves miss-reserve, Complete, then replay returns
// the stored copy (defensively copied both ways). It returns the first
// contract violation so unit tests can drive every branch.
func checkExecuteReplay(ctx context.Context, s idempotency.Store) error {
	out, err := s.Begin(ctx, "exec-01", idempotency.BeginOptions{Fingerprint: []byte("req-v1")})
	if err != nil {
		return fmt.Errorf("idempotencytest: Begin() error = %w", err)
	}

	if out.Replay || out.Result != nil {
		return fmt.Errorf("idempotencytest: Begin() = %+v, want miss {false nil}", out)
	}

	result := []byte("result-v1")
	if err = s.Complete(ctx, "exec-01", []byte("req-v1"), result); err != nil {
		return fmt.Errorf("idempotencytest: Complete() error = %w", err)
	}

	result[0] = 'X'

	replay, err := s.Begin(ctx, "exec-01", idempotency.BeginOptions{Fingerprint: []byte("req-v1")})
	if err != nil {
		return fmt.Errorf("idempotencytest: Begin(replay) error = %w", err)
	}

	if !replay.Replay {
		return errors.New("idempotencytest: Begin(replay) Replay = false, want true")
	}

	if string(replay.Result) != "result-v1" {
		return fmt.Errorf("idempotencytest: Begin(replay) Result = %q, want stored copy", replay.Result)
	}

	replay.Result[0] = 'Y'

	again, err := s.Begin(ctx, "exec-01", idempotency.BeginOptions{Fingerprint: []byte("req-v1")})
	if err != nil {
		return fmt.Errorf("idempotencytest: Begin(replay) error = %w", err)
	}

	if string(again.Result) != "result-v1" {
		return fmt.Errorf("idempotencytest: Begin(replay) Result = %q, want stored copy (returned copy)", again.Result)
	}

	return nil
}

func conformanceInProgress(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	if err := checkInProgress(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkInProgress proves a second Begin on a pending key reports
// ErrInProgress.
func checkInProgress(ctx context.Context, s idempotency.Store) error {
	if _, err := s.Begin(ctx, "flight-01", idempotency.BeginOptions{}); err != nil {
		return fmt.Errorf("idempotencytest: Begin() error = %w", err)
	}

	if _, err := s.Begin(ctx, "flight-01", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrInProgress) {
		return errf("Begin(in-flight) err = %v, want ErrInProgress", err)
	}

	return nil
}

func conformanceFingerprintMismatch(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	if err := checkFingerprintMismatch(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkFingerprintMismatch proves fingerprint mismatch wins even
// mid-flight and on replay, and that Complete with a wrong fingerprint
// also reports ErrKeyMismatch.
func checkFingerprintMismatch(ctx context.Context, s idempotency.Store) error {
	// Mismatch wins even mid-flight: fingerprint is checked first.
	if _, err := s.Begin(ctx, "fp-01", idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		return fmt.Errorf("idempotencytest: Begin() error = %w", err)
	}

	if _, err := s.Begin(ctx, "fp-01", idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		return errf("Begin(mid-flight mismatch) err = %v, want ErrKeyMismatch", err)
	}

	if err := s.Complete(ctx, "fp-01", []byte("a"), []byte("done")); err != nil {
		return fmt.Errorf("idempotencytest: Complete() error = %w", err)
	}

	if _, err := s.Begin(ctx, "fp-01", idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		return errf("Begin(replay mismatch) err = %v, want ErrKeyMismatch", err)
	}

	if err := s.Complete(ctx, "fp-01", []byte("b"), []byte("other")); !errors.Is(err, idempotency.ErrKeyMismatch) {
		return errf("Complete(mismatch) err = %v, want ErrKeyMismatch", err)
	}

	return nil
}

func conformanceForget(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	if err := checkForget(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkForget proves Forget is idempotent and that a forgotten key
// reserves fresh on the next Begin.
func checkForget(ctx context.Context, s idempotency.Store) error {
	if err := s.Forget(ctx, "never-seen"); err != nil {
		return fmt.Errorf("idempotencytest: Forget(missing) error = %w, want nil", err)
	}

	if _, err := s.Begin(ctx, "drop-01", idempotency.BeginOptions{}); err != nil {
		return fmt.Errorf("idempotencytest: Begin() error = %w", err)
	}

	if err := s.Forget(ctx, "drop-01"); err != nil {
		return fmt.Errorf("idempotencytest: Forget() error = %w", err)
	}

	out, err := s.Begin(ctx, "drop-01", idempotency.BeginOptions{})
	if err != nil {
		return fmt.Errorf("idempotencytest: Begin(after forget) error = %w", err)
	}

	if out.Replay {
		return errors.New("idempotencytest: Begin(after forget) Replay = true, want false (fresh reservation)")
	}

	if err := s.Forget(ctx, "drop-01"); err != nil {
		return fmt.Errorf("idempotencytest: Forget(again) error = %w, want nil", err)
	}

	return nil
}

func conformanceInvalidKey(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	if err := checkInvalidKey(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkInvalidKey proves empty keys fail closed on every method plus
// the package ValidateKey. Soft mismatches join so all are reported.
func checkInvalidKey(ctx context.Context, s idempotency.Store) error {
	var errs []error

	if _, err := s.Begin(ctx, "", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrInvalidKey) {
		errs = append(errs, errf("Begin(empty) err = %v, want ErrInvalidKey", err))
	}

	if err := s.Complete(ctx, "", nil, []byte("r")); !errors.Is(err, idempotency.ErrInvalidKey) {
		errs = append(errs, errf("Complete(empty) err = %v, want ErrInvalidKey", err))
	}

	if err := s.Forget(ctx, ""); !errors.Is(err, idempotency.ErrInvalidKey) {
		errs = append(errs, errf("Forget(empty) err = %v, want ErrInvalidKey", err))
	}

	if err := idempotency.ValidateKey(""); !errors.Is(err, idempotency.ErrInvalidKey) {
		errs = append(errs, errf("ValidateKey(empty) err = %v, want ErrInvalidKey", err))
	}

	return errors.Join(errs...)
}

func conformanceClose(t *testing.T, factory func(t *testing.T) idempotency.Store) {
	t.Helper()

	if err := checkClose(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkClose proves Close is idempotent and that every method reports
// ErrClosed afterwards. Soft mismatches join so all are reported.
func checkClose(ctx context.Context, s idempotency.Store) error {
	if err := s.Close(); err != nil {
		return fmt.Errorf("idempotencytest: Close() error = %w", err)
	}

	var errs []error

	if err := s.Close(); err != nil {
		errs = append(errs, fmt.Errorf("idempotencytest: Close() second error = %w, want nil", err))
	}

	if _, err := s.Begin(ctx, "kit-key", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrClosed) {
		errs = append(errs, errf("Begin() err = %v, want ErrClosed", err))
	}

	if err := s.Complete(ctx, "kit-key", nil, []byte("r")); !errors.Is(err, idempotency.ErrClosed) {
		errs = append(errs, errf("Complete() err = %v, want ErrClosed", err))
	}

	if err := s.Forget(ctx, "kit-key"); !errors.Is(err, idempotency.ErrClosed) {
		errs = append(errs, errf("Forget() err = %v, want ErrClosed", err))
	}

	return errors.Join(errs...)
}
