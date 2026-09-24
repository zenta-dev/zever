package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/diag"
)

const inspectUserSchema = `entity User {
  id: uuid @primary
  email: string @unique
}

service UserService {
  rpc GetUser(id: uuid) -> User {
    http: GET "/v1/users/{id}"
    auth: required
  }
}
`

const inspectOrderSchema = `entity User {
	id: uuid @primary
	email: string @unique
}

entity Order {
	id: uuid @primary
	user_id: uuid
	amount_cents: int64

	belongs_to user: User @foreign_key(user_id)
}
`

const inspectOpenAPIOrderSchema = `entity Order {
	id: uuid @primary
	amount_cents: int64
}

service OrderService {
	rpc GetOrder(id: uuid) -> Order {
		http: GET "/v1/orders/{id}"
		auth: required
	}
}
`

const inspectBadlyIndentedSchema = "entity User {\n" +
	"id: uuid @primary\n" +
	"        email: string\n" +
	"}\n"

const inspectBrokenSchema = `entity Broken {
	id: uuid @primary
	bad_field: nonexistent_type
}
`

func writeInspectFixture(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %q: %v", path, err)
	}

	return path
}

// inspectCaptureOutput redirects os.Stdout for fn and returns what was written.
func inspectCaptureOutput(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stdout = w

	fn()

	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe writer: %v", closeErr)
	}

	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	return string(out)
}

func TestRunCompileProtoBackend(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "out")

	if err := runCompile([]string{"--backend=proto", "--out=" + outDir, schemaPath}); err != nil {
		t.Fatalf("runCompile: %v", err)
	}

	protoFile := filepath.Join(outDir, "proto", "schema.proto")
	if _, err := os.Stat(protoFile); err != nil {
		t.Fatalf("expected %q to exist: %v", protoFile, err)
	}

	content, err := os.ReadFile(protoFile) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %q: %v", protoFile, err)
	}

	if len(content) == 0 {
		t.Fatalf("expected non-empty schema.proto content")
	}

	// The shared companion file keeps its wire-stable zever/ path (the
	// ported proto backend retains "zever/annotations.proto" verbatim).
	annotationsFile := filepath.Join(outDir, "proto", "zever", "annotations.proto")
	if _, err := os.Stat(annotationsFile); err != nil {
		t.Fatalf("expected %q to exist: %v", annotationsFile, err)
	}
}

func TestRunCompileUnknownBackend(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	err := runCompile([]string{"--backend=nonexistent", schemaPath})
	if err == nil {
		t.Fatalf("expected error for unknown backend, got nil")
	}
}

func TestRunCompileNoFiles(t *testing.T) {
	err := runCompile(nil)
	if err == nil {
		t.Fatalf("expected error when no input files given, got nil")
	}

	if !errors.Is(err, errNoInputFiles) {
		t.Fatalf("error = %v, want errNoInputFiles", err)
	}
}

func TestRunCompileDiagnosticsOnError(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "broken.zen", inspectBrokenSchema)
	outDir := filepath.Join(dir, "out")

	err := runCompile([]string{"--backend=proto", "--out=" + outDir, schemaPath})
	if err == nil {
		t.Fatalf("expected error for broken schema, got nil")
	}
}

func TestRunCompileWithCoreWritesSummary(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "out")

	var out bytes.Buffer
	cfg := CompileConfig{
		Files:    []string{schemaPath},
		Backends: "proto",
		OutDir:   outDir,
		Out:      &out,
	}

	if err := runCompileWith(cfg); err != nil {
		t.Fatalf("runCompileWith: %v", err)
	}

	if !strings.Contains(out.String(), "1 file(s)") {
		t.Fatalf("output = %q, want success summary", out.String())
	}

	if _, err := os.Stat(filepath.Join(outDir, "proto", "schema.proto")); err != nil {
		t.Fatalf("expected proto output to exist: %v", err)
	}
}

func TestRunCompileWithDefaultsOutDir(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var out bytes.Buffer
	// Empty OutDir falls back to ./generated. Run from the temp dir so
	// ./generated lands there. (Backends stay explicit: the full default
	// set shells out to protoc toolchains, which unit tests must not need.)
	t.Chdir(dir)

	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(dir, "generated"))
	})

	cfg := CompileConfig{
		Files:    []string{schemaPath},
		Backends: "proto,openapi,zenorm",
		Out:      &out,
	}

	if err := runCompileWith(cfg); err != nil {
		t.Fatalf("runCompileWith with defaults: %v", err)
	}

	for _, rel := range []string{
		filepath.Join("generated", "proto", "schema.proto"),
		filepath.Join("generated", "openapi", "openapi.json"),
		filepath.Join("generated", "zenorm", "orm", "gen", "app", "app.go"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("expected %q to exist: %v", rel, err)
		}
	}
}

