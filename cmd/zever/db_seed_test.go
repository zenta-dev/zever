package main

import "testing"

func TestRunDBSeedUsesDefaultSeedEntry(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	if err := runDBSeed([]string{"--truncate"}); err != nil {
		t.Fatalf("runDBSeed: %v", err)
	}

	assertOneLaunch(t, calls, defaultSeedEntry, []string{"--truncate"})
}

func TestRunDBSeedUsesConfiguredSeedEntry(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.json", `{"project":{"seed_entry":"db/fixtures"}}`)
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runDBSeed(nil); err != nil {
		t.Fatalf("runDBSeed: %v", err)
	}

	assertOneLaunch(t, calls, "db/fixtures", nil)
}

// TestRunDBSeedThroughDispatcher proves `zever db seed` reaches runDBSeed via
// the db.go dispatcher, not just that runDBSeed works in isolation.
func TestRunDBSeedThroughDispatcher(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	if err := runDB([]string{"seed", "--only=users"}); err != nil {
		t.Fatalf("runDB seed: %v", err)
	}

	assertOneLaunch(t, calls, defaultSeedEntry, []string{"--only=users"})
}

func TestRunDBSeedHelpDoesNotLaunch(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	if err := runDB([]string{"seed", "-h"}); err != nil {
		t.Fatalf("runDB seed -h: %v", err)
	}

	if len(*calls) != 0 {
		t.Fatalf("-h launched a child: %+v", *calls)
	}
}

// TestResolveSeedConfig pins the XConfig refactor: no side effects.
func TestResolveSeedConfig(t *testing.T) {
	t.Parallel()

	cfg := resolveSeedConfig(ProjectConfig{SeedEntry: "db/fixtures"}.withDefaults(), []string{"--only=users"})

	if cfg.Entry != "db/fixtures" {
		t.Fatalf("Entry = %q, want %q", cfg.Entry, "db/fixtures")
	}
}

// TestRunDBSeedBadConfig proves a broken project file surfaces as an error.
func TestRunDBSeedBadConfig(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.json", "{bad json")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runDBSeed(nil); err == nil {
		t.Fatal("expected a config error, got nil")
	}

	if len(*calls) != 0 {
		t.Fatalf("bad config launched a child: %+v", *calls)
	}
}
