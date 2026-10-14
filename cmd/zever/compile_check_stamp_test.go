package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mapFiles applies fn to every file under dir, rewriting those whose content
// changes, and returns how many were rewritten.
func mapFiles(t *testing.T, dir string, fn func(string) string) int {
	t.Helper()

	n := 0

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		data, readErr := os.ReadFile(path) //nolint:gosec // test temp dir
		if readErr != nil {
			return readErr
		}

		next := fn(string(data))
		if next == string(data) {
			return nil
		}

		n++

		return os.WriteFile(path, []byte(next), 0o600) //nolint:gosec // test temp dir
	})
	if err != nil {
		t.Fatalf("mapFiles: %v", err)
	}

	return n
}

// restampVersion rewrites the CLI-version field of every stamped file under
// outDir, simulating output committed by a different zever release.
func restampVersion(t *testing.T, outDir, oldV string) {
	t.Helper()

	n := mapFiles(t, outDir, func(s string) string {
		s = strings.ReplaceAll(s, " v"+buildVersion+" - schema", " v"+oldV+" - schema")

		return strings.ReplaceAll(s, `"version": "v`+buildVersion+`"`, `"version": "v`+oldV+`"`)
	})
	if n == 0 {
		t.Fatal("restamp changed no files; stamp shape changed?")
	}
}

// TestCompileCheckStampVersionOnlyPasses proves a CLI-version bump alone does
// not fail --check: it passes with a distinct note, and --strict fails.
func TestCompileCheckStampVersionOnlyPasses(t *testing.T) {
	t.Parallel()

	schemaPath, outDir := compileCheckFixture(t, "atlas,openapi,zenorm")
	restampVersion(t, outDir, "0.0.1")

	cfg := CompileConfig{
		Files:    []string{schemaPath},
		Backends: "atlas,openapi,zenorm",
		OutDir:   outDir,
		Check:    true,
	}

	var out bytes.Buffer

	cfg.Out = &out
	if err := runCompileWith(cfg); err != nil {
		t.Fatalf("--check with only a stamp version change: %v", err)
	}

	if !strings.Contains(out.String(), "stamped by a different zever version") {
		t.Fatalf("output = %q, want stamp-version note", out.String())
	}

	cfg.Strict = true
	cfg.Out = &bytes.Buffer{}

	if err := runCompileWith(cfg); err == nil || !strings.Contains(err.Error(), "stamp version") {
		t.Fatalf("--check --strict err = %v, want stamp version failure", err)
	}
}

// TestCompileCheckStampVersionPlusContentFails proves content drift is still
// reported when the stamp version also differs.
func TestCompileCheckStampVersionPlusContentFails(t *testing.T) {
	t.Parallel()

	schemaPath, outDir := compileCheckFixture(t, "atlas")
	restampVersion(t, outDir, "0.0.1")

	if n := mapFiles(t, outDir, func(s string) string { return s + "# edit\n" }); n == 0 {
		t.Fatal("no files edited")
	}

	err := runCompileWith(CompileConfig{Files: []string{schemaPath}, Backends: "atlas", OutDir: outDir, Out: &bytes.Buffer{}, Check: true})
	if err == nil || !strings.Contains(err.Error(), "schema.hcl") {
		t.Fatalf("err = %v, want content drift", err)
	}
}

// TestCompileCheckSchemaHashChangeFails proves a different schema hash in
// the stamp is drift even when the version also differs.
func TestCompileCheckSchemaHashChangeFails(t *testing.T) {
	t.Parallel()

	schemaPath, outDir := compileCheckFixture(t, "atlas,openapi")
	restampVersion(t, outDir, "0.0.1")

	if n := mapFiles(t, outDir, func(s string) string { return strings.ReplaceAll(s, "sha256:", "sha256:0") }); n == 0 {
		t.Fatal("no files edited")
	}

	err := runCompileWith(CompileConfig{Files: []string{schemaPath}, Backends: "atlas,openapi", OutDir: outDir, Out: &bytes.Buffer{}, Check: true})
	if err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("err = %v, want schema hash drift", err)
	}
}
