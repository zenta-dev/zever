// Package sessiontest provides the conformance kit third-party session
// stores run to prove backend parity.
package sessiontest

import (
	"context"
	"errors"
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

	ctx := t.Context()
	s := factory(t)

	created, err := s.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if verr := session.ValidateID(created.ID); verr != nil {
		t.Fatalf("Create() ID invalid: %v", verr)
	}

	if created.ExpiresAt.IsZero() {
		t.Fatal("Create(hour) ExpiresAt is zero, want absolute expiry")
	}

	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("Get().ID = %q, want %q", got.ID, created.ID)
	}

	if _, err := s.Get(ctx, session.NewID()); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Get(unknown) err = %v, want ErrNotFound", err)
	}
}

func conformanceSave(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	created, err := s.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	created.Data["user"] = "u123"

	if serr := s.Save(ctx, created); serr != nil {
		t.Fatalf("Save() error = %v", serr)
	}

	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Data["user"] != "u123" {
		t.Errorf("Get().Data[user] = %v, want u123", got.Data["user"])
	}

	if got.ID != created.ID {
		t.Errorf("Get().ID = %q, want %q", got.ID, created.ID)
	}
}

func conformanceDelete(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if err := s.Delete(ctx, session.NewID()); err != nil {
		t.Fatalf("Delete(missing) error = %v, want nil", err)
	}

	created, err := s.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := s.Get(ctx, created.ID); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Get() after Delete err = %v, want ErrNotFound", err)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Errorf("Delete(again) error = %v, want nil", err)
	}
}

func conformanceTTLExpiry(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	created, err := s.Create(ctx, DefaultEntryTTL)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// TTL maps to an absolute ExpiresAt roughly one TTL out: the store
	// must not mint a session cookie lifetime (MaxAge int-seconds on the
	// cookie adapter mirrors this same span) disconnected from expiry.
	if ttl := time.Until(created.ExpiresAt); ttl <= 0 || ttl > DefaultEntryTTL+time.Minute {
		t.Errorf("ExpiresAt = %v (in %v), want ~%v out", created.ExpiresAt, ttl, DefaultEntryTTL)
	}

	if _, err := s.Get(ctx, created.ID); err != nil {
		t.Fatalf("Get() before expiry error = %v", err)
	}

	eventually(t, "session expired from Get", func(ctx context.Context) bool {
		_, err := s.Get(ctx, created.ID)
		return errors.Is(err, session.ErrNotFound)
	})
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	name := session.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))

	// Register the factory-built behavior under a throwaway name through
	// the production Open path: factory proves the backend, Open proves
	// the wiring.
	probe := factory(t)

	if err := session.Register(name, func(session.Options) (session.Store, error) { return probe, nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := session.Register(name, func(session.Options) (session.Store, error) { return probe, nil }); !errors.Is(err, session.ErrDuplicate) {
		t.Fatalf("Register(dup) err = %v, want ErrDuplicate", err)
	}

	opened, err := session.Open(name, session.Options{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if opened != probe {
		t.Error("Open() did not return the registered store")
	}

	if _, err := session.Open("kit-no-such-adapter", session.Options{}); !errors.Is(err, session.ErrUnknownAdapter) {
		t.Errorf("Open(unknown) err = %v, want ErrUnknownAdapter", err)
	}
}

func conformanceErrors(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if _, err := s.Get(ctx, "bogus"); !errors.Is(err, session.ErrInvalidID) {
		t.Errorf("Get(bogus) err = %v, want ErrInvalidID", err)
	}

	var idErr *session.InvalidIDError
	if _, err := s.Get(ctx, "bogus"); !errors.As(err, &idErr) {
		t.Errorf("errors.As(err, InvalidIDError) = false (err = %T %v)", err, err)
	}

	if err := s.Delete(ctx, "bogus"); !errors.Is(err, session.ErrInvalidID) {
		t.Errorf("Delete(bogus) err = %v, want ErrInvalidID", err)
	}

	if err := s.Save(ctx, session.Session{ID: "bogus"}); !errors.Is(err, session.ErrInvalidID) {
		t.Errorf("Save(bogus) err = %v, want ErrInvalidID", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) session.Store) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if _, err := s.Create(ctx, time.Hour); !errors.Is(err, session.ErrClosed) {
		t.Errorf("Create() err = %v, want ErrClosed", err)
	}

	if _, err := s.Get(ctx, session.NewID()); !errors.Is(err, session.ErrClosed) {
		t.Errorf("Get() err = %v, want ErrClosed", err)
	}

	if err := s.Save(ctx, session.Session{ID: session.NewID()}); !errors.Is(err, session.ErrClosed) {
		t.Errorf("Save() err = %v, want ErrClosed", err)
	}

	if err := s.Delete(ctx, session.NewID()); !errors.Is(err, session.ErrClosed) {
		t.Errorf("Delete() err = %v, want ErrClosed", err)
	}
}

// eventually polls cond until true or DefaultExpiryTimeout elapses. Poll
// ticks use a ticker, never time.Sleep, and cond receives a deadline-bound
// context so backend calls share the same deadline.
func eventually(t *testing.T, msg string, cond func(ctx context.Context) bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), DefaultExpiryTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		if cond(ctx) {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf("condition not met within %v: %s", DefaultExpiryTimeout, msg)
		case <-ticker.C:
		}
	}
}
