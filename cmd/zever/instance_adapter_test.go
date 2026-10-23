package main

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

// TestNamedInstanceAdapterKeysRejected pins the removal for the instance
// namespace: SERVICE__NAME__ADAPTER (and the default SERVICE_ADAPTER) fail
// resolution instead of selecting anything. Named instances inherit the
// code-wired driver; only option keys apply per instance.
func TestNamedInstanceAdapterKeysRejected(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"DB__REPORTING__ADAPTER", "DB_ADAPTER", "db__reporting__adapter"} {
		if _, err := config.Resolve(config.Env{key: "postgres"}); err == nil {
			t.Errorf("Resolve(%s) = nil error, want removal failure", key)
		} else if !strings.Contains(err.Error(), "adapter selection removed") {
			t.Errorf("Resolve(%s) error = %q, want removal message", key, err)
		}
	}
}

// TestNamedInstancesShareCodeWiredDriver pins that named instances carry
// no adapter of their own: redactedInstances rows hold options only, while
// the driver comes from cliDefaultDriver (the compiled-in wiring).
func TestNamedInstancesShareCodeWiredDriver(t *testing.T) {
	t.Parallel()

	env := config.Env{"DB__REPORTING__PATH": "data/reporting.db"}
	services := map[string]config.ServiceConfig{
		"db": {Options: map[string]any{}},
	}

	got := redactedInstances(env, nil, services)

	row, ok := got["db/REPORTING"]
	if !ok {
		t.Fatal("redactedInstances missing db/reporting")
	}

	if row.Options == nil {
		t.Error("db/reporting options = nil, want redacted options map")
	}

	if def := cliDefaultDriver("db"); def == "" {
		t.Error("cliDefaultDriver(db) empty, want wired default")
	}
}
