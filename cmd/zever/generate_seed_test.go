package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGenerateSeed(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	if err := runGenerateSeed(nil); err != nil {
		t.Fatalf("runGenerateSeed: %v", err)
	}

	main := readGenerated(t, filepath.Join(dir, "db", "seed", "main.go"))

	for _, fragment := range []string{
		`"example.com/shop/internal/app"`,
		`"example.com/shop/internal/service/seed"`,
		"func main()",
		"database.Ping(ctx)",
		"seed.Run(ctx, database)",
	} {
		if !strings.Contains(main, fragment) {
			t.Fatalf("seed main.go lacks %q:\n%s", fragment, main)
		}
	}

	// A seeder neither serves nor consumes, so it wires up neither.
	for _, unwanted := range []string{"net/http", "c.Queue()", "c.Router()"} {
		if strings.Contains(main, unwanted) {
			t.Fatalf("seed main.go should not mention %q:\n%s", unwanted, main)
		}
	}

	readGenerated(t, filepath.Join(dir, "internal", "app", "app.go"))

	stub := readGenerated(t, filepath.Join(dir, "internal", "service", "seed", "seed.go"))

	for _, fragment := range []string{
		"package seed",
		"func Run(ctx context.Context, database db.DB) error {",
		// The worked example points at the project's generated ORM directory.
		"generated/",
	} {
		if !strings.Contains(stub, fragment) {
			t.Fatalf("seed stub lacks %q:\n%s", fragment, stub)
		}
	}

	assertGolden(t, "generate_seed_main.golden", []byte(main))
	assertGolden(t, "generate_seed_stub.golden", []byte(stub))
}

// TestRunGenerateSeedNeverOverwritesAnExistingStub proves the skip-if-exists
// contract: re-running (even with --force, which only governs the
// always-regenerated main.go) after a developer has started implementing
// seed logic must leave it untouched.
func TestRunGenerateSeedNeverOverwritesAnExistingStub(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	if err := runGenerateSeed(nil); err != nil {
		t.Fatalf("runGenerateSeed: %v", err)
	}

	stubPath := filepath.Join(dir, "internal", "service", "seed", "seed.go")

	const handwritten = "package seed\n\n// hand written seed rows\n"

	if err := os.WriteFile(stubPath, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write handwritten stub: %v", err)
	}

	if err := runGenerateSeed([]string{"--force"}); err != nil {
		t.Fatalf("runGenerateSeed --force: %v", err)
	}

	if got := readFile(t, stubPath); got != handwritten {
		t.Fatalf("re-running generate seed overwrote hand-implemented seed logic: %q", got)
	}
}

func TestRunGenerateSeedHonoursProjectConfig(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	const cfg = "project:\n  seed_entry: cmd/seeder\n  generated_dir: internal/orm\n"

	if err := os.WriteFile(filepath.Join(dir, "zever.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write zever.yaml: %v", err)
	}

	if err := runGenerateSeed(nil); err != nil {
		t.Fatalf("runGenerateSeed: %v", err)
	}

	readGenerated(t, filepath.Join(dir, "cmd", "seeder", "main.go"))

	stub := readGenerated(t, filepath.Join(dir, "internal", "service", "seed", "seed.go"))

	if !strings.Contains(stub, "internal/orm/") {
		t.Fatalf("seed stub ignored project.generated_dir:\n%s", stub)
	}
}

func TestRunGenerateSeedRefusesToClobberWithoutForce(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	if err := runGenerateSeed(nil); err != nil {
		t.Fatalf("runGenerateSeed: %v", err)
	}

	if err := runGenerateSeed(nil); err == nil {
		t.Fatalf("expected an error for an existing entrypoint")
	}

	if err := runGenerateSeed([]string{"--force"}); err != nil {
		t.Fatalf("runGenerateSeed --force: %v", err)
	}
}

func TestRunGenerateSeedWithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerateSeed(nil); err == nil {
		t.Fatalf("expected an error outside a Go module")
	}
}
