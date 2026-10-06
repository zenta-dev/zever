package redis

import (
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// TestRegister proves Register wires the redis adapter into the idempotency
// registry, so Open resolves it to a live store over a broker.
func TestRegister(t *testing.T) {
	t.Parallel()

	Register()

	s, err := idempotency.Open(idempotency.Redis, testOptions(t))
	if err != nil {
		t.Fatalf("Open(%s) after Register: %v", idempotency.Redis, err)
	}

	if s == nil {
		t.Fatal("Open returned nil store")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
