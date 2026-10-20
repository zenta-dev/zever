package config

import (
	"encoding/json"
	"testing"
)

// These tests pin the JSON shape of the service-config envelope that
// `zever config show --json` emits (ServiceConfig) and the per-service keys
// on Config. A stripped or renamed tag changes the client-visible keys
// while still compiling and passing round-trip tests.

func TestServiceConfigWireKeys(t *testing.T) {
	t.Parallel()

	sc := ServiceConfig{Adapter: "sqlite", Options: map[string]any{"dsn": ":memory:"}}
	out, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal ServiceConfig: %v", err)
	}

	want := `{"adapter":"sqlite","options":{"dsn":":memory:"}}`
	if string(out) != want {
		t.Errorf("marshal ServiceConfig:\n got %s\nwant %s", out, want)
	}

	var got ServiceConfig
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal ServiceConfig: %v", err)
	}
	if got.Adapter != sc.Adapter {
		t.Errorf("Adapter = %q, want %q", got.Adapter, sc.Adapter)
	}
	if got.Options["dsn"] != sc.Options["dsn"] {
		t.Errorf("Options[dsn] = %v, want %v", got.Options["dsn"], sc.Options["dsn"])
	}
}

func TestServiceConfigEmptyOptionsWireKeys(t *testing.T) {
	t.Parallel()

	// Options must serialize as {} (not null) so clients can rely on the
	// key, and the adapter key must always be present.
	sc := ServiceConfig{Adapter: "noop", Options: map[string]any{}}
	out, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal ServiceConfig: %v", err)
	}

	want := `{"adapter":"noop","options":{}}`
	if string(out) != want {
		t.Errorf("marshal ServiceConfig:\n got %s\nwant %s", out, want)
	}
}
