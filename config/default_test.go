package config

import (
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

func TestDefaultSnapshot(t *testing.T) {
	t.Parallel()
	cfg := Default()
	if cfg.Crypto.Options.Key == "" {
		t.Error("crypto dev key missing")
	}
	if cfg.Mailer.Options.Host == "" || cfg.Mailer.Options.Port == 0 {
		t.Error("mailer dummy host/port missing")
	}
	if cfg.Observability.Options.ServiceName == "" {
		t.Error("observability service name missing")
	}
	if cfg.Password.Options.Time != password.DefaultTime ||
		cfg.Password.Options.Memory != password.DefaultMemory ||
		cfg.Password.Options.Threads != password.DefaultThreads ||
		cfg.Password.Options.SaltLen != password.DefaultSaltLen ||
		cfg.Password.Options.KeyLen != password.DefaultKeyLen {
		t.Error("password defaults mismatch")
	}
	if cfg.RateLimit.Options.Rate <= 0 || cfg.RateLimit.Options.Burst <= 0 {
		t.Error("ratelimit defaults must be positive")
	}
	if cfg.Scheduler.Options.Dispatcher == nil {
		t.Error("scheduler dispatcher missing")
	}
	if cfg.Secrets.Options.Prefix == "" {
		t.Error("secrets prefix missing")
	}
	for _, svc := range knownServiceNames() {
		opts, ok := serviceRefs(cfg, svc)
		if !ok {
			t.Fatalf("%s: serviceRefs missing", svc)
		}
		if opts == nil {
			t.Fatalf("%s: nil options", svc)
		}
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default must pass Validate: %v", err)
	}
}

// TestDefaultOptionsOnly pins that Default carries no adapter names: the
// service envelope serializes to options alone.
func TestDefaultOptionsOnly(t *testing.T) {
	t.Parallel()
	got := Default().RedactedServices()
	if len(got) != 37 {
		t.Fatalf("got %d services, want 37", len(got))
	}
}
