package config

import (
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

// These tests pin the JSON shape of the service-config envelope that
// `zever config show --json` emits (ServiceConfig) and the per-service keys
// on Config. A stripped or renamed tag changes the client-visible keys
// while still compiling and passing round-trip tests. The envelope is
// options-only: backend selection is compile-time, so there is no adapter
// key on the wire.

func TestServiceConfigWireKeys(t *testing.T) {
	t.Parallel()

	sc := ServiceConfig{Options: map[string]any{"dsn": ":memory:"}}
	out, err := codec.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal ServiceConfig: %v", err)
	}

	want := `{"options":{"dsn":":memory:"}}`
	if string(out) != want {
		t.Errorf("marshal ServiceConfig:\n got %s\nwant %s", out, want)
	}

	var got ServiceConfig
	if err := codec.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal ServiceConfig: %v", err)
	}
	if got.Options["dsn"] != sc.Options["dsn"] {
		t.Errorf("Options[dsn] = %v, want %v", got.Options["dsn"], sc.Options["dsn"])
	}
}

func TestServiceConfigEmptyOptionsWireKeys(t *testing.T) {
	t.Parallel()

	// Options must serialize as {} (not null) so clients can rely on the
	// key.
	sc := ServiceConfig{Options: map[string]any{}}
	out, err := codec.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal ServiceConfig: %v", err)
	}

	want := `{"options":{}}`
	if string(out) != want {
		t.Errorf("marshal ServiceConfig:\n got %s\nwant %s", out, want)
	}
}
