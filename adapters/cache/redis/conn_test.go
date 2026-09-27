package redis

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

func TestConnOptions_mapping(t *testing.T) {
	t.Parallel()

	opts := cache.Options{}
	opts.URL = "redis://h1:6379"
	opts.Addr = "h2:6379"
	opts.Password = "pw"
	opts.DB = 2
	opts.TLS = true
	opts.PoolSize = 20
	opts.MinIdleConns = 4
	opts.PoolTimeout = time.Second
	opts.MaxConnIdleTime = time.Minute
	opts.MaxConnLifetime = time.Hour

	got := connOptions(opts)

	if got.Addr != "redis://h1:6379" {
		t.Errorf("Addr = %q, want URL to take precedence", got.Addr)
	}

	if !got.TLS {
		t.Error("TLS = false, want true (flag forwarded)")
	}

	if got.Password != "pw" || got.DB != 2 {
		t.Errorf("Password/DB = %q/%d, want pw/2", got.Password, got.DB)
	}

	if got.PoolSize != 20 || got.MinIdleConns != 4 || got.PoolTimeout != time.Second ||
		got.MaxConnIdleTime != time.Minute || got.MaxConnLifetime != time.Hour {
		t.Errorf("pool tuning not forwarded: %+v", got)
	}
}

func TestConnOptions_addrFallback(t *testing.T) {
	t.Parallel()

	got := connOptions(cache.Options{Addr: "h2:6379"})
	if got.Addr != "h2:6379" {
		t.Errorf("Addr = %q, want h2:6379", got.Addr)
	}

	if got.TLS {
		t.Error("TLS = true, want false default")
	}
}
