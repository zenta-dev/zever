package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// entityOnlySchema is a service-less schema: the default pipeline compiles
// it without the auto-added proto/protogogen backends.
const entityOnlySchema = `entity Widget {
	id: uuid @primary
	name: string
}
`

func writeEntityOnlySchema(t *testing.T, dir string) {
	t.Helper()

	schemaDir := filepath.Join(dir, "schema")
	if err := os.MkdirAll(schemaDir, 0o750); err != nil {
		t.Fatalf("mkdir schema: %v", err)
	}

	if err := os.WriteFile(filepath.Join(schemaDir, "shop.zen"), []byte(entityOnlySchema), 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

func generateFullFixture(t *testing.T, schema func(t *testing.T, dir string)) {
	t.Helper()

	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	if schema != nil {
		schema(t, dir)
	}
}

// TestResolveBackendSpec pins the override chain: explicit flag wins, then
// ZEVER_BACKENDS, then GenerateDefaultBackends.
func TestResolveBackendSpec(t *testing.T) {
	if spec, defaulted := resolveBackendSpec("zenorm"); spec != "zenorm" || defaulted {
		t.Fatalf("explicit = %q,%v, want zenorm,false", spec, defaulted)
	}

	t.Setenv("ZEVER_BACKENDS", "atlas,zenorm")

	if spec, defaulted := resolveBackendSpec(""); spec != "atlas,zenorm" || !defaulted {
		t.Fatalf("env = %q,%v, want atlas,zenorm,true", spec, defaulted)
	}

	if spec, _ := resolveBackendSpec("gogen"); spec != "gogen" {
		t.Fatalf("flag must beat env, got %q", spec)
	}

	t.Setenv("ZEVER_BACKENDS", "")

	if spec, defaulted := resolveBackendSpec(""); spec != GenerateDefaultBackends || !defaulted {
		t.Fatalf("default = %q,%v, want %q,true", spec, defaulted, GenerateDefaultBackends)
	}
}

// TestGenerateDefaultBackendsIsOneConstant proves the default pipeline set
// is exactly gogen,zenorm,atlas,openapi: proto/protogogen join only via
// schema-driven auto-add, mcp stays opt-in.
func TestGenerateDefaultBackendsIsOneConstant(t *testing.T) {
	t.Parallel()

	if GenerateDefaultBackends != "gogen,zenorm,atlas,openapi" {
		t.Fatalf("GenerateDefaultBackends = %q", GenerateDefaultBackends)
	}

	for _, name := range []string{"proto", "protogogen", "mcp"} {
		if strings.Contains(GenerateDefaultBackends, name) {
			t.Fatalf("default set must not contain %q", name)
		}
	}
}

// TestSchemaDeclaresServicesAndGRPCBackends pins the auto-add helpers.
func TestSchemaDeclaresServicesAndGRPCBackends(t *testing.T) {
	t.Parallel()

	if schemaDeclaresServices(nil) {
		t.Fatal("nil schema declares services")
	}

	if got := withGRPCBackends("gogen,zenorm"); got != "gogen,zenorm,proto,protogogen" {
		t.Fatalf("withGRPCBackends = %q", got)
	}

	if got := withGRPCBackends("gogen,proto,protogogen"); got != "gogen,proto,protogogen" {
		t.Fatalf("withGRPCBackends must not duplicate: %q", got)
	}

	if missing := missingGRPCBackends("gogen,proto"); len(missing) != 1 || missing[0] != "protogogen" {
		t.Fatalf("missing = %v", missing)
	}
}

// firstGeneratedFile returns the first regular file under dir (backends may
// emit subdirectories), failing the test when there is none.
func firstGeneratedFile(t *testing.T, dir string) string {
	t.Helper()

	var first string

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || first != "" {
			return nil
		}

		first = path

		return nil
	})
	if err != nil {
		t.Fatalf("walk %q: %v", dir, err)
	}

	if first == "" {
		t.Fatalf("no generated files under %q", dir)
	}

	return first
}

