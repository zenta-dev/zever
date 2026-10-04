package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDefaultIndependent pins that Default shares no mutable state between
// calls: mutating one config's options must not leak into the next.
func TestDefaultIndependent(t *testing.T) {
	t.Parallel()

	a := Default()
	a.Crypto.Options.Key = "mutated"
	a.DB.Options.MaxConns = 99

	b := Default()
	if b.Crypto.Options.Key == "mutated" {
		t.Fatal("Default crypto key shared between calls")
	}
	if b.DB.Options.MaxConns == 99 {
		t.Fatal("Default db options shared between calls")
	}
}

// TestRedactedServicesPluginCorruptOptions covers the best-effort display
// contract: unparsable raw plugin options degrade to an empty redacted map
// instead of panicking or failing.
func TestRedactedServicesPluginCorruptOptions(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Plugins = map[string]Service[json.RawMessage]{
		"plugintest_corrupt": {Adapter: "custom", Options: json.RawMessage(`{not json`)},
	}

	got := cfg.RedactedServices()
	sc, ok := got["plugintest_corrupt"]
	if !ok {
		t.Fatal("missing plugintest_corrupt")
	}
	if sc.Adapter != "custom" {
		t.Fatalf("adapter = %q, want custom", sc.Adapter)
	}
	if len(sc.Options) != 0 {
		t.Fatalf("corrupt options = %v, want empty map", sc.Options)
	}
}

// TestLoadVeryLongValue is the string-length boundary: a 1 MiB env value for
// a string field must be accepted without truncation.
func TestLoadVeryLongValue(t *testing.T) {
	seedHostileEnv(t)
	long := strings.Repeat("x", 1<<20)
	t.Setenv("STORAGE_ROOT", long)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Storage.Options.Root != long {
		t.Fatalf("value truncated: got %d bytes, want %d", len(cfg.Storage.Options.Root), len(long))
	}
}
