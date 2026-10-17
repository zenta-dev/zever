package main

import (
	"strings"
	"testing"
)

func TestRunScheduleRunIsAliasForQueueWork(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  worker_entry: cmd/jobs\n")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runScheduleRun([]string{"-v"}); err != nil {
		t.Fatalf("runScheduleRun: %v", err)
	}

	assertOneLaunch(t, calls, defaultWorkerEntry, []string{"-v"})
}

func TestRunScheduleRunHelpStatesTheAlias(t *testing.T) {
	t.Chdir(t.TempDir())

	calls := stubLaunch(t, nil)

	if err := runScheduleRun([]string{"-h"}); err != nil {
		t.Fatalf("runScheduleRun -h: %v", err)
	}

	if len(*calls) != 0 {
		t.Fatalf("-h launched a child: %+v", *calls)
	}

	if got := scheduleRunHelp; !strings.Contains(got, "alias for worker") {
		t.Fatalf("schedule run help must state the alias plainly:\n%s", got)
	}
}

// TestRunScheduleRunIgnoresBadLegacyConfig proves legacy config errors no
// longer propagate through the alias: layout is defaults-only.
func TestRunScheduleRunIgnoresBadLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  worker_entry: [unclosed\n")
	t.Chdir(dir)

	calls := stubLaunch(t, nil)

	if err := runScheduleRun(nil); err != nil {
		t.Fatalf("runScheduleRun: %v", err)
	}

	assertOneLaunch(t, calls, defaultWorkerEntry, nil)
}
