package redis

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/auth/jwt/revocation"
	"github.com/zenta-dev/zever/auth/jwt/revocation/revocationtest"
	zredis "github.com/zenta-dev/zever/internal/redis"
)

// zredisConnectOptions builds ConnectOptions pointed at addr.
func zredisConnectOptions(addr string) zredis.ConnectOptions {
	return zredis.ConnectOptions{Addr: addr}
}

// newTestStore starts a fresh miniredis server per test and opens a store
// against it. internal/redis keeps a singleton client, so a distinct
// address per test forces a fresh connection instead of reusing a closed one.
func newTestStore(t *testing.T) revocation.Store {
	t.Helper()

	s := miniredis.RunT(t)

	st, err := New(Options{ConnectOptions: zredisConnectOptions(s.Addr())})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	return st
}

func TestStoreContract(t *testing.T) {
	revocationtest.Run(t, newTestStore)
}

func TestNew_DefaultPrefix(t *testing.T) {
	s := miniredis.RunT(t)

	si, err := New(Options{ConnectOptions: zredisConnectOptions(s.Addr())})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = si.Close() })

	st, ok := si.(*store)
	if !ok {
		t.Fatalf("New() returned %T, want *store", si)
	}
	if st.prefix != defaultPrefix {
		t.Fatalf("prefix = %q, want %q", st.prefix, defaultPrefix)
	}
}

func TestNew_BadURL(t *testing.T) {
	if _, err := New(Options{URL: "redis://"}); err == nil {
		t.Fatal("New(bad url) succeeded, want error")
	}
}

func TestNew_PingFailure(t *testing.T) {
	_, err := New(Options{ConnectOptions: zredisConnectOptions("127.0.0.1:1")})
	if err == nil {
		t.Fatal("New() with unreachable addr succeeded, want error")
	}
}

func TestRevoke_ExpiresWithTTL(t *testing.T) {
	s := miniredis.RunT(t)

	si, err := New(Options{ConnectOptions: zredisConnectOptions(s.Addr())})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = si.Close() })

	ctx := t.Context()
	until := time.Now().Add(time.Minute)
	if rerr := si.Revoke(ctx, "jti-ttl", until); rerr != nil {
		t.Fatalf("Revoke() error = %v", rerr)
	}

	revoked, err := si.IsRevoked(ctx, "jti-ttl")
	if err != nil || !revoked {
		t.Fatalf("IsRevoked() = %v, %v, want true, nil", revoked, err)
	}

	s.FastForward(2 * time.Minute)

	revoked, err = si.IsRevoked(ctx, "jti-ttl")
	if err != nil {
		t.Fatalf("IsRevoked() after expiry error = %v", err)
	}
	if revoked {
		t.Fatal("IsRevoked() after TTL expiry = true, want false")
	}
}

func TestClose_Idempotent(t *testing.T) {
	s := miniredis.RunT(t)

	si, err := New(Options{ConnectOptions: zredisConnectOptions(s.Addr())})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := si.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := si.Close(); err != nil {
		t.Fatalf("Close() second call error = %v, want nil", err)
	}
}

func TestRevoke_AfterClose(t *testing.T) {
	s := miniredis.RunT(t)

	si, err := New(Options{ConnectOptions: zredisConnectOptions(s.Addr())})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := si.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := si.Revoke(t.Context(), "jti", time.Now().Add(time.Hour)); !errors.Is(err, revocation.ErrClosed) {
		t.Fatalf("Revoke() after Close error = %v, want ErrClosed", err)
	}
}

func TestIsRevoked_AfterClose(t *testing.T) {
	s := miniredis.RunT(t)

	si, err := New(Options{ConnectOptions: zredisConnectOptions(s.Addr())})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := si.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := si.IsRevoked(t.Context(), "jti"); !errors.Is(err, revocation.ErrClosed) {
		t.Fatalf("IsRevoked() after Close error = %v, want ErrClosed", err)
	}
}

func TestRevoke_EmptyJTI(t *testing.T) {
	s := miniredis.RunT(t)

	si, err := New(Options{ConnectOptions: zredisConnectOptions(s.Addr())})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = si.Close() })

	if err := si.Revoke(t.Context(), "", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("Revoke(empty jti) succeeded, want error")
	}
}
