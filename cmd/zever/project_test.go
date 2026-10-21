package main

import (
	"os"
	"strings"
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

// TestLoadProjectConfigIgnoresLegacyConfigFile pins the clean break: a
// pre-v0.8.0 config file in the working directory steers nothing. Layout
// comes from the environment, so a stray config file -- malformed or not --
// leaves the defaults in place.
func TestLoadProjectConfigIgnoresLegacyConfigFile(t *testing.T) {
	dir := t.TempDir()
	writeProjectFixture(t, dir, strayConfigFile, "project:\n  schema_dir: src/zen\n")

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	if got != defaultProjectConfig() {
		t.Fatalf("got %+v, want %+v", got, defaultProjectConfig())
	}
}

// TestLoadProjectConfigReadsDotEnv pins that the layout keys are read from
// the project's .env at the project root, exactly like every other setting
// in the framework: a key the CLI never read from a file would make the
// help text a lie.
func TestLoadProjectConfigReadsDotEnv(t *testing.T) {
	dir := t.TempDir()
	writeProjectFixture(t, dir, ".env", EnvSchemaDir+"=src/zen\n"+EnvServerEntry+"=cmd/api\n")

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	want := defaultProjectConfig()
	want.SchemaDir = "src/zen"
	want.ServerEntry = "cmd/api"

	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// TestLoadProjectConfigPrecedence pins the resolution order the framework
// uses everywhere: defaults < .env < .env.local < process environment. The
// knob walks all four layers, so a project can move its entrypoint in the
// file it already keeps its settings in, override it per machine in
// .env.local, and override both from the shell.
func TestLoadProjectConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// The process layer must be absent, not empty: an empty value still
	// shadows the files, because the process environment is the top layer.
	unsetEnv(t, EnvServerEntry)

	// Nothing set anywhere: the compiled-in default wins.
	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig (defaults): %v", err)
	}

	if got.ServerEntry != defaultServerEntry {
		t.Fatalf("ServerEntry = %q, want the default %q", got.ServerEntry, defaultServerEntry)
	}

	// .env at the project root wins over the default.
	writeZeverFixture(t, dir, ".env", EnvServerEntry+"=cmd/api\n")

	got, err = loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig (.env): %v", err)
	}

	if got.ServerEntry != "cmd/api" {
		t.Fatalf("ServerEntry = %q, want %q", got.ServerEntry, "cmd/api")
	}

	// .env.local wins over .env.
	writeZeverFixture(t, dir, ".env.local", EnvServerEntry+"=cmd/local\n")

	got, err = loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig (.env.local): %v", err)
	}

	if got.ServerEntry != "cmd/local" {
		t.Fatalf("ServerEntry = %q, want %q", got.ServerEntry, "cmd/local")
	}

	// The process environment wins over both files.
	t.Setenv(EnvServerEntry, "cmd/proc")

	got, err = loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig (process env): %v", err)
	}

	if got.ServerEntry != "cmd/proc" {
		t.Fatalf("ServerEntry = %q, want %q", got.ServerEntry, "cmd/proc")
	}
}

// TestLoadProjectConfigMalformedDotEnvFailsClosed pins the fail-closed
// rule the framework applies to every env read: a malformed .env names the
// file and the line instead of silently resolving to the defaults.
func TestLoadProjectConfigMalformedDotEnvFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeProjectFixture(t, dir, ".env", "this line has no equals sign\n")

	_, err := loadProjectConfig()
	if err == nil {
		t.Fatal("want an error for an unparsable .env, got nil")
	}

	if !strings.Contains(err.Error(), ".env") {
		t.Fatalf("error %q does not name the file", err)
	}
}

// TestLoadProjectConfigHonorsEnv pins the ZEVER_* overrides: every layout
// field resolves defaults < environment, with whitespace-only values
// counting as unset.
func TestLoadProjectConfigHonorsEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EnvSchemaDir, "src/zen")
	t.Setenv(EnvGeneratedDir, "out/gen")
	t.Setenv(EnvServerEntry, "cmd/api")
	t.Setenv(EnvWorkerEntry, "cmd/jobs")
	t.Setenv(EnvTinkerEntry, "cmd/shim")
	t.Setenv(EnvSeedEntry, "db/bootstrap")

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	want := ProjectConfig{
		SchemaDir:    "src/zen",
		GeneratedDir: "out/gen",
		ServerEntry:  "cmd/api",
		WorkerEntry:  "cmd/jobs",
		TinkerEntry:  "cmd/shim",
		SeedEntry:    "db/bootstrap",
	}

	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// TestLoadProjectConfigEnvWhitespaceCountsAsUnset pins that a
// whitespace-only ZEVER_* value falls back to its default.
func TestLoadProjectConfigEnvWhitespaceCountsAsUnset(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EnvServerEntry, "  ")

	got, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}

	if got.ServerEntry != defaultServerEntry {
		t.Fatalf("ServerEntry = %q, want default %q", got.ServerEntry, defaultServerEntry)
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

// unsetEnv removes key from the process environment for the duration of the
// test and restores the prior value afterwards. testing.T has no Unsetenv,
// and the knob precedence needs the process layer to be absent: an empty
// value still shadows the dotenv files, because the process environment is
// the top layer.
func unsetEnv(t *testing.T, key string) {
	t.Helper()

	old, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}

	t.Cleanup(func() {
		if had {
			//nolint:usetesting // t.Setenv cannot run inside Cleanup; restore directly.
			_ = os.Setenv(key, old)
		}
	})
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
