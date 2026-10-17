package main

import (
	"testing"
)

func defaultProjectConfig() ProjectConfig {
	return ProjectConfig{
		SchemaDir:    "schema",
		GeneratedDir: "generated",
		ServerEntry:  "cmd/server",
		WorkerEntry:  "cmd/worker",
		TinkerEntry:  "cmd/tinker-shim",
		SeedEntry:    "db/seed",
	}
}

func TestLoadProjectConfigReturnsDefaults(t *testing.T) {
	t.Chdir(t.TempDir())

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	if got != defaultProjectConfig() {
		t.Fatalf("got %+v, want %+v", got, defaultProjectConfig())
	}
}

// TestLoadProjectConfigIgnoresFiles pins the defaults-only stub: legacy
// config files in the working directory are not parsed.
func TestLoadProjectConfigIgnoresFiles(t *testing.T) {
	dir := t.TempDir()
	writeProjectFixture(t, dir, "zever.yaml", "project:\n  schema_dir: src/zen\n")
	writeProjectFixture(t, dir, ".env", "SCHEMA_DIR=src/zen\n")

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	if got != defaultProjectConfig() {
		t.Fatalf("got %+v, want %+v", got, defaultProjectConfig())
	}
}

// writeProjectFixture chdirs into a fresh temp dir containing name/content,
// so tests observe exactly that working directory and no state leaks
// between tests.
func writeProjectFixture(t *testing.T, dir, name, content string) {
	t.Helper()

	writeZeverFixture(t, dir, name, content)

	t.Chdir(dir)
}

// TestProjectConfigWithDefaultsTrimsWhitespace pins the empty/whitespace
// fallback per field: a whitespace-only override is the same as unset.
func TestProjectConfigWithDefaultsTrimsWhitespace(t *testing.T) {
	got := ProjectConfig{SchemaDir: "  ", ServerEntry: "\t"}.withDefaults()

	if got.SchemaDir != defaultSchemaDir {
		t.Fatalf("SchemaDir = %q, want default %q", got.SchemaDir, defaultSchemaDir)
	}

	if got.ServerEntry != defaultServerEntry {
		t.Fatalf("ServerEntry = %q, want default %q", got.ServerEntry, defaultServerEntry)
	}
}
