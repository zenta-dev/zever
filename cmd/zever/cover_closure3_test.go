package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClosure3SeedStubRender(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/app")

	// Newline generated_dir breaks the stub's gofmt; --force gets past the
	// entrypoint collision so the stub render is what fails.
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  generated_dir: \"a\\nb\"\n")

	if err := runGenerateSeed([]string{"--force"}); err == nil {
		t.Fatal("want stub render error")
	}

	if err := os.Remove(filepath.Join(dir, "zever.yaml")); err != nil {
		t.Fatal(err)
	}

	// loadProjectConfig error inside runGenerateSeed.
	writeZeverFixture(t, dir, "zever.yaml", "project: [unclosed\n")

	if err := runGenerateSeed(nil); err == nil {
		t.Fatal("want project error")
	}
}

func TestClosure3ServerLoadFiles(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	// Explicit but unreadable file: resolveInputFiles passes it through,
	// loadFiles fails.
	locked := writeZeverFixture(t, dir, "locked.zen", "entity A {\n\tid: uuid @primary\n}\n")

	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })

	if _, err := loadServerData("example.com/app", "./generated", "schema", []string{locked}, false); err == nil {
		t.Fatal("want load error")
	}

	_ = os.Chmod(locked, 0o600)

	// Auto-discovery path: no explicit files, schema dir has content.
	writeZeverFixture(t, dir, filepath.Join("schema", "ok.zen"), "entity A {\n\tid: uuid @primary\n}\n")

	if _, err := loadServerData("example.com/app", "./generated", "schema", nil, false); err != nil {
		t.Fatalf("discovery: %v", err)
	}
}

func TestClosure3AdapterHint(t *testing.T) {
	stubPromptTTY(t, false)

	dir := t.TempDir()
	withWorkingDir(t, dir)

	// Near-miss battery prints the did-you-mean hint (hints enabled by default).
	if err := runGenerateAdapter([]string{"dbs", "x"}); err == nil {
		t.Fatal("want unknown battery error")
	}
}

func TestClosure3TinkerAppValidator(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	origIn := promptInputForTinkerGen
	t.Cleanup(func() { promptInputForTinkerGen = origIn })

	dir := t.TempDir()
	withWorkingDir(t, dir)

	// No go.mod: outdir prompt keeps default, app prompt exercises its
	// validator then succeeds.
	promptInputForTinkerGen = func(title, def string, v func(string) error) (string, error) {
		if title == "Output directory" {
			return def, nil
		}

		if v != nil {
			_ = v("")
			_ = v("   ")
			_ = v("example.com/app/internal/app")
		}

		return "example.com/app/internal/app", nil
	}

	if err := runGenerateTinker([]string{"--force"}); err != nil {
		t.Fatalf("tinker app validator: %v", err)
	}
}

func TestClosure3ExtractGofileErrors(t *testing.T) {
	projDir, _ := setupExtractProject(t)

	schema, err := compileSchemaDir("tag", "schema")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	gm := goModInfo{ModulePath: "example.com/shop", GoVersion: "1.24", FrameworkVersion: "v0.0.0", FrameworkDir: "."}

	// First go-file write error: block internal/app with a file.
	plan, err := planExtraction("tag", ExtractConfig{Module: "billing", OutDir: filepath.Join(projDir, "out-app")}, schema, gm)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	writeZeverFixture(t, projDir, filepath.Join("out-app", "internal", "app"), "blocker")

	if writeErr := writeExtraction("tag", plan, gm); writeErr == nil {
		t.Fatal("want app write error")
	}

	// Job-stub write error: read-only service root under the output dir.
	plan2, err := planExtraction("tag", ExtractConfig{Module: "billing", OutDir: filepath.Join(projDir, "out-jobs")}, schema, gm)
	if err != nil {
		t.Fatalf("plan2: %v", err)
	}

	svc := filepath.Join(projDir, "out-jobs", "internal", "service")
	if err := os.MkdirAll(svc, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(svc, 0o555); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(svc, 0o750) })

	if err := writeExtraction("tag", plan2, gm); err == nil {
		t.Fatal("want job stub error")
	}
}