func TestRunCompileWithEmptyFiles(t *testing.T) {
	var out bytes.Buffer
	if err := runCompileWith(CompileConfig{Out: &out}); err == nil {
		t.Fatalf("expected error for empty file list, got nil")
	}
}

func TestRunCompileWithMissingFile(t *testing.T) {
	var out bytes.Buffer
	err := runCompileWith(CompileConfig{
		Files:    []string{filepath.Join(t.TempDir(), "nope.zen")},
		Backends: "proto",
		OutDir:   t.TempDir(),
		Out:      &out,
	})
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}

func TestRunCompileWithUnknownBackendSuggests(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var out bytes.Buffer
	err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "protto",
		OutDir:   filepath.Join(dir, "out"),
		Out:      &out,
	})
	if err == nil {
		t.Fatalf("expected error for near-miss backend, got nil")
	}

	if !strings.Contains(err.Error(), `did you mean "proto"`) {
		t.Fatalf("error = %q, want a did-you-mean suggestion", err.Error())
	}
}

func TestRunCompileFlagParseError(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	if err := runCompile([]string{"--bogus-flag", schemaPath}); err == nil {
		t.Fatalf("expected flag parse error, got nil")
	}
}

func TestRunCompileBackendTypoSuggests(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	err := runCompile([]string{"--backend=protto", schemaPath})
	if err == nil {
		t.Fatalf("expected error for near-miss backend, got nil")
	}
}

func TestRunCompileInteractiveFlag(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "out")

	prev := interactiveMode
	t.Cleanup(func() { interactiveMode = prev })

	if err := runCompile([]string{"-i", "--backend=proto", "--out=" + outDir, schemaPath}); err != nil {
		t.Fatalf("runCompile -i: %v", err)
	}

	if !interactiveMode {
		t.Fatalf("expected -i to set interactiveMode")
	}
}

func TestRunCompileHelpFlag(t *testing.T) {
	err := runCompile([]string{"-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want flag.ErrHelp", err)
	}
}

func TestRunCompileFlagsAfterPositional(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "out")

	// File first, flags after — previously failed because flag.Parse stops at first non-flag.
	if err := runCompile([]string{schemaPath, "--backend=proto", "--out=" + outDir}); err != nil {
		t.Fatalf("runCompile file-first: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "proto", "schema.proto")); err != nil {
		t.Fatalf("expected proto output to exist: %v", err)
	}
}

func TestRunCompileInterleaved(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "out2")

	// Interleaved: file, flag, file — flexibleParse must collect flags regardless of position.
	if err := runCompile([]string{schemaPath, "--out=" + outDir, "--backend=proto"}); err != nil {
		t.Fatalf("runCompile interleaved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "proto", "schema.proto")); err != nil {
		t.Fatalf("expected proto output to exist: %v", err)
	}
}

func TestRunCompileOpenAPIBackend(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "order.zen", inspectOpenAPIOrderSchema)
	outDir := filepath.Join(dir, "out")

	if err := runCompile([]string{"--backend=openapi", "--out=" + outDir, schemaPath}); err != nil {
		t.Fatalf("runCompile: %v", err)
	}

	for _, rel := range []string{
		filepath.Join(outDir, "openapi", "default", "openapi.json"),
		filepath.Join(outDir, "openapi", "openapi.json"),
	} {
		content, err := os.ReadFile(rel) //nolint:gosec // test fixture path
		if err != nil {
			t.Fatalf("read %q: %v", rel, err)
		}

		var doc map[string]any
		if err := json.Unmarshal(content, &doc); err != nil {
			t.Fatalf("%q is not valid JSON: %v", rel, err)
		}

		for _, key := range []string{"openapi", "info", "paths", "components"} {
			if _, ok := doc[key]; !ok {
				t.Fatalf("%q missing top-level key %q: %v", rel, key, doc)
			}
		}
	}
}

func TestRunCompileUnknownBackendListsAvailable(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "order.zen", inspectOrderSchema)

	err := runCompile([]string{"--backend=nope", "--out=" + filepath.Join(dir, "out"), schemaPath})
	if err == nil {
		t.Fatal("expected an error for an unknown backend")
	}

	if !strings.Contains(err.Error(), "proto") || !strings.Contains(err.Error(), "zenorm") {
		t.Fatalf("error %q should list the available backends", err)
	}
}

func TestRunCompileZenormBackend(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "order.zen", inspectOrderSchema)
	outDir := filepath.Join(dir, "out")

	if err := runCompile([]string{"--backend=zenorm", "--out=" + outDir, schemaPath}); err != nil {
		t.Fatalf("runCompile: %v", err)
	}

	appFile := filepath.Join(outDir, "zenorm", "orm", "gen", "app", "app.go")

	content, err := os.ReadFile(appFile) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %q: %v", appFile, err)
	}

	for _, want := range []string{
		"package app",
		"type User struct",
		"type Order struct",
		"var Users = orm.NewTable[User]",
		"func (e *User) Scan(",
		"github.com/zenta-dev/zever/orm",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("generated %s missing %q:\n%s", appFile, want, content)
		}
	}
}

