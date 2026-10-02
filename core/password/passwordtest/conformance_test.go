package passwordtest_test

import (
	"testing"

	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/password/passwordtest"
)

// TestConformanceArgon2 proves the kit passes against the argon2 adapter.
// Minimal valid parameters keep the kit fast.
func TestConformanceArgon2(t *testing.T) {
	t.Parallel()

	passwordtest.Conformance(t, func(t *testing.T) password.Hasher {
		t.Helper()

		h, err := passwordargon2.New(password.Options{Time: 1, Memory: 8 * 1024, Threads: 1, SaltLen: 8, KeyLen: 16})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return h
	})
}
