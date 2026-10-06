package vault

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

// TestRegisterAdapter proves Register wires the vault factory into the
// secrets registry: after registration the adapter kind resolves instead of
// reporting ErrUnknownAdapter, and a repeated Register is harmless.
func TestRegisterAdapter(t *testing.T) {
	t.Parallel()

	Register()
	Register()

	s, err := secrets.Open(secrets.AdapterVault, secrets.Options{})
	if errors.Is(err, secrets.ErrUnknownAdapter) {
		t.Fatalf("secrets.Open(%q) after Register() = %v, want adapter wired", secrets.AdapterVault, err)
	}

	if s != nil {
		_ = s.Close(t.Context())
	}
}