func TestRunCompileZenormTogetherWithProto(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "order.zen", inspectOrderSchema)
	outDir := filepath.Join(dir, "out")

	if err := runCompile([]string{"--backend=zenorm,proto", "--out=" + outDir, schemaPath}); err != nil {
		t.Fatalf("runCompile: %v", err)
	}

	for _, path := range []string{
		filepath.Join(outDir, "zenorm", "orm", "gen", "app", "app.go"),
		filepath.Join(outDir, "proto", "schema.proto"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %q to exist: %v", path, err)
		}
	}
}

func TestRunCompileWithEmptyBackendsUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "broken.zen", inspectBrokenSchema)

	var out bytes.Buffer
	// Empty Backends selects the registered defaults; the broken schema
	// fails before any backend runs, proving resolution happened.
	err := runCompileWith(CompileConfig{
		Files:  []string{schemaPath},
		OutDir: filepath.Join(dir, "out"),
		Out:    &out,
	})
	if err == nil {
		t.Fatalf("expected error for broken schema, got nil")
	}

	if !strings.Contains(err.Error(), "failed to compile") {
		t.Fatalf("error = %q, want compile failure", err.Error())
	}
}

func TestComputeBackendRootsNoGoMod(t *testing.T) {
	t.Chdir(t.TempDir())

	roots := computeBackendRoots("./generated")
	if roots.pbImportRoot != "" || roots.annotationsGoPackageRoot != "" {
		t.Fatalf("roots = %+v, want empty outside a Go module", roots)
	}
}

func TestResolveBackendsSkipsEmptyEntries(t *testing.T) {
	backends, err := resolveBackends("proto,,", backendRoots{})
	if err != nil {
		t.Fatalf("resolveBackends: %v", err)
	}

	if len(backends) != 1 {
		t.Fatalf("got %d backends, want 1", len(backends))
	}
}

func TestComputeBackendRootsWithGoMod(t *testing.T) {
	for _, tc := range []struct {
		name   string
		outDir string
		rel    string
	}{
		{"dot-slash prefix", "./generated", "generated"},
		{"bare relative", "generated", "generated"},
		{"absolute", "/out/gen", "out/gen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n\ngo 1.24\n"), 0o600); err != nil {
				t.Fatalf("write go.mod: %v", err)
			}
			t.Chdir(dir)

			roots := computeBackendRoots(tc.outDir)
			wantPB := "example.com/foo/" + tc.rel + "/protogogen"
			if roots.pbImportRoot != wantPB {
				t.Fatalf("pbImportRoot = %q, want %q", roots.pbImportRoot, wantPB)
			}
			if roots.annotationsGoPackageRoot != wantPB+"/zever" {
				t.Fatalf("annotationsGoPackageRoot = %q, want %q", roots.annotationsGoPackageRoot, wantPB+"/zever")
			}
		})
	}
}

func TestRunCompileInteractiveLongForm(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "out")

	prev := interactiveMode
	t.Cleanup(func() { interactiveMode = prev })

	if err := runCompile([]string{"--interactive", "--backend=proto", "--out=" + outDir, schemaPath}); err != nil {
		t.Fatalf("runCompile --interactive: %v", err)
	}
	if !interactiveMode {
		t.Fatalf("expected --interactive to set interactiveMode")
	}
}

func TestRunCompileTTYMultiSelect(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "out")
	stubPromptTTY(t, true)
	prevMode := interactiveMode
	interactiveMode = true
	t.Cleanup(func() { interactiveMode = prevMode })

	prevMS, prevIn, prevDisc := promptMultiSelectFn, promptInputFn, discoverZenFilesFn
	t.Cleanup(func() { promptMultiSelectFn, promptInputFn, discoverZenFilesFn = prevMS, prevIn, prevDisc })
	discoverZenFilesFn = func() []string { return []string{schemaPath} }
	calledInput := false
	promptInputFn = func(_, _ string) (string, error) {
		calledInput = true
		return "", nil
	}
	promptMultiSelectFn = func(_ string, _ []string) ([]string, error) {
		return []string{schemaPath}, nil
	}

	if err := runCompile([]string{"--backend=proto", "--out=" + outDir}); err != nil {
		t.Fatalf("runCompile TTY multiselect: %v", err)
	}
	if calledInput {
		t.Fatalf("expected manual prompt to be skipped when multiselect yields files")
	}
	if _, err := os.Stat(filepath.Join(outDir, "proto", "schema.proto")); err != nil {
		t.Fatalf("expected proto output: %v", err)
	}
}

