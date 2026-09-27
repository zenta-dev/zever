package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validateGoSyntax is defined in generate_server_test.go and shared across
// every generate_*_test.go file that checks scaffolded Go output.

func TestRenderTinkerShimIsValidGo(t *testing.T) {
	t.Parallel()

	src, err := renderTinkerShim("example.com/proj/internal/app", "cmd/tinker-shim")
	if err != nil {
		t.Fatalf("renderTinkerShim: %v", err)
	}

	validateGoSyntax(t, "main.go", src)

	text := string(src)

	for _, want := range []string{
		`app "example.com/proj/internal/app"`,
		`app.New()`,
		tinkerFramePrefix,
		`case "ping":`,
		`case "db.query":`,
		`case "db.exec":`,
		`case "cache.get":`,
		`case "cache.set":`,
		`case "cache.delete":`,
		`case "cache.exists":`,
		`case "queue.push":`,
		`case "queue.length":`,
		`case "job.dispatch":`,
		"MUST STAY IN SYNC WITH cmd/zever/tinker_protocol.go",
		`go run ./cmd/tinker-shim`,
		`"github.com/zenta-dev/zever/container"`,
		`"github.com/zenta-dev/zever/core/db"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generated shim does not contain %q", want)
		}
	}

	assertGolden(t, "generate_tinker_shim.golden", src)
}

// TestRenderTinkerShimCoversEveryVerb keeps the generated switch and the
// protocol's verb constants from drifting apart.
func TestRenderTinkerShimCoversEveryVerb(t *testing.T) {
	t.Parallel()

	src, err := renderTinkerShim("example.com/proj/internal/app", "cmd/tinker-shim")
	if err != nil {
		t.Fatalf("renderTinkerShim: %v", err)
	}

	verbs := []string{
		verbPing, verbDBQuery, verbDBExec,
		verbCacheGet, verbCacheSet, verbCacheDelete, verbCacheExists,
		verbQueuePush, verbQueueLength, verbJobDispatch,
	}

	for _, v := range verbs {
		if !strings.Contains(string(src), `case "`+v+`":`) {
			t.Errorf("generated shim has no case for verb %q", v)
		}
	}
}

func TestRunGenerateTinkerWritesShim(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if err := os.WriteFile("go.mod", []byte("module example.com/proj\n\ngo 1.27.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := runGenerateTinker(nil); err != nil {
		t.Fatalf("runGenerateTinker: %v", err)
	}

	path := filepath.Join(defaultTinkerEntry, "main.go")

	src, err := os.ReadFile(path) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatalf("read generated shim: %v", err)
	}

	validateGoSyntax(t, path, src)

	if !strings.Contains(string(src), `app "example.com/proj/internal/app"`) {
		t.Errorf("app import path was not derived from go.mod:\n%s", src)
	}

	// A second run must refuse to clobber the developer's edits.
	if err := runGenerateTinker(nil); err == nil {
		t.Fatal("expected second runGenerateTinker to refuse without --force")
	}

	if err := runGenerateTinker([]string{"--force"}); err != nil {
		t.Fatalf("runGenerateTinker --force: %v", err)
	}
}

func TestRunGenerateTinkerRespectsFlags(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	err := runGenerateTinker([]string{"--app", "other.example/x/app", "--dir", "tools/shim"})
	if err != nil {
		t.Fatalf("runGenerateTinker: %v", err)
	}

	src, rerr := os.ReadFile(filepath.Join("tools", "shim", "main.go")) //nolint:gosec // test-controlled path
	if rerr != nil {
		t.Fatalf("read generated shim: %v", rerr)
	}

	if !strings.Contains(string(src), `app "other.example/x/app"`) {
		t.Errorf("--app was not honored:\n%s", src)
	}
}

// TestRunGenerateTinkerNeedsGoMod: without --app the module path has to come
// from go.mod, so a missing one must be a clear error, not a broken scaffold.
func TestRunGenerateTinkerNeedsGoMod(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	err := runGenerateTinker(nil)
	if err == nil {
		t.Fatal("expected an error when go.mod is absent and --app is unset")
	}

	if !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("error should mention go.mod, got: %v", err)
	}
}

// TestGenerateTinkerRejectsTraversal pins the output-directory gate.
func TestGenerateTinkerRejectsTraversal(t *testing.T) {
	if _, err := GenerateTinker(GenerateTinkerConfig{AppPackage: "example.com/x/app", OutDir: ""}); err == nil {
		t.Fatal("expected an error for an empty output directory")
	}

	if _, err := GenerateTinker(GenerateTinkerConfig{AppPackage: "", OutDir: "tools/shim"}); err == nil {
		t.Fatal("expected an error for an empty app package")
	}
}

func TestRunGenerateDispatch(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if err := os.WriteFile("go.mod", []byte("module example.com/proj\n\ngo 1.27.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := runGenerate([]string{"tinker"}); err != nil {
		t.Fatalf("generate tinker: %v", err)
	}

	if _, err := os.Stat(filepath.Join(defaultTinkerEntry, "main.go")); err != nil {
		t.Fatalf("generate tinker did not write the shim: %v", err)
	}

	if err := runGenerate([]string{"module", "billing"}); err != nil {
		t.Fatalf("generate module: %v", err)
	}

	if _, err := os.Stat(filepath.Join("schema", "billing", "billing.zen")); err != nil {
		t.Fatalf("generate module did not write the stub: %v", err)
	}

	if err := runGenerate([]string{"nonsense"}); err == nil {
		t.Fatal("expected an error for an unknown generate target")
	}
}

// chdir moves the process into dir for the duration of the test. The
// generate/tinker commands are working-directory relative by design (they
// discover go.mod and zever.* the way a developer's shell would), so their
// tests cannot run in parallel with each other.
func chdir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
}
