// Package revocationtest provides a shared contract test suite run against
// every revocation.Store implementation, so memory and redis stay behavior
// compatible.
package revocationtest

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/auth/revocation"
)

// Run exercises the revocation.Store contract against a fresh store built by
// newStore for each subtest.
func Run(t *testing.T, newStore func(t *testing.T) revocation.Store) {
	t.Helper()

	t.Run("UnknownJTIIsNotRevoked", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		revoked, err := s.IsRevoked(ctx, "unknown-jti")
		if err != nil {
			t.Fatalf("IsRevoked() error = %v", err)
		}
		if revoked {
			t.Fatal("IsRevoked() = true, want false for unknown jti")
		}
	})

	t.Run("RevokeThenIsRevoked", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		until := time.Now().Add(time.Hour)
		if err := s.Revoke(ctx, "jti-1", until); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}

		revoked, err := s.IsRevoked(ctx, "jti-1")
		if err != nil {
			t.Fatalf("IsRevoked() error = %v", err)
		}
		if !revoked {
			t.Fatal("IsRevoked() = false, want true after Revoke")
		}
	})

	t.Run("RevokeIsIdempotent", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		until := time.Now().Add(time.Hour)
		if err := s.Revoke(ctx, "jti-2", until); err != nil {
			t.Fatalf("Revoke() first call error = %v", err)
		}
		if err := s.Revoke(ctx, "jti-2", until); err != nil {
			t.Fatalf("Revoke() second call error = %v, want nil", err)
		}
	})

	t.Run("EmptyJTIIsNotRevoked", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		revoked, err := s.IsRevoked(ctx, "")
		if err != nil {
			t.Fatalf("IsRevoked(empty) error = %v", err)
		}
		if revoked {
			t.Fatal("IsRevoked(empty) = true, want false")
		}
	})

	t.Run("RevokeRejectsEmptyJTI", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		if err := s.Revoke(ctx, "", time.Now().Add(time.Hour)); err == nil {
			t.Fatal("Revoke(empty jti) succeeded, want error")
		}
	})

	t.Run("ExpiredRevocationIsNotRevoked", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		// Already-past expiry: the entry is recorded but must read back as
		// not revoked (mirrors a token whose grace window has lapsed).
		past := time.Now().Add(-time.Minute)
		if err := s.Revoke(ctx, "jti-3", past); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}

		revoked, err := s.IsRevoked(ctx, "jti-3")
		if err != nil {
			t.Fatalf("IsRevoked() error = %v", err)
		}
		if revoked {
			t.Fatal("IsRevoked() = true, want false for an already-expired revocation")
		}
	})

	t.Run("DistinctJTIsAreIndependent", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		if err := s.Revoke(ctx, "jti-a", time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("Revoke(jti-a) error = %v", err)
		}

		revoked, err := s.IsRevoked(ctx, "jti-b")
		if err != nil {
			t.Fatalf("IsRevoked(jti-b) error = %v", err)
		}
		if revoked {
			t.Fatal("IsRevoked(jti-b) = true, want false: unrevoked sibling jti")
		}
	})
}
