package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// removedDBAdapterNames are the per-backend adapter names the db-backed
// batteries used to offer. They never selected a backend -- the adapter
// picks PostgreSQL or embedded SQLite from the DSN -- so they only ever
// named one, and they are gone. The canonical "db" replaces them.
var removedDBAdapterNames = map[string][]string{
	"search":      {"postgres", "sqlite"},
	"vectorstore": {"pgvector", "sqlite"},
	"workflow":    {"postgres"},
}

// TestBatteryAdaptersOfferOnlyCanonicalNames pins the offer list: every
// name the scaffolder offers must resolve at runtime, so the db-backed
// batteries offer "db" (the backend follows the DSN) and never a
// per-backend name.
func TestBatteryAdaptersOfferOnlyCanonicalNames(t *testing.T) {
	t.Parallel()

	all := batteryAdapters()

	for battery, removed := range removedDBAdapterNames {
		choices := all[battery]

		for _, name := range removed {
			if batteryPicked(choices, name) {
				t.Errorf("batteryAdapters[%q] = %v, must not offer the removed name %q", battery, choices, name)
			}
		}

		if !batteryPicked(choices, "db") {
			t.Errorf("batteryAdapters[%q] = %v, want the canonical %q", battery, choices, "db")
		}
	}

	// The db battery's own sqlite and postgres adapters are real, distinct
	// databases (not aliases of one), so they stay.
	db := all["db"]
	if !batteryPicked(db, "sqlite") || !batteryPicked(db, "postgres") {
		t.Errorf("batteryAdapters[db] = %v, want sqlite and postgres", db)
	}
}

// TestRemovedAdapterNameRejected proves a removed name fails closed on
// every pick path (the --with pin, the --adapters pair and the `zever add`
// target), with the canonical replacement in the message.
func TestRemovedAdapterNameRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		battery string
		adapter string
	}{
		{"search", "postgres"},
		{"search", "sqlite"},
		{"vectorstore", "pgvector"},
		{"vectorstore", "sqlite"},
		{"workflow", "postgres"},
	}

	for _, tc := range cases {
		t.Run(tc.battery+"/"+tc.adapter, func(t *testing.T) {
			t.Parallel()

			pin := tc.battery + "=" + tc.adapter

			_, _, err := parseWithFlag(pin)
			assertRemovedAdapterError(t, err, tc.adapter)

			_, err = parseAdaptersFlag(pin)
			assertRemovedAdapterError(t, err, tc.adapter)

			_, err = parseAddTarget(tc.battery+"/"+tc.adapter, "", "", "")
			assertRemovedAdapterError(t, err, tc.adapter)
		})
	}
}

// TestRunNewRejectsRemovedAdapterName proves the scaffold never writes a
// project whose own adapter does not resolve: a removed name fails before
// any file is written.
func TestRunNewRejectsRemovedAdapterName(t *testing.T) {
	resetInteractiveMode(t)

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	err := runNew([]string{"aliasapp", "--with", "vectorstore=pgvector"})
	if err == nil {
		t.Fatal("runNew --with vectorstore=pgvector = nil error, want unknown adapter error")
	}

	assertRemovedAdapterError(t, err, "pgvector")

	if _, statErr := os.Stat(filepath.Join(workDir, "aliasapp")); !os.IsNotExist(statErr) {
		t.Errorf("runNew wrote a project for a removed adapter name (stat error: %v)", statErr)
	}
}

// assertRemovedAdapterError checks that err is the unknown-adapter error
// for the given name and that it points at the canonical replacement.
func assertRemovedAdapterError(t *testing.T, err error, adapter string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected an unknown-adapter error for %q, got nil", adapter)
	}

	msg := err.Error()

	if !strings.Contains(msg, `unknown adapter "`+adapter+`"`) {
		t.Errorf("error %q does not name the rejected adapter %q", msg, adapter)
	}

	if !strings.Contains(msg, `removed name, use "db"`) {
		t.Errorf("error %q does not point at the canonical replacement %q", msg, "db")
	}
}
