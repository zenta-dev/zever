package argon2

import (
	"testing"

	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/password/passwordtest"
)

// TestArgon2Conformance proves the argon2 adapter honors the
// password.Hasher contract via the shared conformance kit. Minimal
// valid parameters keep the kit fast; each subtest hashes fresh.
func TestArgon2Conformance(t *testing.T) {
	passwordtest.Conformance(t, func(t *testing.T) password.Hasher {
		t.Helper()

		h, err := New(password.Options{Time: 1, Memory: 8 * 1024, Threads: 1, SaltLen: 8, KeyLen: 16})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return h
	})
}
