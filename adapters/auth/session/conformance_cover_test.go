package session_test

import (
	"testing"

	authsession "github.com/zenta-dev/zever/adapters/auth/session"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/auth/authtest"
	"github.com/zenta-dev/zever/core/session"
)

// TestSessionConformance proves the session-backed adapter honors the
// auth.Auth contract via the shared conformance kit. Each subtest gets a
// fresh adapter over a fresh in-memory session store (no network).
// Documented exemptions live in the kit: store-expiry surfaces as
// ErrInvalidToken (miss/expiry indistinguishable by design), post-revoke
// verification is a miss (ErrInvalidToken, not ErrTokenRevoked), and
// unserializable claims persist opaquely in-process.
func TestSessionConformance(t *testing.T) {
	t.Parallel()

	authtest.Conformance(t, func(t *testing.T) auth.Auth {
		t.Helper()

		st, err := sessionmemory.New(session.Options{})
		if err != nil {
			t.Fatalf("memory.New() error = %v", err)
		}

		a, err := authsession.New(auth.Options{Session: auth.SessionOptions{Store: st}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = a.Close() })

		return a
	})
}
