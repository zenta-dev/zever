package main

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
)

// jobModuleFixture declares one parameterless job, the precondition every
// successful `generate schedule` has.
const jobModuleFixture = "job Ship() {\n\tqueue: default\n}\n"

func TestRunGenerateScheduleAppendsDeclaration(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", jobModuleFixture)

	if err := runGenerateSchedule([]string{"shop", "Nightly", "--cron", "0 0 * * *", "--dispatch", "Ship"}); err != nil {
		t.Fatalf("runGenerateSchedule: %v", err)
	}

	got := readFile(t, path)

	const want = "schedule Nightly {\n" +
		"\tcron: \"0 0 * * *\"\n" +
		"\tdispatch: Ship()\n" +
		"}\n"

	if !strings.HasSuffix(got, want) {
		t.Fatalf("content =\n%s\nwant suffix\n%s", got, want)
	}

	assertZenParses(t, path)
}

// TestRunGenerateScheduleRejectsUnknownJob is the point of the command's
// pre-check: the resolver would reject a schedule dispatching a job that does
// not exist, so the CLI refuses to write it in the first place.
func TestRunGenerateScheduleRejectsUnknownJob(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", jobModuleFixture)

	err := runGenerateSchedule([]string{"shop", "Nightly", "--cron", "0 0 * * *", "--dispatch", "Pack"})
	if err == nil {
		t.Fatalf("expected an error for a job the module does not declare")
	}

	// The error must name the jobs that do exist, so a typo is obvious.
	if !strings.Contains(err.Error(), "declared jobs: Ship") {
		t.Fatalf("error lacks the job hint: %v", err)
	}

	if got := readFile(t, path); got != jobModuleFixture {
		t.Fatalf("file was modified: %q", got)
	}
}

func TestRunGenerateScheduleRejectsJobWithParameters(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	const original = "job Ship(order_id: uuid) {\n\tqueue: default\n}\n"

	path := writeModuleFixture(t, dir, "shop", original)

	err := runGenerateSchedule([]string{"shop", "Nightly", "--cron", "0 0 * * *", "--dispatch", "Ship"})
	if err == nil {
		t.Fatalf("expected an error for a job that takes parameters")
	}

	if !strings.Contains(err.Error(), "parameter") {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readFile(t, path); got != original {
		t.Fatalf("file was modified: %q", got)
	}
}

func TestRunGenerateScheduleArgumentErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	writeModuleFixture(t, dir, "shop", jobModuleFixture)

	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"missing schedule name", []string{"shop", "--cron", "0 0 * * *", "--dispatch", "Ship"}},
		{"missing cron", []string{"shop", "Nightly", "--dispatch", "Ship"}},
		{"blank cron", []string{"shop", "Nightly", "--cron", "   ", "--dispatch", "Ship"}},
		{"cron with a quote", []string{"shop", "Nightly", "--cron", `0 0 " * *`, "--dispatch", "Ship"}},
		{"cron with a backslash", []string{"shop", "Nightly", "--cron", `0 0 \ * *`, "--dispatch", "Ship"}},
		{"missing dispatch", []string{"shop", "Nightly", "--cron", "0 0 * * *"}},
		{"dispatch is not an identifier", []string{"shop", "Nightly", "--cron", "0 0 * * *", "--dispatch", "Ship()"}},
		{"name is not an identifier", []string{"shop", "night-ly", "--cron", "0 0 * * *", "--dispatch", "Ship"}},
		{"unknown module", []string{"warehouse", "Nightly", "--cron", "0 0 * * *", "--dispatch", "Ship"}},
		{"duplicate name", []string{"shop", "Ship", "--cron", "0 0 * * *", "--dispatch", "Ship"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := runGenerateSchedule(tc.args); err == nil {
				t.Fatalf("expected an error for %v", tc.args)
			}
		})
	}
}

func TestRenderScheduleDecl(t *testing.T) {
	got := renderScheduleDecl("Nightly", "*/5 * * * *", "Ship")

	const want = "schedule Nightly {\n" +
		"\tcron: \"*/5 * * * *\"\n" +
		"\tdispatch: Ship()\n" +
		"}\n"

	if got != want {
		t.Fatalf("renderScheduleDecl = %q, want %q", got, want)
	}

	assertGolden(t, "generate_schedule_decl.golden", []byte(got))
}

func TestJobHintWithNoJobs(t *testing.T) {
	if got := jobHint(&ast.File{}); !strings.Contains(got, "no jobs at all") {
		t.Fatalf("jobHint = %q", got)
	}
}

