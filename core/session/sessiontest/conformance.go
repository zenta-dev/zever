// Package sessiontest provides the conformance kit third-party session
// stores run to prove backend parity.
package sessiontest

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/session"
)

const (
	// DefaultEntryTTL is the short TTL conformance expiry tests set before polling for disappearance.
	DefaultEntryTTL = 30 * time.Millisecond
	// DefaultExpiryTimeout bounds how long expiry polls wait before failing.
	DefaultExpiryTimeout = 2 * time.Second
	// DefaultPollInterval is the tick between expiry-poll attempts.
	DefaultPollInterval = 5 * time.Millisecond
)

var adapterSeq atomic.Uint64

// Conformance verifies factory-built stores implement the session.Store
// contract: Create/Get round-trip, Save mutation, Delete idempotency, TTL
// expiry, Open/Register registry wiring, and Close. Each subtest takes a
// fresh instance from factory so cases stay isolated. Expiry waits poll
// with a context deadline; they never synchronize with time.Sleep and
// never touch the network.
//
// Data values stay JSON-shaped (strings) because DB-backed stores
// round-trip Data through JSON while memory stores keep values as-is.
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

func Conformance(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	t.Run("CreateGet", func(t *testing.T) { conformanceCreateGet(t, factory) })
	t.Run("Save", func(t *testing.T) { conformanceSave(t, factory) })
	t.Run("Delete", func(t *testing.T) { conformanceDelete(t, factory) })
	t.Run("TTLExpiry", func(t *testing.T) { conformanceTTLExpiry(t, factory) })
	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t, factory) })
	t.Run("Errors", func(t *testing.T) { conformanceErrors(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceCreateGet(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	if err := checkCreateGet(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkCreateGet proves Create mints a valid ID with absolute expiry and
// Get round-trips it, while unknown IDs report ErrNotFound.
func checkCreateGet(ctx context.Context, s session.Store) error {
	created, err := s.Create(ctx, time.Hour)
	if err != nil {
		return fmt.Errorf("sessiontest: Create() error = %w", err)
	}

	if verr := session.ValidateID(created.ID); verr != nil {
		return fmt.Errorf("Create() ID invalid: %w", verr)
	}

	if created.ExpiresAt.IsZero() {
		return errors.New("sessiontest: Create(hour) ExpiresAt is zero, want absolute expiry")
	}

	got, err := s.Get(ctx, created.ID)
	if err != nil {
		return fmt.Errorf("sessiontest: Get() error = %w", err)
	}

	var errs []error

	if got.ID != created.ID {
		errs = append(errs, fmt.Errorf("sessiontest: Get().ID = %q, want %q", got.ID, created.ID))
	}

	if _, err := s.Get(ctx, session.NewID()); !errors.Is(err, session.ErrNotFound) {
		errs = append(errs, errf("Get(unknown) err = %v, want ErrNotFound", err))
	}

	return errors.Join(errs...)
}

func conformanceSave(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	if err := checkSave(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkSave proves Save persists Data mutations visible on the next Get.
func checkSave(ctx context.Context, s session.Store) error {
	created, err := s.Create(ctx, time.Hour)
	if err != nil {
		return fmt.Errorf("sessiontest: Create() error = %w", err)
	}

	created.Data["user"] = "u123"

	if serr := s.Save(ctx, created); serr != nil {
		return fmt.Errorf("sessiontest: Save() error = %w", serr)
	}

	got, err := s.Get(ctx, created.ID)
	if err != nil {
		return fmt.Errorf("sessiontest: Get() error = %w", err)
	}

	var errs []error

	if got.Data["user"] != "u123" {
		errs = append(errs, fmt.Errorf("sessiontest: Get().Data[user] = %v, want u123", got.Data["user"]))
	}

	if got.ID != created.ID {
		errs = append(errs, fmt.Errorf("sessiontest: Get().ID = %q, want %q", got.ID, created.ID))
	}

	return errors.Join(errs...)
}

func conformanceDelete(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	if err := checkDelete(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkDelete proves Delete is idempotent (nil on missing IDs) and that
// deleted sessions read back as ErrNotFound.
func checkDelete(ctx context.Context, s session.Store) error {
	if err := s.Delete(ctx, session.NewID()); err != nil {
		return fmt.Errorf("sessiontest: Delete(missing) error = %w, want nil", err)
	}

	created, err := s.Create(ctx, time.Hour)
	if err != nil {
		return fmt.Errorf("sessiontest: Create() error = %w", err)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		return fmt.Errorf("sessiontest: Delete() error = %w", err)
	}

	var errs []error

	if _, err := s.Get(ctx, created.ID); !errors.Is(err, session.ErrNotFound) {
		errs = append(errs, errf("Get() after Delete err = %v, want ErrNotFound", err))
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		errs = append(errs, fmt.Errorf("sessiontest: Delete(again) error = %w, want nil", err))
	}

	return errors.Join(errs...)
}

func conformanceTTLExpiry(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	if err := checkTTLExpiry(t.Context(), factory(t), DefaultEntryTTL, DefaultExpiryTimeout); err != nil {
		t.Fatal(err)
	}
}

// checkTTLExpiry proves a short-TTL session is readable before expiry,
// maps TTL to an absolute ExpiresAt, and disappears (ErrNotFound) after
// polling. Polling uses timeout/interval so unit tests can drive both
// the expiry and timeout branches quickly.
func checkTTLExpiry(ctx context.Context, s session.Store, ttl, timeout time.Duration) error {
	created, err := s.Create(ctx, ttl)
	if err != nil {
		return fmt.Errorf("sessiontest: Create() error = %w", err)
	}

	// TTL maps to an absolute ExpiresAt roughly one TTL out: the store
	// must not mint a session cookie lifetime (MaxAge int-seconds on the
	// cookie adapter mirrors this same span) disconnected from expiry.
	var errs []error

	if until := time.Until(created.ExpiresAt); until <= 0 || until > ttl+time.Minute {
		errs = append(errs, fmt.Errorf("sessiontest: ExpiresAt = %v (in %v), want ~%v out", created.ExpiresAt, until, ttl))
	}

	if _, err := s.Get(ctx, created.ID); err != nil {
		errs = append(errs, fmt.Errorf("sessiontest: Get() before expiry error = %w", err))
		return errors.Join(errs...)
	}

	if err := pollExpiry(ctx, timeout, "session expired from Get", func(ctx context.Context) bool {
		_, err := s.Get(ctx, created.ID)
		return errors.Is(err, session.ErrNotFound)
	}); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	if err := checkOpenRegister(factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkOpenRegister proves the production Open path returns the
// registered probe, rejects duplicates, and reports unknown adapters.
func checkOpenRegister(probe session.Store) error {
	name := session.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))

	// Register the factory-built behavior under a throwaway name through
	// the production Open path: factory proves the backend, Open proves
	// the wiring.
	if err := session.Register(name, func(session.Options) (session.Store, error) { return probe, nil }); err != nil {
		return fmt.Errorf("sessiontest: Register() error = %w", err)
	}

	if err := session.Register(name, func(session.Options) (session.Store, error) { return probe, nil }); !errors.Is(err, session.ErrDuplicate) {
		return fmt.Errorf("sessiontest: Register(dup) err = %w, want ErrDuplicate", err)
	}

	opened, err := session.Open(name, session.Options{})
	if err != nil {
		return fmt.Errorf("sessiontest: Open() error = %w", err)
	}

	var errs []error

	if opened != probe {
		errs = append(errs, errors.New("sessiontest: Open() did not return the registered store"))
	}

	if _, err := session.Open("kit-no-such-adapter", session.Options{}); !errors.Is(err, session.ErrUnknownAdapter) {
		errs = append(errs, errf("Open(unknown) err = %v, want ErrUnknownAdapter", err))
	}

	return errors.Join(errs...)
}

func conformanceErrors(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	if err := checkErrors(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkErrors proves malformed IDs fail closed with ErrInvalidID on
// every method. Soft mismatches join so all are reported.
func checkErrors(ctx context.Context, s session.Store) error {
	var errs []error

	if _, err := s.Get(ctx, "bogus"); !errors.Is(err, session.ErrInvalidID) {
		errs = append(errs, errf("Get(bogus) err = %v, want ErrInvalidID", err))
	}

	var idErr session.InvalidIDError
	if _, err := s.Get(ctx, "bogus"); !errors.As(err, &idErr) {
		errs = append(errs, errf("errors.As(err, InvalidIDError) = false (err = %T %v)", err, err))
	}

	if err := s.Delete(ctx, "bogus"); !errors.Is(err, session.ErrInvalidID) {
		errs = append(errs, errf("Delete(bogus) err = %v, want ErrInvalidID", err))
	}

	if err := s.Save(ctx, session.Session{ID: "bogus"}); !errors.Is(err, session.ErrInvalidID) {
		errs = append(errs, errf("Save(bogus) err = %v, want ErrInvalidID", err))
	}

	return errors.Join(errs...)
}

func conformanceClose(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	if err := checkClose(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkClose proves Close is idempotent and that every method reports
// ErrClosed afterwards. Soft mismatches join so all are reported.
func checkClose(ctx context.Context, s session.Store) error {
	if err := s.Close(); err != nil {
		return fmt.Errorf("sessiontest: Close() error = %w", err)
	}

	var errs []error

	if err := s.Close(); err != nil {
		errs = append(errs, fmt.Errorf("sessiontest: Close() second error = %w, want nil", err))
	}

	if _, err := s.Create(ctx, time.Hour); !errors.Is(err, session.ErrClosed) {
		errs = append(errs, errf("Create() err = %v, want ErrClosed", err))
	}

	if _, err := s.Get(ctx, session.NewID()); !errors.Is(err, session.ErrClosed) {
		errs = append(errs, errf("Get() err = %v, want ErrClosed", err))
	}

	if err := s.Save(ctx, session.Session{ID: session.NewID()}); !errors.Is(err, session.ErrClosed) {
		errs = append(errs, errf("Save() err = %v, want ErrClosed", err))
	}

	if err := s.Delete(ctx, session.NewID()); !errors.Is(err, session.ErrClosed) {
		errs = append(errs, errf("Delete() err = %v, want ErrClosed", err))
	}

	return errors.Join(errs...)
}

// pollExpiry polls cond until true or timeout elapses, ticking every
// interval. It returns a descriptive error on timeout so check helpers
// can propagate it without touching *testing.T. (It replaces the old
// eventually(t, ...) helper, whose timeout branch was uncoverable: a
// *testing.T failure cannot be scripted without a real test run.)
func pollExpiry(ctx context.Context, timeout time.Duration, msg string, cond func(ctx context.Context) bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		if cond(ctx) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("condition not met within %v: %s", timeout, msg)
		case <-ticker.C:
		}
	}
}
