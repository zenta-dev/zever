package config

import (
	"strings"
	"testing"
)

func TestValidateDefaultPasses(t *testing.T) {
	t.Parallel()
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default must validate: %v", err)
	}
}

func TestValidateFailClosed(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Crypto.Options.Key = "x"
	cfg.DB.Adapter = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "crypto") {
		t.Fatalf("missing crypto failure: %v", err)
	}
	if !strings.Contains(err.Error(), "db") {
		t.Fatalf("missing db failure: %v", err)
	}
}

func TestValidateBadAdapter(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Router.Adapter = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("want error")
	}
}

func TestValidatePoisonCrypto(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Crypto.Options.Key = "not-valid-base64!!!"
	if err := cfg.Validate(); err == nil {
		t.Fatal("want error")
	}
}

func TestValidateSchedulerNilDispatcher(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Scheduler.Options.Dispatcher = nil
	err := cfg.Validate()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "scheduler") {
		t.Fatalf("missing service name: %v", err)
	}
}

func TestValidateSkipsCacheLogQueue(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Cache.Options.Capacity = -5
	cfg.Queue.Options.Buffer = -5
	if err := cfg.Validate(); err != nil {
		t.Fatalf("cache/log/queue have no options Validate: %v", err)
	}
}

func TestValidateWrapsTypedErrors(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.RateLimit.Options.Rate = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "ratelimit") {
		t.Fatalf("missing service name: %v", err)
	}
}

func TestValidateRateLimitBadTrustedProxy(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.RateLimit.Options.TrustedProxies = []string{"not-a-cidr"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("want error for malformed trusted_proxies entry")
	}
	if !strings.Contains(err.Error(), "ratelimit") {
		t.Fatalf("missing service name: %v", err)
	}
	if !strings.Contains(err.Error(), "trusted_proxies") {
		t.Fatalf("missing field name: %v", err)
	}
}

func TestValidateRateLimitGoodTrustedProxies(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.RateLimit.Options.TrustedProxies = []string{"10.0.0.0/8", "2001:db8::/32"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid trusted_proxies must validate: %v", err)
	}
}
func TestLoadRateLimitTrustedProxies(t *testing.T) {
	t.Parallel()
	p := writeFile(t, t.TempDir(), "zever.yaml",
		"ratelimit:\n  adapter: memory\n  options:\n    rate: 10\n    burst: 20\n    trusted_proxies:\n      - 10.0.0.0/8\n      - 2001:db8::/32\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.RateLimit.Options.TrustedProxies) != 2 {
		t.Fatalf("TrustedProxies = %v, want 2 entries", cfg.RateLimit.Options.TrustedProxies)
	}
}

func TestLoadRateLimitBadTrustedProxy(t *testing.T) {
	t.Parallel()
	p := writeFile(t, t.TempDir(), "zever.yaml",
		"ratelimit:\n  adapter: memory\n  options:\n    rate: 10\n    burst: 20\n    trusted_proxies:\n      - not-a-cidr\n")
	_, err := Load(p)
	if err == nil {
		t.Fatal("want error for malformed trusted_proxies entry")
	}
	if !strings.Contains(err.Error(), "trusted_proxies") {
		t.Fatalf("error = %q, want it to name trusted_proxies", err)
	}
}

func TestValidateRouterTLSUnpaired(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Router.Options.TLSCertFile = "server.crt"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("want error for tls_cert without tls_key")
	}
	if !strings.Contains(err.Error(), "router") {
		t.Fatalf("missing service name: %v", err)
	}
	if !strings.Contains(err.Error(), "tls_key") {
		t.Fatalf("missing pair reason: %v", err)
	}
}

func TestValidateRouterTLSPaired(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Router.Options.TLSCertFile = "server.crt"
	cfg.Router.Options.TLSKeyFile = "server.key"
	cfg.Router.Options.TLSClientCAFile = "client-ca.crt"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("paired TLS must validate: %v", err)
	}
}