// TestGenerateFullWritesTree proves the bare pipeline writes compiled
// outputs plus the serve-based scaffold, printing an R2 summary.
func TestGenerateFullWritesTree(t *testing.T) {
	generateFullFixture(t, writeEntityOnlySchema)

	var out bytes.Buffer

	err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &out})
	if err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	// gogen emits per-service code only: an entity-only schema yields no
	// gogen output, while zenorm/atlas/openapi always render.
	for _, backend := range []string{"zenorm", "atlas", "openapi"} {
		entries, readErr := os.ReadDir(filepath.Join("generated", backend))
		if readErr != nil || len(entries) == 0 {
			t.Fatalf("backend %s produced no output: %v", backend, readErr)
		}
	}

	if _, err := os.Stat("generated/proto"); !os.IsNotExist(err) {
		t.Fatal("service-less schema must not auto-add proto output")
	}

	for _, path := range []string{
		filepath.Join("cmd", "server", "main.go"),
		filepath.Join("internal", "app", "modules.go"),
		filepath.Join("internal", "app", "app.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %q: %v", path, err)
		}
	}

	summary := out.String()
	if !strings.Contains(summary, "created") || !strings.Contains(summary, GenerateDefaultBackends) {
		t.Fatalf("summary lacks created counts/backends:\n%s", summary)
	}
}

// TestGenerateFullRerunUnchanged proves the second run is a no-op write-wise
// and says so.
func TestGenerateFullRerunUnchanged(t *testing.T) {
	generateFullFixture(t, writeEntityOnlySchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("first GenerateFull: %v", err)
	}

	var out bytes.Buffer

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &out}); err != nil {
		t.Fatalf("second GenerateFull: %v", err)
	}

	if !strings.Contains(out.String(), "0 created, 0 updated") {
		t.Fatalf("rerun summary = %q, want all-unchanged", out.String())
	}
}

// TestGenerateFullDiffPrintsWithoutWriting proves --diff prints a unified
// diff and leaves the tree untouched.
func TestGenerateFullDiffPrintsWithoutWriting(t *testing.T) {
	generateFullFixture(t, writeEntityOnlySchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join("generated", "zenorm"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("zenorm output missing: %v", entries)
	}

	victim := firstGeneratedFile(t, filepath.Join("generated", "zenorm"))

	before, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatalf("read %q: %v", victim, readErr)
	}

	if staleErr := os.WriteFile(victim, append(before, []byte("\n-- stale\n")...), 0o600); staleErr != nil { // test-controlled temp path
		t.Fatalf("write stale: %v", staleErr)
	}

	var out bytes.Buffer

	if diffErr := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &out, Diff: true}); diffErr != nil {
		t.Fatalf("GenerateFull --diff: %v", diffErr)
	}

	diff := out.String()
	if !strings.Contains(diff, "--- a/") || !strings.Contains(diff, "+++ b/") || !strings.Contains(diff, "-") {
		t.Fatalf("diff output is not a unified diff:\n%s", diff)
	}

	after, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read %q: %v", victim, err)
	}

	if !strings.Contains(string(after), "-- stale") {
		t.Fatal("--diff rewrote the tree; diff mode must be read-only")
	}
}

// TestGenerateFullCheckCleanTree proves --check exits 0 on a fresh tree.
func TestGenerateFullCheckCleanTree(t *testing.T) {
	generateFullFixture(t, writeEntityOnlySchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	var out bytes.Buffer

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &out, Check: true}); err != nil {
		t.Fatalf("--check on clean tree: %v", err)
	}

	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("check output = %q, want up-to-date confirmation", out.String())
	}
}

// TestGenerateFullCheckDetectsDrift proves --check fails on stale content
// and on missing files, writing nothing either way.
func TestGenerateFullCheckDetectsDrift(t *testing.T) {
	generateFullFixture(t, writeEntityOnlySchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join("generated", "atlas"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("atlas output missing: %v", entries)
	}

	victim := filepath.Join("generated", "atlas", entries[0].Name())

	data, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatalf("read %q: %v", victim, readErr)
	}

	if staleErr := os.WriteFile(victim, append(data, 0x0a), 0o600); staleErr != nil { // test-controlled temp path
		t.Fatalf("write stale: %v", staleErr)
	}

	checkErr := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}, Check: true})
	if checkErr == nil || !strings.Contains(checkErr.Error(), entries[0].Name()) {
		t.Fatalf("check err = %v, want drift naming the file", checkErr)
	}

	after, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read %q: %v", victim, err)
	}

	if len(after) != len(data)+1 {
		t.Fatal("--check repaired the tree; check mode must be read-only")
	}

	if err := os.Remove(victim); err != nil {
		t.Fatalf("remove %q: %v", victim, err)
	}

	checkErr = GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}, Check: true})
	if checkErr == nil || !strings.Contains(checkErr.Error(), "(missing)") {
		t.Fatalf("check err = %v, want missing-file drift", checkErr)
	}
}

