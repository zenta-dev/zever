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
	cfg.DB.Adapter = "bogus"
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
	cfg.Router.Adapter = "bogus"
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
