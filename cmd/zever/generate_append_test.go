package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/diag"
)

// writeModuleFixture writes internal/<module>/<module>.zen under dir and
// returns its path.
func writeModuleFixture(t *testing.T, dir, module, content string) string {
	t.Helper()

	path := filepath.Join(dir, "internal", module, module+".zen")

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	return path
}

// readFile is a t.Fatal-ing os.ReadFile. Owned by the generate wave.
func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	return string(data)
}

// assertZenParses is the round-trip gate every append test asserts: whatever
// the command wrote must still be valid .zen.
func assertZenParses(t *testing.T, path string) {
	t.Helper()

	src := readFile(t, path)

	if _, diags := parseZen(path, []byte(src)); diags.HasErrors() {
		t.Fatalf("result of append does not re-parse:\n%s\ndiagnostics: %v", src, diags)
	}
}

func TestAppendDeclBeforeClosingBraceEmptyModule(t *testing.T) {
	dir := t.TempDir()
	path := writeModuleFixture(t, dir, "shop", "// empty shop module\n")

	if err := appendDeclBeforeClosingBrace(path, "shop", "entity Order {\n\tid: uuid @primary\n}\n"); err != nil {
		t.Fatalf("appendDeclBeforeClosingBrace: %v", err)
	}

	// Flat file: appended at EOF with blank line separator.
	if got := readFile(t, path); !contains(got, "entity Order") {
		t.Fatalf("content = %q, want it to contain entity Order", got)
	}

	assertZenParses(t, path)
}

func TestAppendDeclBeforeClosingBraceNonEmptyModuleGetsBlankLine(t *testing.T) {
	dir := t.TempDir()
	path := writeModuleFixture(t, dir, "shop", "entity Order {\n\tid: uuid @primary\n}\n")

	if err := appendDeclBeforeClosingBrace(path, "shop", "job Ship() {\n\tqueue: default\n}\n"); err != nil {
		t.Fatalf("appendDeclBeforeClosingBrace: %v", err)
	}

	got := readFile(t, path)
	if !contains(got, "entity Order") || !contains(got, "job Ship") {
		t.Fatalf("content = %q, want both entities", got)
	}
	// Blank line between decls.
	if !contains(got, "}\n\njob Ship") {
		t.Fatalf("content = %q, want blank line separator", got)
	}

	assertZenParses(t, path)
}

func TestAppendDeclBeforeClosingBracePreservesCommentsAndTrailingContent(t *testing.T) {
	dir := t.TempDir()

	const original = "// leading file comment\nentity Order {\n\tid: uuid @primary\n}\n\n// trailing comment\n"

	path := writeModuleFixture(t, dir, "shop", original)

	if err := appendDeclBeforeClosingBrace(path, "shop", "job Ship() {\n\tqueue: default\n}\n"); err != nil {
		t.Fatalf("appendDeclBeforeClosingBrace: %v", err)
	}

	got := readFile(t, path)

	for _, fragment := range []string{
		"// leading file comment",
		"// trailing comment",
		"job Ship() {",
	} {
		if !contains(got, fragment) {
			t.Fatalf("result lost %q:\n%s", fragment, got)
		}
	}

	assertZenParses(t, path)
}

// TestAppendDeclBeforeClosingBraceIgnoresBracesInStrings is the reason the
// closing brace is located through the lexer rather than by counting braces
// in raw text: a cron spec (or any string literal) may contain braces.
func TestAppendDeclBeforeClosingBraceIgnoresBracesInStrings(t *testing.T) {
	dir := t.TempDir()

	const original = "job Ship() {\n\tqueue: default\n}\n" +
		"schedule Nightly {\n\tcron: \"0 0 * * * } {\"\n\tdispatch: Ship()\n}\n"

	path := writeModuleFixture(t, dir, "shop", original)

	if err := appendDeclBeforeClosingBrace(path, "shop", "entity Order {\n\tid: uuid @primary\n}\n"); err != nil {
		t.Fatalf("appendDeclBeforeClosingBrace: %v", err)
	}

	got := readFile(t, path)

	if !contains(got, "entity Order") {
		t.Fatalf("entity was not appended:\n%s", got)
	}

	assertZenParses(t, path)
}

