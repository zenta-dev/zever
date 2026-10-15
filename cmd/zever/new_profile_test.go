package main

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestProfileBatteriesKnown proves every --profile name resolves to a battery
// set drawn from real services, and unknown profiles fail closed with the
// choices listed.
func TestProfileBatteriesKnown(t *testing.T) {
	t.Parallel()

	for _, profile := range []string{"minimal", "api", "full"} {
		extras, err := profileBatteries(profile)
		if err != nil {
			t.Fatalf("profileBatteries(%q): %v", profile, err)
		}

		for _, battery := range extras {
			if _, ok := allServiceAdapters[battery]; !ok {
				t.Errorf("profile %q lists unknown battery %q", profile, battery)
			}
		}
	}

	if _, err := profileBatteries("huge"); err == nil {
		t.Fatal("profileBatteries(huge) = nil, want unknown-profile error")
	} else if !strings.Contains(err.Error(), "minimal") || !strings.Contains(err.Error(), "api") || !strings.Contains(err.Error(), "full") {
		t.Fatalf("unknown-profile error = %q, want choices listed", err.Error())
	}
}

// TestProfileBatteriesOrdered proves the profile sets nest: minimal is the
// boot set alone, api adds the service batteries a JSON API needs, and full
// covers every known service.
func TestProfileBatteriesOrdered(t *testing.T) {
	t.Parallel()

	boot := batteriesFor(nil, "")

	minimal, err := profileBatteries("minimal")
	if err != nil {
		t.Fatalf("profileBatteries(minimal): %v", err)
	}

	if got := batteriesFor(minimal, ""); !equalStringSlices(got, boot) {
		t.Fatalf("minimal resolves to %v, want boot set %v", got, boot)
	}

	api, err := profileBatteries("api")
	if err != nil {
		t.Fatalf("profileBatteries(api): %v", err)
	}

	if len(api) == 0 {
		t.Fatal("api profile adds no batteries")
	}

	for _, want := range []string{"cache", "queue", "session", "password", "scheduler"} {
		if !batteryPicked(api, want) {
			t.Errorf("api profile lacks %q: %v", want, api)
		}
	}

	full, err := profileBatteries("full")
	if err != nil {
		t.Fatalf("profileBatteries(full): %v", err)
	}

	fullSet := batteriesFor(full, "")

	// Services with no zero-infra adapter (they need an API key, a dataset
	// or an embedded FS supplied from code) would scaffold a config that
	// fails `zever doctor`, so the full profile leaves them for `zever add`.
	needInputs := map[string]bool{"ai": true, "geo": true, "i18n": true}

	if want := len(allServiceAdapters) - len(needInputs); len(fullSet) != want {
		t.Fatalf("full resolves to %d batteries, want %d (all services but %v)", len(fullSet), want, needInputs)
	}

	for battery := range allServiceAdapters {
		picked := batteryPicked(fullSet, battery)
		if needInputs[battery] && picked {
			t.Errorf("full profile wires %q, which cannot resolve from zever.yaml alone", battery)
		}

		if !needInputs[battery] && !picked {
			t.Errorf("full profile missing service %q", battery)
		}
	}
}

// TestProfileBatteriesSorted proves profile extras are sorted, so go.mod
// requires and zever.yaml stay deterministic across runs.
func TestProfileBatteriesSorted(t *testing.T) {
	t.Parallel()

	for _, profile := range []string{"api", "full"} {
		extras, err := profileBatteries(profile)
		if err != nil {
			t.Fatalf("profileBatteries(%q): %v", profile, err)
		}

		if !sort.StringsAreSorted(extras) {
			t.Errorf("profile %q extras not sorted: %v", profile, extras)
		}
	}
}

// equalStringSlices reports whether a and b hold the same strings in order.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// TestProfileAPIScaffoldWiresBatteries proves the api preset flows end to
// end: profile extras merged through batteriesFor land in zever.yaml with
// default adapters.
func TestProfileAPIScaffoldWiresBatteries(t *testing.T) {
	t.Parallel()

	extras, err := profileBatteries("api")
	if err != nil {
		t.Fatalf("profileBatteries(api): %v", err)
	}

	cfg := scaffoldTestConfig(t, "v0.6.1")
	cfg.Batteries = batteriesFor(extras, "")

	if _, err := writeNewProject("zever new", cfg); err != nil {
		t.Fatalf("writeNewProject: %v", err)
	}

	var doc map[string]batteryYAML
	if err := yaml.Unmarshal([]byte(readFile(t, filepath.Join(cfg.OutDir, "zever.yaml"))), &doc); err != nil {
		t.Fatalf("parse zever.yaml: %v", err)
	}

	for _, want := range append([]string(nil), apiProfileBatteries...) {
		entry, ok := doc[want]
		if !ok {
			t.Errorf("zever.yaml lacks api battery %q", want)
			continue
		}

		if entry.Adapter != defaultServiceAdapter(want) {
			t.Errorf("zever.yaml %s adapter = %q, want default %q", want, entry.Adapter, defaultServiceAdapter(want))
		}
	}
}

// TestRunNewUnknownProfileFailsClosed proves a typo'd --profile never
// scaffolds the floor set silently. Not parallel: withWorkingDir chdirs.
func TestRunNewUnknownProfileFailsClosed(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := runNew([]string{"typoapp", "--profile", "huge", "--no-tidy"}); err == nil {
		t.Fatal("runNew --profile huge = nil, want unknown-profile error")
	}
}