// TestGenerateFullCheckStubRules proves R1 under --check: a missing stub is
// drift (a fresh clone must create it), a modified existing stub is not.
func TestGenerateFullCheckStubRules(t *testing.T) {
	generateFullFixture(t, writeServerSchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	stub := filepath.Join("internal", "service", "app", "task_service.go")

	handwritten, err := os.ReadFile(stub)
	if err != nil {
		t.Fatalf("read stub: %v", err)
	}

	// A hand-modified stub is never drift.
	if err := os.WriteFile(stub, append(handwritten, []byte("\n// mine\n")...), 0o600); err != nil { // test-controlled temp path
		t.Fatalf("write stub: %v", err)
	}

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}, Check: true}); err != nil {
		t.Fatalf("--check must tolerate hand-edited stubs: %v", err)
	}

	// A missing stub is drift.
	if err := os.Remove(stub); err != nil {
		t.Fatalf("remove stub: %v", err)
	}

	checkErr := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}, Check: true})
	if checkErr == nil || !strings.Contains(checkErr.Error(), "task_service.go") {
		t.Fatalf("check err = %v, want missing-stub drift", checkErr)
	}
}

// TestGenerateFullCheckStrictOnVersionOnly proves a version-only stamp
// difference is a note by default and a failure under Strict.
func TestGenerateFullCheckStrictOnVersionOnly(t *testing.T) {
	generateFullFixture(t, writeEntityOnlySchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join("generated", "zenorm"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("zenorm output missing: %v", entries)
	}

	victim := firstGeneratedFile(t, filepath.Join("generated", "zenorm"))

	data, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatalf("read %q: %v", victim, readErr)
	}

	restamped := bytes.Replace(data, []byte("v"+buildVersion), []byte("v0.0.0-check"), 1)
	if string(restamped) == string(data) {
		t.Fatal("restamp changed no bytes; stamp shape changed?")
	}

	if err := os.WriteFile(victim, restamped, 0o600); err != nil { // test-controlled temp path
		t.Fatalf("write restamped: %v", err)
	}

	var out bytes.Buffer

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &out, Check: true}); err != nil {
		t.Fatalf("--check with only a stamp version change: %v", err)
	}

	if !strings.Contains(out.String(), "different zever version") {
		t.Fatalf("check output = %q, want stamp-version note", out.String())
	}

	strictErr := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}, Check: true, Strict: true})
	if strictErr == nil || !strings.Contains(strictErr.Error(), "stamp version") {
		t.Fatalf("strict err = %v, want stamp version failure", strictErr)
	}
}

// TestGenerateFullExplicitBackendsNoAutoAdd proves an explicit selection is
// honored exactly: no proto output for a service schema.
// TestCompileSchemaWithDefaultsAutoAddsGRPC proves the default selection
// grows proto/protogogen when the schema declares services. It runs without
// chdir: protogogen shells out to a go tool declared by this module, so it
// only succeeds inside the module tree.
func TestCompileSchemaWithDefaultsAutoAddsGRPC(t *testing.T) {
	files := map[string]string{"schema/app.zen": serverSchemaFixture}

	result, spec, diags, err := compileSchemaWithDefaults(files, "schema", "", "./generated")
	if err != nil {
		t.Fatalf("compileSchemaWithDefaults: %v", err)
	}

	if diags.HasErrors() {
		t.Fatalf("diags errors: %v", diags)
	}

	for _, name := range []string{"proto", "protogogen"} {
		if !strings.Contains(","+spec+",", ","+name+",") {
			t.Fatalf("spec = %q, want auto-added %q", spec, name)
		}

		if _, ok := result.Outputs[name]; !ok {
			t.Fatalf("outputs lack auto-added backend %q", name)
		}
	}
}

// TestGenerateFullDegradesWithoutGRPCTools proves the pipeline stays green
// in a project that does not declare the gRPC tools: the auto-added
// backends are skipped with a visible warning, and base outputs plus the
// scaffold still land. Temp dirs chdir away from the module tree, so the
// go tool lookup fails there by construction.
func TestGenerateFullDegradesWithoutGRPCTools(t *testing.T) {
	generateFullFixture(t, writeServerSchema)

	var out bytes.Buffer

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &out}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	if _, err := os.Stat("generated/proto"); !os.IsNotExist(err) {
		t.Fatal("proto output must be skipped without the grpc tools")
	}

	if entries, err := os.ReadDir("generated/gogen"); err != nil || len(entries) == 0 {
		t.Fatalf("expected base gogen output despite skipped grpc backends: %v", err)
	}

	if _, err := os.Stat(filepath.Join("internal", "service", "app", "task_service.go")); err != nil {
		t.Fatalf("expected service stub despite skipped grpc backends: %v", err)
	}
}

