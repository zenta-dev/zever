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

func TestRunQueueWorkUsesConfiguredWorkerEntry(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  worker_entry: cmd/jobs\n")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runQueueWork(nil); err != nil {
		t.Fatalf("runQueueWork: %v", err)
	}

	assertOneLaunch(t, calls, "cmd/jobs", nil)
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

// TestRunQueueWorkBadConfig proves a broken project file surfaces as an error.
func TestRunQueueWorkBadConfig(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  worker_entry: [unclosed\n")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runQueueWork(nil); err == nil {
		t.Fatal("expected a config error, got nil")
	}

	if len(*calls) != 0 {
		t.Fatalf("bad config launched a child: %+v", *calls)
	}
}
