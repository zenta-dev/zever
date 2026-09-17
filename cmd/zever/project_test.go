package main

import (
	"os"
	"path/filepath"
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

// writeProjectFixture chdirs into a fresh temp dir containing name/content, so
// loadProjectConfig's working-directory discovery finds exactly that file and
// no state leaks between tests.
func writeProjectFixture(t *testing.T, name, content string) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	t.Chdir(dir)
}

func TestLoadProjectConfigNoFileReturnsDefaults(t *testing.T) {
	t.Chdir(t.TempDir())

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	if got != defaultProjectConfig() {
		t.Fatalf("got %+v, want %+v", got, defaultProjectConfig())
	}
}

// TestLoadProjectConfigPartialOverride asserts the same two-field override
// decodes identically from every supported file format, and that the other
// four fields still fall back to their defaults.
func TestLoadProjectConfigPartialOverride(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
	}{
		{
			name: "yaml",
			file: "zever.yaml",
			content: `project:
  schema_dir: src/zen
  server_entry: cmd/api
`,
		},
		{
			name: "yml",
			file: "zever.yml",
			content: `project:
  schema_dir: src/zen
  server_entry: cmd/api
`,
		},
		{
			name:    "json",
			file:    "zever.json",
			content: `{"project": {"schema_dir": "src/zen", "server_entry": "cmd/api"}}`,
		},
	}

	want := defaultProjectConfig()
	want.SchemaDir = "src/zen"
	want.ServerEntry = "cmd/api"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeProjectFixture(t, tt.file, tt.content)

			got, err := loadProjectConfig()
			if err != nil {
				t.Fatalf("loadProjectConfig: %v", err)
			}

			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

// TestLoadProjectConfigCoexistsWithBatteryConfig is the key regression test:
// one physical file carries both the CLI's project table and service tables
// owned by config.Load. Each loader must read its own section and ignore the
// other's without erroring.
func TestLoadProjectConfigCoexistsWithBatteryConfig(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
	}{
		{
			name: "yaml",
			file: "zever.yaml",
			content: `db:
  adapter: sqlite
  options:
    path: file:app.db
project:
  generated_dir: gen
  seed_entry: db/fixtures
cache:
  adapter: memory
`,
		},
		{
			name: "yml",
			file: "zever.yml",
			content: `db:
  adapter: sqlite
project:
  generated_dir: gen
  seed_entry: db/fixtures
cache:
  adapter: memory
`,
		},
		{
			name: "json",
			file: "zever.json",
			content: `{
  "db": {"adapter": "sqlite", "options": {"path": "file:app.db"}},
  "project": {"generated_dir": "gen", "seed_entry": "db/fixtures"},
  "cache": {"adapter": "memory"}
}`,
		},
	}

	want := defaultProjectConfig()
	want.GeneratedDir = "gen"
	want.SeedEntry = "db/fixtures"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeProjectFixture(t, tt.file, tt.content)

			got, err := loadProjectConfig()
			if err != nil {
				t.Fatalf("loadProjectConfig: %v", err)
			}

			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

// TestLoadProjectConfigDiscoveryOrder asserts YAML wins over yml and JSON
// when several config files sit side by side, matching config.Load's order.
func TestLoadProjectConfigDiscoveryOrder(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"zever.yaml": "project:\n  schema_dir: from-yaml\n",
		"zever.yml":  "project:\n  schema_dir: from-yml\n",
		"zever.json": `{"project": {"schema_dir": "from-json"}}`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	t.Chdir(dir)

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	if got.SchemaDir != "from-yaml" {
		t.Fatalf("SchemaDir = %q, want %q", got.SchemaDir, "from-yaml")
	}
}

// TestLoadProjectConfigYmlBeatsJson pins the second half of the discovery
// order when no .yaml file is present.
func TestLoadProjectConfigYmlBeatsJson(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"zever.yml":  "project:\n  schema_dir: from-yml\n",
		"zever.json": `{"project": {"schema_dir": "from-json"}}`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	t.Chdir(dir)

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	if got.SchemaDir != "from-yml" {
		t.Fatalf("SchemaDir = %q, want %q", got.SchemaDir, "from-yml")
	}
}

// TestDecodeProjectFileRejectsUnsupportedExtension pins the format boundary:
// TOML (which zengo supported) is not decoded by the zever project pass,
// matching config.decodeFile's YAML/JSON-only support.
func TestDecodeProjectFileRejectsUnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zever.toml")
	if err := os.WriteFile(path, []byte("[project]\nschema_dir = \"src/zen\"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := decodeProjectFile(path); err == nil {
		t.Fatal("expected an error for a .toml project file")
	}
}

// TestDecodeProjectFileRejectsBadContent covers malformed files: an error
// must surface, never a half-decoded config.
func TestDecodeProjectFileRejectsBadContent(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
	}{
		{name: "yaml", file: "zever.yaml", content: "project:\n\tschema_dir: [unclosed\n"},
		{name: "json", file: "zever.json", content: `{"project": {"schema_dir": `},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tt.file)
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			if _, err := decodeProjectFile(path); err == nil {
				t.Fatalf("expected an error for malformed %s", tt.file)
			}
		})
	}
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