func TestRunCompileTTYFallbackInput(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "out")
	stubPromptTTY(t, true)
	prevMode := interactiveMode
	interactiveMode = true
	t.Cleanup(func() { interactiveMode = prevMode })

	prevMS, prevIn, prevDisc := promptMultiSelectFn, promptInputFn, discoverZenFilesFn
	t.Cleanup(func() { promptMultiSelectFn, promptInputFn, discoverZenFilesFn = prevMS, prevIn, prevDisc })
	discoverZenFilesFn = func() []string { return nil }
	promptMultiSelectFn = func(_ string, _ []string) ([]string, error) {
		t.Fatalf("multiselect must not run with no discovered files")
		return nil, nil
	}
	promptInputFn = func(_, _ string) (string, error) { return schemaPath, nil }

	if err := runCompile([]string{"--backend=proto", "--out=" + outDir}); err != nil {
		t.Fatalf("runCompile TTY fallback: %v", err)
	}
}

func TestRunCompileTTYPromptsFailFallsThrough(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, "schema/app.zen", inspectUserSchema)
	t.Chdir(dir)
	stubPromptTTY(t, true)
	prevMode := interactiveMode
	interactiveMode = true
	t.Cleanup(func() { interactiveMode = prevMode })
	t.Setenv("ZEVER_NO_HINT", "1")

	prevMS, prevIn, prevDisc := promptMultiSelectFn, promptInputFn, discoverZenFilesFn
	t.Cleanup(func() { promptMultiSelectFn, promptInputFn, discoverZenFilesFn = prevMS, prevIn, prevDisc })
	discoverZenFilesFn = discoverZenFiles
	promptMultiSelectFn = func(_ string, _ []string) ([]string, error) {
		return nil, errors.New("no tty")
	}
	promptInputFn = func(_, _ string) (string, error) { return "", errors.New("no tty") }

	outDir := filepath.Join(dir, "out")
	if err := runCompile([]string{"--backend=proto", "--out=" + outDir}); err != nil {
		t.Fatalf("runCompile TTY errors fall through to auto-discovery: %v", err)
	}
}

func TestRunCompileAutoDiscoveryNoPositional(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, "schema/app.zen", inspectUserSchema)
	t.Chdir(dir)
	t.Setenv("ZEVER_NO_HINT", "1")
	stubPromptTTY(t, false)

	outDir := filepath.Join(dir, "out")
	if err := runCompile([]string{"--backend=proto", "--out=" + outDir}); err != nil {
		t.Fatalf("runCompile auto-discovery: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "proto", "schema.proto")); err != nil {
		t.Fatalf("expected proto output: %v", err)
	}
}

func TestRunCompileWithWriteOutputsError(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	var out bytes.Buffer
	err := runCompileWith(CompileConfig{
		Files:    []string{schemaPath},
		Backends: "proto",
		OutDir:   blocker,
		Out:      &out,
	})
	if err == nil {
		t.Fatalf("expected writeOutputs error when outDir is a file, got nil")
	}
}

func TestPrintDiagnosticsSeverities(t *testing.T) {
	captureStderr(t, func() {
		printDiagnostics(diag.List{
			{Pos: diag.Position{File: "a.zen", Line: 1, Col: 1}, Severity: diag.SeverityError, Phase: "parse", Msg: "boom"},
			{Pos: diag.Position{File: "a.zen", Line: 2, Col: 1}, Severity: diag.SeverityWarning, Phase: "resolve", Msg: "careful"},
			{Pos: diag.Position{File: "a.zen", Line: 3, Col: 1}, Severity: diag.Severity(99), Phase: "x", Msg: "info"},
		})
	})
}

func TestWriteOutputsErrors(t *testing.T) {
	t.Run("mkdir fails", func(t *testing.T) {
		dir := t.TempDir()
		blocker := filepath.Join(dir, "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		outputs := map[string]map[string][]byte{"proto": {"schema.proto": []byte("x")}}
		if err := writeOutputs(blocker, outputs); err == nil {
			t.Fatalf("expected mkdir error, got nil")
		}
	})
	t.Run("write fails", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(dir, "out")
		destDir := filepath.Join(outDir, "proto")
		if err := os.MkdirAll(destDir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Pre-create a directory where the file should go: WriteFile on a
		// directory fails while MkdirAll succeeds.
		if err := os.MkdirAll(filepath.Join(destDir, "schema.proto"), 0o750); err != nil {
			t.Fatalf("mkdir dest dir: %v", err)
		}
		outputs := map[string]map[string][]byte{"proto": {"schema.proto": []byte("x")}}
		if err := writeOutputs(outDir, outputs); err == nil {
			t.Fatalf("expected write error, got nil")
		}
	})
}