func TestGenerateFullExplicitBackendsNoAutoAdd(t *testing.T) {
	generateFullFixture(t, writeServerSchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Backends: "zenorm", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	if _, err := os.Stat("generated/proto"); !os.IsNotExist(err) {
		t.Fatal("explicit backends must not auto-add proto output")
	}
}

// TestGenerateFullEnvOverride proves ZEVER_BACKENDS replaces the constant
// when no flag is passed, and loses to an explicit flag.
func TestGenerateFullEnvOverride(t *testing.T) {
	t.Setenv("ZEVER_BACKENDS", "atlas")

	generateFullFixture(t, writeEntityOnlySchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	if _, err := os.Stat("generated/zenorm"); !os.IsNotExist(err) {
		t.Fatal("ZEVER_BACKENDS=atlas must suppress the default set")
	}
}

// TestGenerateFullKeepsHandwrittenStubs proves the pipeline never rewrites
// an existing stub (R1: byte-for-byte), even with schema services present.
func TestGenerateFullKeepsHandwrittenStubs(t *testing.T) {
	generateFullFixture(t, writeServerSchema)

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("GenerateFull: %v", err)
	}

	stub := filepath.Join("internal", "service", "app", "task_service.go")

	const handwritten = "package appimpl\n\n// hand written business logic\n"

	if err := os.WriteFile(stub, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write handwritten stub: %v", err)
	}

	var stderr bytes.Buffer

	if err := GenerateFull(GenerateFullConfig{OutDir: "./generated", Stdout: &bytes.Buffer{}, Stderr: &stderr}); err != nil {
		t.Fatalf("second GenerateFull: %v", err)
	}

	content, err := os.ReadFile(stub)
	if err != nil {
		t.Fatalf("read stub: %v", err)
	}

	if string(content) != handwritten {
		t.Fatalf("pipeline overwrote a hand-implemented stub: %q", content)
	}
}

// TestUnifiedDiffTable pins the preview format: hunk header, context lines,
// and full-add rendering for missing files.
func TestUnifiedDiffTable(t *testing.T) {
	t.Parallel()

	diff := previewDiff("gen/a.go", []byte("one\ntwo\nthree\n"), []byte("one\nTWO\nthree\n"))
	if !strings.Contains(diff, "--- a/gen/a.go") || !strings.Contains(diff, "+++ b/gen/a.go") {
		t.Fatalf("diff lacks file headers:\n%s", diff)
	}

	if !strings.Contains(diff, "-two\n") || !strings.Contains(diff, "+TWO\n") {
		t.Fatalf("diff lacks changed lines:\n%s", diff)
	}

	if !strings.Contains(diff, " one\n") {
		t.Fatalf("diff lacks context lines:\n%s", diff)
	}

	full := previewDiff("gen/new.go", nil, []byte("x\n"))
	if !strings.Contains(full, "+x") {
		t.Fatalf("missing-file diff must render full adds:\n%s", full)
	}

	if got := previewDiff("gen/same.go", []byte("x\n"), []byte("x\n")); hasDiffBody(got) {
		t.Fatalf("identical files must render no changed lines:\n%s", got)
	}
}

// hasDiffBody reports whether a unified diff renders changed lines (as
// opposed to headers and context only).
func hasDiffBody(diff string) bool {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			return true
		}

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			return true
		}
	}

	return false
}

// TestRunGenerateRoutesPipelineFlags proves flag-first pipeline words reach
// the full pipeline instead of the legacy generate-module path.
func TestRunGenerateRoutesPipelineFlags(t *testing.T) {
	withWorkingDir(t, t.TempDir())

	for _, args := range [][]string{{"--diff"}, {"--check"}, {"--backend=zenorm"}} {
		err := runGenerate(args)
		if err == nil {
			t.Fatalf("runGenerate(%v) in empty dir: want pipeline error, got nil", args)
		}

		if strings.Contains(err.Error(), "usage: zever generate module") {
			t.Fatalf("runGenerate(%v) took the module path: %v", args, err)
		}
	}
}