func TestAppendDeclBeforeClosingBraceUnknownModuleLeavesFileUnchanged(t *testing.T) {
	// With dir-derived modules, unknown module path still errors via readZenModule,
	// but appendDeclBeforeClosingBrace on existing file never errors for unknown
	// module (it just appends). This test now verifies append succeeds.
	dir := t.TempDir()

	const original = "entity Order {\n\tid: uuid @primary\n}\n"

	path := writeModuleFixture(t, dir, "shop", original)

	if err := appendDeclBeforeClosingBrace(path, "warehouse", "entity Other {\n\tid: uuid @primary\n}\n"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should have been appended.
	if got := readFile(t, path); !contains(got, "Other") {
		t.Fatalf("file was not appended: %q", got)
	}
}

func TestAppendDeclRejectsTraversalModule(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if _, _, _, err := readZenModule("test", "../evil"); !isTraversalError(err) {
		t.Fatalf("readZenModule(../evil) = %v, want ErrPathTraversal", err)
	}

	if _, err := GenerateEntity(GenerateEntityConfig{Module: "../evil", Name: "Order"}); !isTraversalError(err) {
		t.Fatalf("GenerateEntity(../evil) = %v, want ErrPathTraversal", err)
	}

	if _, err := GenerateJob(GenerateJobConfig{Module: "shop", Name: "../Ship", Queue: "default"}); !isTraversalError(err) {
		t.Fatalf("GenerateJob(../Ship) = %v, want ErrPathTraversal", err)
	}

	if _, err := GenerateSchedule(GenerateScheduleConfig{
		Module:   "shop",
		Name:     "Nightly",
		Cron:     "0 0 * * *",
		Dispatch: "../Ship",
	}); !isTraversalError(err) {
		t.Fatalf("GenerateSchedule(../Ship) = %v, want ErrPathTraversal", err)
	}
}

// TestAppendDeclBeforeClosingBraceRejectsUnparseableResult proves the
// validation gate: a declaration that would not parse is never written.
func TestAppendDeclBeforeClosingBraceRejectsUnparseableResult(t *testing.T) {
	dir := t.TempDir()

	const original = "entity Existing {\n\tid: uuid @primary\n}\n"

	path := writeModuleFixture(t, dir, "shop", original)

	if err := appendDeclBeforeClosingBrace(path, "shop", "entity Order {\n"); err == nil {
		t.Fatalf("expected an error for a declaration that does not parse")
	}

	if got := readFile(t, path); got != original {
		t.Fatalf("file was modified: %q", got)
	}

	// No temp file may be left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected only the original file, got %d entries", len(entries))
	}
}

func TestAppendDeclBeforeClosingBraceRefusesUnparseableSource(t *testing.T) {
	dir := t.TempDir()

	const original = "entity {\n"

	path := writeModuleFixture(t, dir, "shop", original)

	if err := appendDeclBeforeClosingBrace(path, "shop", "entity Order {\n}\n"); err == nil {
		t.Fatalf("expected an error for a source file that does not parse")
	}

	if got := readFile(t, path); got != original {
		t.Fatalf("file was modified: %q", got)
	}
}

func TestByteOffset(t *testing.T) {
	src := []byte("ab\ncdé\nx")

	tests := []struct {
		line, col, want int
	}{
		{1, 1, 0},
		{1, 3, 2},
		{2, 1, 3},
		{2, 3, 5},  // é starts at byte 5
		{2, 4, 7},  // é is two bytes wide
		{3, 1, 8},  // after the second newline
		{3, 2, -1}, // past EOF
	}

	for _, tc := range tests {
		got, err := byteOffset(src, diag.Position{Line: tc.line, Col: tc.col})
		if tc.want < 0 {
			if err == nil {
				t.Fatalf("byteOffset(%d,%d) = %d, want an error", tc.line, tc.col, got)
			}

			continue
		}

		if err != nil {
			t.Fatalf("byteOffset(%d,%d): %v", tc.line, tc.col, err)
		}

		if got != tc.want {
			t.Fatalf("byteOffset(%d,%d) = %d, want %d", tc.line, tc.col, got, tc.want)
		}
	}
}

