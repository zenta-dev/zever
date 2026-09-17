package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRunFmtCheckOnlyListsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectBadlyIndentedSchema)

	var runErr error

	output := inspectCaptureOutput(t, func() {
		runErr = runFmt([]string{path})
	})

	if runErr == nil {
		t.Fatalf("expected non-zero-exit error when a file needs formatting")
	}

	if output == "" {
		t.Fatalf("expected the file path to be listed on stdout")
	}

	content, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	if string(content) != inspectBadlyIndentedSchema {
		t.Fatalf("check-only mode must not write: got %q, want unchanged %q", content, inspectBadlyIndentedSchema)
	}
}

func TestRunFmtCheckOnlyCleanFileNoOutput(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var runErr error

	output := inspectCaptureOutput(t, func() {
		runErr = runFmt([]string{path})
	})

	if runErr != nil {
		t.Fatalf("runFmt on an already-canonical file: %v", runErr)
	}

	if output != "" {
		t.Fatalf("expected no output for an already-canonical file, got %q", output)
	}
}

func TestRunFmtWriteRewritesInPlace(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectBadlyIndentedSchema)

	if err := runFmt([]string{"--write", path}); err != nil {
		t.Fatalf("runFmt --write: %v", err)
	}

	content, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	if string(content) == inspectBadlyIndentedSchema {
		t.Fatalf("expected file to be rewritten, content unchanged")
	}

	// Idempotency: running --write again produces no further changes.
	if rewriteErr := runFmt([]string{"--write", path}); rewriteErr != nil {
		t.Fatalf("second runFmt --write: %v", rewriteErr)
	}

	second, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	if string(second) != string(content) {
		t.Fatalf("format is not idempotent:\nfirst:\n%q\nsecond:\n%q", content, second)
	}

	// And check-only mode now reports it clean.
	if err := runFmt([]string{path}); err != nil {
		t.Fatalf("runFmt check-only after --write should report clean, got: %v", err)
	}
}

func TestRunFmtShortWriteFlag(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectBadlyIndentedSchema)

	if err := runFmt([]string{"-w", path}); err != nil {
		t.Fatalf("runFmt -w: %v", err)
	}

	content, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	if string(content) == inspectBadlyIndentedSchema {
		t.Fatalf("expected file to be rewritten, content unchanged")
	}
}

func TestRunFmtNoFiles(t *testing.T) {
	err := runFmt(nil)
	if !errors.Is(err, errNoInputFiles) {
		t.Fatalf("error = %v, want errNoInputFiles", err)
	}
}

func TestRunFmtSyntaxErrorFile(t *testing.T) {
	dir := t.TempDir()
	brokenSchema := "entity Broken {\n" +
		"name: string @default(\"unterminated)\n" +
		"}\n"
	path := writeInspectFixture(t, dir, "broken.zen", brokenSchema)

	if err := runFmt([]string{path}); err == nil {
		t.Fatalf("expected error for a file with syntax errors, got nil")
	}
}

func TestRunFmtInteractiveFlag(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	prev := interactiveMode
	t.Cleanup(func() { interactiveMode = prev })

	if err := runFmt([]string{"-i", path}); err != nil {
		t.Fatalf("runFmt -i on clean file: %v", err)
	}

	if !interactiveMode {
		t.Fatalf("expected -i to set interactiveMode")
	}
}

func TestRunFmtWithCoreWriteError(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectBadlyIndentedSchema)

	// Read-only file: rewriting it must fail (overwriting an existing
	// file needs write permission on the file itself).
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	var out bytes.Buffer
	err := runFmtWith(FmtConfig{Files: []string{path}, Write: true, Out: &out})
	if err == nil {
		t.Fatalf("expected write error in a read-only dir, got nil")
	}
}

func TestRunFmtHelpPrintsUsage(t *testing.T) {
	if err := runFmt([]string{"-h"}); err != nil {
		t.Fatalf("runFmt -h: %v", err)
	}
}

func TestRunFmtFlagParseError(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	if err := runFmt([]string{"--bogus-flag", path}); err == nil {
		t.Fatalf("expected flag parse error, got nil")
	}
}

func TestRunFmtWithCoreCheckOnlyReportsChanged(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectBadlyIndentedSchema)

	var out bytes.Buffer
	err := runFmtWith(FmtConfig{Files: []string{path}, Out: &out})
	if err == nil {
		t.Fatalf("expected error when a file needs formatting, got nil")
	}

	if !strings.Contains(err.Error(), "would be reformatted") {
		t.Fatalf("error = %q, want reformatted count", err.Error())
	}

	if !strings.Contains(out.String(), path) {
		t.Fatalf("output = %q, want the file path listed", out.String())
	}
}

func TestRunFmtWithCoreWriteListsChanged(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectBadlyIndentedSchema)

	var out bytes.Buffer
	if err := runFmtWith(FmtConfig{Files: []string{path}, Write: true, Out: &out}); err != nil {
		t.Fatalf("runFmtWith write: %v", err)
	}

	if !strings.Contains(out.String(), path) {
		t.Fatalf("output = %q, want the rewritten path listed", out.String())
	}
}

func TestRunFmtWithCoreEmptyFiles(t *testing.T) {
	var out bytes.Buffer
	if err := runFmtWith(FmtConfig{Out: &out}); err == nil {
		t.Fatalf("expected error for empty file list, got nil")
	}
}

func TestRunFmtWithCoreMissingFile(t *testing.T) {
	var out bytes.Buffer
	err := runFmtWith(FmtConfig{Files: []string{"nope.zen"}, Out: &out})
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}

func TestRunFmtAutoDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, "schema/app.zen", inspectUserSchema)
	t.Chdir(dir)
	t.Setenv("ZEVER_NO_HINT", "1")

	var runErr error
	output := inspectCaptureOutput(t, func() {
		runErr = runFmt(nil)
	})
	if runErr != nil {
		t.Fatalf("runFmt auto-discovered clean: %v (output = %q)", runErr, output)
	}
	if output != "" {
		t.Fatalf("expected no output for clean auto-discovered file, got %q", output)
	}
}

func TestRunFmtInteractiveLongForm(t *testing.T) {
	dir := t.TempDir()
	path := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	prev := interactiveMode
	t.Cleanup(func() { interactiveMode = prev })

	if err := runFmt([]string{"--interactive=true", path}); err != nil {
		t.Fatalf("runFmt --interactive=true on clean file: %v", err)
	}
	if !interactiveMode {
		t.Fatalf("expected --interactive=true to set interactiveMode")
	}
}
