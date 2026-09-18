package pgvector

import (
	"testing"
)

// RED: PoolConfig helper validates shared DSN and caps MaxConns.
func TestPoolConfig_sharedDSN_defaultsMaxConns(t *testing.T) {
	t.Parallel()

	cfg, err := PoolConfig("postgres://localhost:5432/zever", 0)
	if err != nil {
		t.Fatalf("PoolConfig: %v", err)
	}

	if cfg.MaxConns != int32(DefaultMaxConns) {
		t.Errorf("want MaxConns=%d, got %d", DefaultMaxConns, cfg.MaxConns)
	}
}

func TestPoolConfig_explicitMaxConns_appliesCap(t *testing.T) {
	t.Parallel()

	cfg, err := PoolConfig("postgres://localhost:5432/zever", 2)
	if err != nil {
		t.Fatalf("PoolConfig: %v", err)
	}

	if cfg.MaxConns != 2 {
		t.Errorf("want MaxConns=2, got %d", cfg.MaxConns)
	}
}

func TestPoolConfig_invalidDSN_returnsError(t *testing.T) {
	t.Parallel()

	if _, err := PoolConfig("://bad", 0); err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}
}