func TestFindJobDecl(t *testing.T) {
	module := &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{Name: "Order"},
		&ast.JobDecl{Name: "Ship"},
	}}

	if job := findJobDecl(module, "Ship"); job == nil || job.Name != "Ship" {
		t.Fatalf("findJobDecl did not find the job: %+v", job)
	}

	// An entity of the same name is not a job.
	if job := findJobDecl(module, "Order"); job != nil {
		t.Fatalf("findJobDecl matched an entity: %+v", job)
	}
}

// TestCoverGenerateSchedulePure covers traversal/ident/cron/dispatch/taken.
func TestCoverGenerateSchedulePure(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "../e", Name: "S", Cron: "x", Dispatch: "J"}); !isTraversalError(err) {
		t.Fatalf("want traversal %v", err)
	}
	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "shop", Name: "bad!", Cron: "x", Dispatch: "J"}); err == nil {
		t.Fatalf("want ident error")
	}
	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "shop", Name: "S", Cron: "   ", Dispatch: "J"}); err == nil {
		t.Fatalf("want cron error")
	}
	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "shop", Name: "S", Cron: "a\"b", Dispatch: "J"}); err == nil {
		t.Fatalf("want cron quote error")
	}
	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "shop", Name: "S", Cron: "x", Dispatch: "bad!"}); err == nil {
		t.Fatalf("want dispatch error")
	}
}

// TestCoverRunGenerateScheduleInteractive covers all prompt branches.
func TestCoverRunGenerateScheduleInteractive(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)
	origSel, origIn := promptSelectForSchedule, promptInputForSchedule
	t.Cleanup(func() { promptSelectForSchedule, promptInputForSchedule = origSel, origIn })
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeModuleFixture(t, dir, "shop", jobModuleFixture)
	// Full interactive: module select, name input, cron input, dispatch select.
	promptSelectForSchedule = func(title string, _ []string) (string, error) {
		if title == "Module" {
			return "shop", nil
		}
		return "Ship", nil
	}
	promptInputForSchedule = func(title, _ string, _ func(string) error) (string, error) {
		switch title {
		case "Schedule name":
			return "PromptedSched", nil
		case "Cron spec":
			return "*/5 * * * *", nil
		case "Module name":
			return "shop", nil
		default:
			return "x", nil
		}
	}
	if err := runGenerateSchedule(nil); err != nil {
		t.Fatalf("interactive schedule: %v", err)
	}
	// Module input error (no mods).
	empty := t.TempDir()
	withWorkingDir(t, empty)
	promptSelectForSchedule = func(string, []string) (string, error) { return "", errTestSentinel }
	promptInputForSchedule = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }
	if err := runGenerateSchedule(nil); err == nil {
		t.Fatalf("want module error")
	}
	// Cron prompt error.
	dir2 := t.TempDir()
	withWorkingDir(t, dir2)
	writeModuleFixture(t, dir2, "shop", jobModuleFixture)
	promptInputForSchedule = func(title, _ string, _ func(string) error) (string, error) {
		if title == "Cron spec" {
			return "", errTestSentinel
		}
		if title == "Schedule name" {
			return "S2", nil
		}
		return "shop", nil
	}
	promptSelectForSchedule = func(string, []string) (string, error) { return "shop", nil }
	if err := runGenerateSchedule([]string{"shop", "S2"}); err == nil {
		t.Fatalf("want cron error")
	}
	// Dispatch via job list select success.
	promptSelectForSchedule = func(title string, _ []string) (string, error) {
		if title == "Dispatch job" {
			return "Ship", nil
		}
		return "shop", nil
	}
	promptInputForSchedule = func(title, _ string, _ func(string) error) (string, error) {
		if title == "Cron spec" {
			return "0 0 * * *", nil
		}
		return "x", nil
	}
	if err := runGenerateSchedule([]string{"shop", "S3", "--cron", "0 0 * * *"}); err != nil {
		t.Fatalf("dispatch select: %v", err)
	}
	// Dispatch input when no jobs.
	dir3 := t.TempDir()
	withWorkingDir(t, dir3)
	writeModuleFixture(t, dir3, "empty", "entity E {\n\tid: uuid @primary\n}\n")
	promptSelectForSchedule = func(string, []string) (string, error) { return "shop", nil }
	promptInputForSchedule = func(title, _ string, _ func(string) error) (string, error) {
		if title == "Dispatch job name" {
			return "MyJob", nil
		}
		if title == "Cron spec" {
			return "0 0 * * *", nil
		}
		return "x", nil
	}
	_ = runGenerateSchedule([]string{"empty", "S4", "--cron", "0 0 * * *"})
	// Dispatch input error.
	promptInputForSchedule = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }
	if err := runGenerateSchedule([]string{"empty", "S5", "--cron", "0 0 * * *"}); err == nil {
		t.Fatalf("want dispatch input error")
	}
	// Parse error.
	if err := runGenerateSchedule([]string{"--badflag"}); err == nil {
		t.Fatalf("want parse error")
	}
}
