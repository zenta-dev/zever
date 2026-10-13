package main

import (
	"strings"
	"testing"
)

// TestP32NewNoTidyFlag proves --no-tidy skips go mod tidy and is registered.
func TestP32NewNoTidyFlag(t *testing.T) {
	fs := newNewFlagSet()
	if fs.Lookup("no-tidy") == nil {
		t.Fatalf("--no-tidy flag not defined")
	}

	// With stubbed tidy, --no-tidy must not call the runner.
	called := false
	prev := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { called = true; return nil }
	defer func() { runGoModTidyFunc = prev }()

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	if err := runNew([]string{"notidyapp", "--no-tidy"}); err != nil {
		t.Fatalf("runNew --no-tidy: %v", err)
	}
	if called {
		t.Fatalf("--no-tidy still ran go mod tidy")
	}

	// Without the flag, the runner is called.
	called = false
	if err := runNew([]string{"tidyapp"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}
	if !called {
		t.Fatalf("runNew without --no-tidy did not run go mod tidy")
	}
}

// TestP32NewTidyFailureActionable proves tidy failures carry the fix.
func TestP32NewTidyFailureActionable(t *testing.T) {
	prev := runGoModTidyFunc
	runGoModTidyFunc = func(tag, _ string) error {
		return runGoModTidy(tag, "/nonexistent-dir-for-p32-test")
	}
	defer func() { runGoModTidyFunc = prev }()

	// Direct unit: runGoModTidy on a bad dir must mention `go mod tidy`.
	err := runGoModTidy("zever new", "/nonexistent-dir-for-p32-test")
	if err == nil {
		t.Fatal("expected tidy error")
	}
	if !strings.Contains(err.Error(), "go mod tidy") {
		t.Fatalf("tidy error %q lacks fix", err)
	}
}
