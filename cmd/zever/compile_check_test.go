package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// compileCheckFixture compiles the shared inspect schema once (write mode)
// and returns the schema path and the committed output dir.
func compileCheckFixture(t *testing.T, backends string) (string, string) {
	t.Helper()

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "generated")

	var out bytes.Buffer
	if err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: backends,
		OutDir:   outDir,
		Out:      &out,
	}); err != nil {
		t.Fatalf("runCompileWith (write): %v", err)
	}

	return schemaPath, outDir
}

// TestCompileCheckCleanTree proves --check exits 0 when the committed output
// matches a fresh compile of the same schema.
func TestCompileCheckCleanTree(t *testing.T) {
	t.Parallel()

	schemaPath, outDir := compileCheckFixture(t, "atlas,zenorm")

	var out bytes.Buffer
	if err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "atlas,zenorm",
		OutDir:   outDir,
		Out:      &out,
		Check:    true,
	}); err != nil {
		t.Fatalf("runCompileWith --check on clean tree: %v", err)
	}

	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("check output = %q, want up-to-date confirmation", out.String())
	}
}

// TestCompileCheckDetectsDrift proves --check exits 1 naming the drifted
// file when committed output no longer matches the schema, and writes
// nothing.
func TestCompileCheckDetectsDrift(t *testing.T) {
	t.Parallel()

	schemaPath, outDir := compileCheckFixture(t, "atlas")

	// Simulate a stale commit: hand-edit one generated file.
	stale := filepath.Join(outDir, "atlas", "schema.hcl")
	data, readErr := os.ReadFile(stale)
	if readErr != nil {
		t.Fatalf("read %q: %v", stale, readErr)
	}

	if writeErr := os.WriteFile(stale, append(data, []byte("\n# stale hand edit\n")...), 0o600); writeErr != nil { //nolint:gosec // test temp dir
		t.Fatalf("write stale %q: %v", stale, writeErr)
	}

	var out bytes.Buffer
	checkErr := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "atlas",
		OutDir:   outDir,
		Out:      &out,
		Check:    true,
	})
	if checkErr == nil {
		t.Fatal("expected drift error for stale output, got nil")
	}

	if !strings.Contains(checkErr.Error(), "zever compile") {
		t.Fatalf("drift error = %q, want zever compile prefix", checkErr.Error())
	}

	if !strings.Contains(checkErr.Error(), "schema.hcl") {
		t.Fatalf("drift error = %q, want drifted file named", checkErr.Error())
	}

	// --check must never repair the tree: the stale edit stays put.
	after, err := os.ReadFile(stale)
	if err != nil {
		t.Fatalf("read %q: %v", stale, err)
	}

	if !strings.Contains(string(after), "stale hand edit") {
		t.Fatal("--check rewrote the output tree; check mode must be read-only")
	}
}

// TestCompileCheckMissingOutput proves --check fails when the output dir was
// never generated (e.g. fresh clone with generated/ gitignored).
func TestCompileCheckMissingOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var out bytes.Buffer
	err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "atlas",
		OutDir:   filepath.Join(dir, "generated"),
		Out:      &out,
		Check:    true,
	})
	if err == nil {
		t.Fatal("expected drift error for missing output, got nil")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "generated")); !os.IsNotExist(statErr) {
		t.Fatal("--check created the output dir; check mode must be read-only")
	}
}

// TestCompileCheckDetectsExtraFile proves --check fails when the committed
// tree holds a generated-tree file the schema no longer produces (stale
// file from a deleted model).
func TestCompileCheckDetectsExtraFile(t *testing.T) {
	t.Parallel()

	schemaPath, outDir := compileCheckFixture(t, "atlas")

	extra := filepath.Join(outDir, "atlas", "stale.hcl")
	if err := os.WriteFile(extra, []byte("// stale\n"), 0o600); err != nil {
		t.Fatalf("write extra %q: %v", extra, err)
	}

	var out bytes.Buffer
	err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "atlas",
		OutDir:   outDir,
		Out:      &out,
		Check:    true,
	})
	if err == nil {
		t.Fatal("expected drift error for extra file, got nil")
	}

	if !strings.Contains(err.Error(), "stale.hcl") {
		t.Fatalf("drift error = %q, want extra file named", err.Error())
	}
}

// TestCompileCheckJSONEmitsEnvelope proves --check in JSON mode emits a
// single envelope: success carries check:true, drift carries the file list
// and a nonzero exit code. Sequential (no t.Parallel): mustJSONMode flips a
// package-global sink.
func TestCompileCheckJSONEmitsEnvelope(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "generated")

	var out bytes.Buffer
	if err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "atlas",
		OutDir:   outDir,
		Out:      &out,
	}); err != nil {
		t.Fatalf("runCompileWith (write): %v", err)
	}

	jbuf.Reset()

	if err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "atlas",
		OutDir:   outDir,
		Out:      &out,
		JSON:     true,
		Check:    true,
	}); err != nil {
		t.Fatalf("runCompileWith --check --json: %v", err)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "compile" {
		t.Fatalf("envelope = %+v, want ok compile envelope", env)
	}

	if data := envelopeData(t, env); data["check"] != true {
		t.Fatalf("envelope data = %v, want check:true", data)
	}

	stale := filepath.Join(outDir, "atlas", "schema.hcl")
	data, err := os.ReadFile(stale)
	if err != nil {
		t.Fatalf("read %q: %v", stale, err)
	}

	if err := os.WriteFile(stale, append(data, 0x0a), 0o600); err != nil { //nolint:gosec // test temp dir
		t.Fatalf("write stale %q: %v", stale, err)
	}

	jbuf.Reset()

	if err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "atlas",
		OutDir:   outDir,
		Out:      &out,
		JSON:     true,
		Check:    true,
	}); err == nil {
		t.Fatal("expected drift error for stale output in JSON mode, got nil")
	}

	env = mustDecodeEnvelope(t, jbuf)
	if env.OK || env.ExitCode == ExitOK {
		t.Fatalf("envelope = %+v, want failing envelope", env)
	}

	dataMap := envelopeData(t, env)
	drift, ok := dataMap["drift"].([]any)
	if !ok || len(drift) != 1 {
		t.Fatalf("envelope data = %v, want one drift entry", dataMap)
	}

	name, ok := drift[0].(string)
	if !ok || !strings.Contains(name, "schema.hcl") {
		t.Fatalf("drift = %v, want schema.hcl named", drift)
	}
}
