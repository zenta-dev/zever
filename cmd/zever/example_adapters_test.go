package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleAppsRejectAdapterEnv is the guard that no example app selects
// its adapters through the environment. Adapter selection is compile-time:
// each app imports its adapter packages and wires them with
// container.Provide in internal/app, then rebuilds to change backends.
// Config is options-only, so any <SERVICE>_ADAPTER or
// <SERVICE>__<NAME>__ADAPTER key in an example's .env.example would fail
// resolution (config.Resolve rejects every *_ADAPTER key with an
// adapter-selection-removed error); this test catches such a line here
// instead of at boot time.
func TestExampleAppsRejectAdapterEnv(t *testing.T) {
	t.Parallel()

	root := repoRootAbs(t)

	envs, err := filepath.Glob(filepath.Join(root, "examples", "*", ".env.example"))
	if err != nil {
		t.Fatalf("glob example env templates: %v", err)
	}

	if len(envs) == 0 {
		t.Fatal("no examples/*/.env.example found")
	}

	for _, envPath := range envs {
		exampleDir := filepath.Dir(envPath)
		name := filepath.Base(exampleDir)

		raw, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read %s: %v", envPath, err)
		}

		if pairs := selectedAdapters(string(raw)); len(pairs) > 0 {
			t.Errorf("%s: adapter selection is code, not env: remove %q (wire in internal/app with container.Provide)", name, strings.Join(pairs, ", "))
		}
	}
}

// selectedAdapters returns the "<battery>/<adapter>" pairs an .env.example
// still selects through <SERVICE>_ADAPTER or <SERVICE>__<NAME>__ADAPTER.
// Every entry is a rejection: config.Resolve fails such keys with an
// adapter-selection-removed error, so the caller reports them instead of
// matching them against imports.
func selectedAdapters(envExample string) []string {
	var out []string

	for _, line := range strings.Split(envExample, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.ToUpper(strings.TrimSpace(key))
		if !strings.HasSuffix(key, "_ADAPTER") {
			continue
		}

		head, _, _ := strings.Cut(key, "__")
		head = strings.TrimSuffix(head, "_ADAPTER")
		if head == "" {
			continue
		}

		out = append(out, strings.ToLower(head)+"/"+strings.TrimSpace(value))
	}

	return out
}
