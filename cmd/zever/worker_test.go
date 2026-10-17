package main

import "testing"

func TestRunQueueWorkUsesDefaultWorkerEntry(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	if err := runQueueWork([]string{"--concurrency=8"}); err != nil {
		t.Fatalf("runQueueWork: %v", err)
	}

	assertOneLaunch(t, calls, defaultWorkerEntry, []string{"--concurrency=8"})
}

func TestRunQueueWorkIgnoresLegacyWorkerEntry(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  worker_entry: cmd/jobs\n")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runQueueWork(nil); err != nil {
		t.Fatalf("runQueueWork: %v", err)
	}

	assertOneLaunch(t, calls, defaultWorkerEntry, nil)
}

func TestRunQueueWorkHelpDoesNotLaunch(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	if err := runQueueWork([]string{"help"}); err != nil {
		t.Fatalf("runQueueWork help: %v", err)
	}

	if len(*calls) != 0 {
		t.Fatalf("help launched a child: %+v", *calls)
	}
}

// TestResolveWorkerConfig pins the XConfig refactor: no side effects.
func TestResolveWorkerConfig(t *testing.T) {
	t.Parallel()

	cfg := resolveWorkerConfig(ProjectConfig{WorkerEntry: "cmd/jobs"}.withDefaults(), nil)

	if cfg.Entry != "cmd/jobs" {
		t.Fatalf("Entry = %q, want %q", cfg.Entry, "cmd/jobs")
	}
}

// TestRunQueueWorkIgnoresBadLegacyConfig proves a broken legacy project
// file no longer surfaces: layout is defaults-only, so the default worker
// launches.
func TestRunQueueWorkIgnoresBadLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  worker_entry: [unclosed\n")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runQueueWork(nil); err != nil {
		t.Fatalf("runQueueWork: %v", err)
	}

	assertOneLaunch(t, calls, defaultWorkerEntry, nil)
}