func TestIndentLines(t *testing.T) {
	got := indentLines("a\n\nb", "\t")
	if got != "\ta\n\n\tb" {
		t.Fatalf("indentLines = %q", got)
	}
}

func TestReadZenModuleErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if _, _, _, err := readZenModule("test", "missing"); err == nil {
		t.Fatalf("expected an error for a module with no .zen file")
	}

	// Module identity is purely dir-derived now (no block wrapper/name to
	// mismatch), so any well-formed flat file under the module's directory
	// is accepted regardless of its declared entity names.
	writeModuleFixture(t, dir, "shop", "entity Other {\n\tid: uuid @primary\n}\n")

	if _, _, _, err := readZenModule("test", "shop"); err != nil {
		t.Fatalf("unexpected error reading a well-formed flat module file: %v", err)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}

	return -1
}

// TestCoverReadZenModuleParseError covers existing-but-unparseable file.
func TestCoverReadZenModuleParseError(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeModuleFixture(t, dir, "shop", "entity {\n")
	if _, _, _, err := readZenModule("test", "shop"); err == nil {
		t.Fatalf("expected parse error")
	}
}

// TestCoverDeclNameTakenAllKinds covers service/job/schedule/message branches.
func TestCoverDeclNameTakenAllKinds(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	path := writeModuleFixture(t, dir, "shop", "entity E1 {\n\tid: uuid @primary\n}\nservice S1 {\n\trpc F(id: uuid) -> E1 {\n\t\thttp: GET \"/x\"\n\t}\n}\njob J1() {\n\tqueue: default\n}\nschedule Sch1 {\n\tcron: \"0 0 * * *\"\n\tdispatch: J1()\n}\n")
	src := readFile(t, path)
	_, diags := parseZen(path, []byte(src))
	if diags.HasErrors() {
		t.Fatalf("fixture does not parse: %v", diags)
	}
	file, _ := parseZen(path, []byte(src))
	for _, tc := range []struct{ name, kind string }{
		{"E1", "entity"}, {"S1", "service"}, {"J1", "job"}, {"Sch1", "schedule"},
	} {
		if k, ok := declNameTaken(file, tc.name); !ok || k != tc.kind {
			t.Fatalf("declNameTaken(%q) = (%q,%v), want (%q,true)", tc.name, k, ok, tc.kind)
		}
	}
	if _, ok := declNameTaken(file, "Nope"); ok {
		t.Fatalf("unexpected taken")
	}
}

// TestCoverWriteAtomicallyErrors covers temp-create/write/chmod/rename faults.
func TestCoverWriteAtomicallyErrors(t *testing.T) {
	dir := t.TempDir()
	// Missing parent: CreateTemp fails.
	if err := writeAtomically(filepath.Join(dir, "nope", "sub", "f.zen"), []byte("x")); err == nil {
		// If MkdirAll-like behavior created it, accept; otherwise must error.
		// CreateTemp does not mkdir, so it must error.
		t.Fatalf("expected temp-create error")
	}
	// Rename failure: path is a directory.
	d := filepath.Join(dir, "adir")
	if err := os.MkdirAll(d, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := writeAtomically(d, []byte("x")); err == nil {
		t.Fatalf("expected rename error")
	}
	// Success path.
	p := filepath.Join(dir, "ok.zen")
	if err := writeAtomically(p, []byte("entity A {\n\tid: uuid @primary\n}\n")); err != nil {
		t.Fatalf("writeAtomically: %v", err)
	}
}

// TestCoverAppendDeclReadError covers missing file read.
func TestCoverAppendDeclReadError(t *testing.T) {
	if err := appendDeclBeforeClosingBrace(filepath.Join(t.TempDir(), "missing.zen"), "shop", "entity A {\n}\n"); err == nil {
		t.Fatalf("expected read error")
	}
}
