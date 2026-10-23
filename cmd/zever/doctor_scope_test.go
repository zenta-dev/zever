package main

import (
	"testing"
)

// TestCliDefaultDriverKnownBatteries pins that the CLI wires a default for
// the floor batteries it must always check.
func TestCliDefaultDriverKnownBatteries(t *testing.T) {
	t.Parallel()

	for _, svc := range []string{"log", "router", "db"} {
		if got := cliDefaultDriver(svc); got == "" {
			t.Errorf("cliDefaultDriver(%q) empty, want wired default", svc)
		}
	}
}

// TestCliDriverNamesCoverDefault pins the drivers table lists the wired
// default for every battery that has one.
func TestCliDriverNamesCoverDefault(t *testing.T) {
	t.Parallel()

	for _, svc := range []string{"log", "router", "db", "cache", "queue"} {
		def := cliDefaultDriver(svc)
		if def == "" {
			continue
		}

		found := false
		for _, d := range cliDriverNames(svc) {
			if d == def {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("cliDriverNames(%q) = %v, missing default %q", svc, cliDriverNames(svc), def)
		}
	}

	if got := cliDriverNames("nope"); len(got) != 0 {
		t.Errorf("cliDriverNames(nope) = %v, want nil", got)
	}
}
