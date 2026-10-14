package main

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

// stubRegistered pretends only log/slog and router/stdhttp exist in the
// process, like a `zever build --adapters=log,router` slim binary.
func stubRegistered(service string) []string {
	switch service {
	case "log":
		return []string{"noop", "pretty", "slog", "zerolog"}
	case "router":
		return []string{"fiber", "stdhttp"}
	default:
		return nil
	}
}

// TestScopedValidateAdaptersEnabledOnlyPasses pins that a slim registry
// passes the adapters check when the project enables only included
// batteries.
func TestScopedValidateAdaptersEnabledOnlyPasses(t *testing.T) {
	t.Parallel()

	resolved := config.Default()

	err := scopedValidateAdapters(resolved, map[string]bool{"log": true, "router": true}, stubRegistered)
	if err != nil {
		t.Fatalf("scopedValidateAdapters enabled subset error = %v", err)
	}
}

// TestScopedValidateAdaptersNilEnabledChecksAll pins that without a project
// config the check keeps its strict all-battery behavior.
func TestScopedValidateAdaptersNilEnabledChecksAll(t *testing.T) {
	t.Parallel()

	resolved := config.Default()

	err := scopedValidateAdapters(resolved, nil, stubRegistered)
	if err == nil {
		t.Fatal("scopedValidateAdapters nil enabled = nil error, want unknown adapter failures")
	}

	if !strings.Contains(err.Error(), "cache") {
		t.Errorf("error %q missing excluded battery", err.Error())
	}
}

// TestScopedValidateAdaptersEnabledMissingFails pins that an enabled
// battery missing from the registry still fails closed.
func TestScopedValidateAdaptersEnabledMissingFails(t *testing.T) {
	t.Parallel()

	resolved := config.Default()

	err := scopedValidateAdapters(resolved, map[string]bool{"log": true, "cache": true}, stubRegistered)
	if err == nil {
		t.Fatal("scopedValidateAdapters enabled cache = nil error, want failure")
	}

	if !strings.Contains(err.Error(), "cache") {
		t.Errorf("error %q missing enabled battery", err.Error())
	}
}
