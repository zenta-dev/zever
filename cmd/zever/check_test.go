package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRunCheckValidSchema(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	entriesBefore, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	var runErr error

	output := inspectCaptureOutput(t, func() {
		runErr = runCheck([]string{schemaPath})
	})

	if runErr != nil {
		t.Fatalf("runCheck: %v", runErr)
	}

	if output == "" {
		t.Fatalf("expected a success message on stdout, got none")
	}

	entriesAfter, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("runCheck wrote files: before=%v after=%v", entriesBefore, entriesAfter)
	}
}

func TestRunCheckInvalidSchema(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "broken.zen", inspectBrokenSchema)

	entriesBefore, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if checkErr := runCheck([]string{schemaPath}); checkErr == nil {
		t.Fatalf("expected error for broken schema, got nil")
	}

	entriesAfter, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("runCheck wrote files: before=%v after=%v", entriesBefore, entriesAfter)
	}
}

func TestRunCheckNoFiles(t *testing.T) {
	err := runCheck(nil)
	if err == nil {
		t.Fatalf("expected error when no input files given, got nil")
	}

	if !errors.Is(err, errNoInputFiles) {
		t.Fatalf("error = %v, want errNoInputFiles", err)
	}
}

func TestRunCheckHelpPrintsUsage(t *testing.T) {
	if err := runCheck([]string{"-h"}); err != nil {
		t.Fatalf("runCheck -h: %v", err)
	}
}

func TestRunCheckWithCoreWritesToOut(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var out bytes.Buffer
	if err := runCheckWith(CheckConfig{Files: []string{schemaPath}, Out: &out}); err != nil {
		t.Fatalf("runCheckWith: %v", err)
	}

	if !strings.Contains(out.String(), "1 file(s) valid") {
		t.Fatalf("output = %q, want validity summary", out.String())
	}
}

func TestRunCheckWithCoreInvalidSchema(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "broken.zen", inspectBrokenSchema)

	var out bytes.Buffer
	err := runCheckWith(CheckConfig{Files: []string{schemaPath}, Out: &out})
	if err == nil {
		t.Fatalf("expected error for broken schema, got nil")
	}

	if !strings.Contains(err.Error(), "failed to compile") {
		t.Fatalf("error = %q, want compile failure", err.Error())
	}
}

func TestRunCheckWithCoreEmptyFiles(t *testing.T) {
	var out bytes.Buffer
	if err := runCheckWith(CheckConfig{Out: &out}); err == nil {
		t.Fatalf("expected error for empty file list, got nil")
	}
}

func TestRunCheckAutoDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, "schema/app.zen", inspectUserSchema)
	t.Chdir(dir)
	t.Setenv("ZEVER_NO_HINT", "1")

	var runErr error
	output := inspectCaptureOutput(t, func() {
		runErr = runCheck(nil)
	})
	if runErr != nil {
		t.Fatalf("runCheck auto-discovered: %v (output = %q)", runErr, output)
	}
	if output == "" {
		t.Fatalf("expected success message on stdout, got none")
	}
}
