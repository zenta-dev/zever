package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExtractDryRunWritesNothing(t *testing.T) {
	dir, _ := setupExtractProject(t)

	if err := runExtract([]string{"billing", "--dry-run"}); err != nil {
		t.Fatalf("runExtract --dry-run: %v", err)
	}

	out := filepath.Join(dir, "billing-service")
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("dry run created %s", out)
	}
}

func TestPrintExtractDryRunMentionsPlan(t *testing.T) {
	dir, _ := setupExtractProject(t)

	out := captureStdout(t, func() {
		if err := runExtract([]string{"billing", "--dry-run"}); err != nil {
			t.Fatalf("runExtract --dry-run: %v", err)
		}
	})
	_ = dir

	for _, want := range []string{"dry run", "billing-service"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestNewFlagSetHasDryRun(t *testing.T) {
	t.Parallel()

	if f := newNewFlagSet().Lookup("dry-run"); f == nil {
		t.Fatal("new flag set missing --dry-run")
	}
}

func TestAddFlagSetHasDryRun(t *testing.T) {
	t.Parallel()

	if f := newAddFlagSet().Lookup("dry-run"); f == nil {
		t.Fatal("add flag set missing --dry-run")
	}
}

func TestPrintNewDryRunMentionsPlan(t *testing.T) {
	t.Parallel()

	out := captureStdout(t, func() {
		printNewDryRun(NewConfig{Name: "demo", OutDir: "./demo", ModulePath: "example.com/demo", Batteries: []string{"log", "router"}}, "./demo")
	})

	for _, want := range []string{"dry run", "demo", "example.com/demo", "log, router"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}
