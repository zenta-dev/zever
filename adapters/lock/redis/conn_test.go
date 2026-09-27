package redis

import (
	"testing"

	"github.com/zenta-dev/zever/core/lock"
)

func TestConnOptions_mapping(t *testing.T) {
	t.Parallel()

	opts := lock.Options{}
	opts.URL = "redis://h1:6379"
	opts.Addr = "h2:6379"
	opts.Password = "pw"
	opts.DB = 4
	opts.TLS = true

	got := connOptions(opts)

	if got.Addr != "redis://h1:6379" {
		t.Errorf("Addr = %q, want URL to take precedence", got.Addr)
	}

	if !got.TLS {
		t.Error("TLS = false, want true (flag forwarded)")
	}

	if got.Password != "pw" || got.DB != 4 {
		t.Errorf("Password/DB = %q/%d, want pw/4", got.Password, got.DB)
	}
}

func TestConnOptions_addrFallback(t *testing.T) {
	t.Parallel()

	got := connOptions(lock.Options{Addr: "h2:6379"})
	if got.Addr != "h2:6379" {
		t.Errorf("Addr = %q, want h2:6379", got.Addr)
	}

	if got.TLS {
		t.Error("TLS = true, want false default")
	}
}
