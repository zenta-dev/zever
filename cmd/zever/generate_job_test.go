package main

import (
	"strings"
	"testing"
)

func TestRunGenerateJobAppendsDeclaration(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "")

	if err := runGenerateJob([]string{"shop", "Ship"}); err != nil {
		t.Fatalf("runGenerateJob: %v", err)
	}

	const want = "job Ship() {\n" +
		"\tqueue: default\n" +
		"\tretry: max_attempts(3), backoff(exponential, base: 30s)\n" +
		"}\n"

	if got := readFile(t, path); got != want {
		t.Fatalf("content =\n%q\nwant\n%q", got, want)
	}

	assertZenParses(t, path)
}

func TestRunGenerateJobHonoursQueueFlag(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "")

	if err := runGenerateJob([]string{"--queue", "shipping", "shop", "Ship"}); err != nil {
		t.Fatalf("runGenerateJob: %v", err)
	}

	got := readFile(t, path)
	if !strings.Contains(got, "\tqueue: shipping\n") {
		t.Fatalf("queue flag was not honoured:\n%s", got)
	}

	assertZenParses(t, path)
}

// TestRunGenerateJobAppendsAfterExistingDecl checks the blank-line separator
// between the existing declaration and the appended job.
func TestRunGenerateJobAppendsAfterExistingDecl(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "entity Order {\n\tid: uuid @primary\n}\n")

	if err := runGenerateJob([]string{"shop", "Ship"}); err != nil {
		t.Fatalf("runGenerateJob: %v", err)
	}

	got := readFile(t, path)

	if !strings.Contains(got, "}\n\njob Ship() {") {
		t.Fatalf("expected a blank-line separated job:\n%s", got)
	}

	if !strings.HasSuffix(got, "}\n") {
		t.Fatalf("job was not appended at EOF:\n%s", got)
	}

	assertZenParses(t, path)
}

func TestRunGenerateJobArgumentErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	writeModuleFixture(t, dir, "shop", "job Ship() {\n\tqueue: default\n}\n")

	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"missing job name", []string{"shop"}},
		{"module is not an identifier", []string{"sh op", "Ship"}},
		{"name is not an identifier", []string{"shop", "Ship!"}},
		{"queue is not an identifier", []string{"shop", "Pack", "--queue", "high-priority"}},
		{"duplicate name", []string{"shop", "Ship"}},
		{"unknown module", []string{"warehouse", "Ship"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := runGenerateJob(tc.args); err == nil {
				t.Fatalf("expected an error for %v", tc.args)
			}
		})
	}
}

func TestRenderJobDecl(t *testing.T) {
	got := renderJobDecl("Ship", "shipping")

	const want = "job Ship() {\n" +
		"\tqueue: shipping\n" +
		"\tretry: max_attempts(3), backoff(exponential, base: 30s)\n" +
		"}\n"

	if got != want {
		t.Fatalf("renderJobDecl = %q, want %q", got, want)
	}

	assertGolden(t, "generate_job_decl.golden", []byte(got))
}

// TestCoverGenerateJobPureErrors covers traversal/ident/queue/taken paths.
func TestCoverGenerateJobPureErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	if _, err := GenerateJob(GenerateJobConfig{Module: "../e", Name: "J", Queue: "default"}); !isTraversalError(err) {
		t.Fatalf("want traversal %v", err)
	}
	if _, err := GenerateJob(GenerateJobConfig{Module: "shop", Name: "bad!", Queue: "default"}); err == nil {
		t.Fatalf("want ident error")
	}
	if _, err := GenerateJob(GenerateJobConfig{Module: "shop", Name: "J", Queue: "bad-queue"}); err == nil {
		t.Fatalf("want queue error")
	}
}

// TestCoverRunGenerateJobInteractive covers module/job/queue prompts.
func TestCoverRunGenerateJobInteractive(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)
	origSel, origIn := promptSelectForJob, promptInputForJob
	t.Cleanup(func() { promptSelectForJob, promptInputForJob = origSel, origIn })
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeModuleFixture(t, dir, "shop", "")
	// No mods discovered? discoverModules finds shop; promptSelect returns shop.
	promptSelectForJob = func(string, []string) (string, error) { return "shop", nil }
	promptInputForJob = func(title, _ string, _ func(string) error) (string, error) {
		if title == "Job name" {
			return "PromptedJob", nil
		}
		if title == "Queue" {
			return "custom", nil
		}
		return "shop", nil
	}
	if err := runGenerateJob(nil); err != nil {
		t.Fatalf("interactive job: %v", err)
	}
	// Select error -> falls back to input? Actually code returns err.
	promptSelectForJob = func(string, []string) (string, error) { return "", errTestSentinel }
	promptInputForJob = func(string, string, func(string) error) (string, error) { return "shop", nil }
	// With no mods? Force empty by chdir to empty dir and select error.
	empty := t.TempDir()
	withWorkingDir(t, empty)
	if err := runGenerateJob(nil); err == nil {
		t.Fatalf("want select error")
	}
	// Input error for module name when no mods.
	promptInputForJob = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }
	if err := runGenerateJob(nil); err == nil {
		t.Fatalf("want module input error")
	}
	// Job name input error.
	dir2 := t.TempDir()
	withWorkingDir(t, dir2)
	writeModuleFixture(t, dir2, "shop", "")
	promptInputForJob = func(title, _ string, _ func(string) error) (string, error) {
		if title == "Module name" {
			return "shop", nil
		}
		return "", errTestSentinel
	}
	// Need select to succeed for module? Actually with mods present, first prompt is select.
	promptSelectForJob = func(string, []string) (string, error) { return "shop", nil }
	if err := runGenerateJob([]string{"shop"}); err == nil {
		t.Fatalf("want job name error")
	}
	// Queue prompt error path (queue default triggers prompt).
	promptInputForJob = func(title, _ string, _ func(string) error) (string, error) {
		if title == "Queue" {
			return "", errTestSentinel
		}
		return "x", nil
	}
	if err := runGenerateJob([]string{"shop"}); err == nil {
		t.Fatalf("want queue/job error")
	}
	// Parse error.
	if err := runGenerateJob([]string{"--badflag"}); err == nil {
		t.Fatalf("want parse error")
	}
}
