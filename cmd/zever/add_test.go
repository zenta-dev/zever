package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAddFixture lays out a minimal calling project in dir: a go.mod
// requiring the framework root at v0.4.0 (plus a root replace when
// frameworkDir is set), a floor internal/app/app.go, and a floor
// zever.yaml. It returns the starting file contents for later comparison.
func writeAddFixture(t *testing.T, dir, frameworkDir string) {
	t.Helper()

	gomod := "module example.com/shop\n\ngo 1.24\n\nrequire github.com/zenta-dev/zever v0.4.0\n"
	if frameworkDir != "" {
		gomod += "replace github.com/zenta-dev/zever => " + frameworkDir + "\n"
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	app, err := renderAppContent("test", coreBatterySelections())
	if err != nil {
		t.Fatalf("renderAppContent: %v", err)
	}

	appPath := filepath.Join(dir, "internal", "app", "app.go")

	if merr := os.MkdirAll(filepath.Dir(appPath), 0o750); merr != nil {
		t.Fatalf("mkdir app: %v", merr)
	}

	if werr := os.WriteFile(appPath, app, 0o600); werr != nil {
		t.Fatalf("write app.go: %v", werr)
	}

	yamlData, err := renderZeverYaml(NewConfig{Batteries: []string{"log", "router"}})
	if err != nil {
		t.Fatalf("renderZeverYaml: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "zever.yaml"), append([]byte(zeverYamlSchemaModeline), yamlData...), 0o600); err != nil {
		t.Fatalf("write zever.yaml: %v", err)
	}
}

func TestParseAddArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		arg         string
		wantBattery string
		wantAdapter string
		wantErr     string
	}{
		{name: "battery only resolves default", arg: "cache", wantBattery: "cache", wantAdapter: "memory"},
		{name: "explicit adapter", arg: "cache/redis", wantBattery: "cache", wantAdapter: "redis"},
		{name: "db postgres", arg: "db/postgres", wantBattery: "db", wantAdapter: "postgres"},
		{name: "password default is phc canonical", arg: "password", wantBattery: "password", wantAdapter: "argon2id"},
		{name: "empty", arg: "", wantErr: "empty argument"},
		{name: "missing battery", arg: "/redis", wantErr: "no empty part"},
		{name: "missing adapter", arg: "cache/", wantErr: "no empty part"},
		{name: "extra slash", arg: "cache/redis/extra", wantErr: "single optional"},
		{name: "unknown battery", arg: "kafka", wantErr: `unknown battery "kafka"`},
		{name: "uppercase battery", arg: "Cache", wantErr: `unknown battery "Cache"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			battery, adapter, err := parseAddArg(tc.arg)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("parseAddArg(%q) succeeded, want error containing %q", tc.arg, tc.wantErr)
				}

				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseAddArg(%q) error = %v, want %q", tc.arg, err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseAddArg(%q): %v", tc.arg, err)
			}

			if battery != tc.wantBattery || adapter != tc.wantAdapter {
				t.Fatalf("parseAddArg(%q) = (%q, %q), want (%q, %q)", tc.arg, battery, adapter, tc.wantBattery, tc.wantAdapter)
			}
		})
	}
}

func TestRunAddEditsProjectFiles(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")

	if err := runAdd([]string{"cache/redis"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	gomod := readFile(t, filepath.Join(dir, "go.mod"))

	for _, fragment := range []string{
		"require github.com/zenta-dev/zever/core/cache v0.4.0",
		"require github.com/zenta-dev/zever/adapters/cache/redis v0.4.0",
	} {
		if !strings.Contains(gomod, fragment) {
			t.Fatalf("go.mod lacks %q:\n%s", fragment, gomod)
		}
	}

	// A published-version project gets no local replaces.
	if strings.Contains(gomod, "replace github.com/zenta-dev/zever/adapters/") {
		t.Fatalf("go.mod must not gain adapter replaces without a local checkout:\n%s", gomod)
	}

	app := readGenerated(t, filepath.Join(dir, "internal", "app", "app.go"))

	for _, fragment := range []string{
		`cacheredis "github.com/zenta-dev/zever/adapters/cache/redis"`,
		"cacheredis.Register()",
		// The floor survives alongside the addition.
		"logslog.Register()",
		"routerstdhttp.Register()",
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("app.go lacks %q:\n%s", fragment, app)
		}
	}

	yamlContent := readFile(t, filepath.Join(dir, "zever.yaml"))

	for _, fragment := range []string{"cache:", "adapter: redis", "log:", "router:"} {
		if !strings.Contains(yamlContent, fragment) {
			t.Fatalf("zever.yaml lacks %q:\n%s", fragment, yamlContent)
		}
	}

	// Re-running the same add is a no-op: byte-identical files.
	before := map[string]string{
		"go.mod":     gomod,
		"app.go":     app,
		"zever.yaml": yamlContent,
	}

	if err := runAdd([]string{"cache/redis"}); err != nil {
		t.Fatalf("runAdd again: %v", err)
	}

	for file, want := range map[string]string{
		"go.mod":     readFile(t, filepath.Join(dir, "go.mod")),
		"app.go":     readGenerated(t, filepath.Join(dir, "internal", "app", "app.go")),
		"zever.yaml": readFile(t, filepath.Join(dir, "zever.yaml")),
	} {
		if want != before[file] {
			t.Fatalf("%s changed on re-add:\n--- before ---\n%s\n--- after ---\n%s", file, before[file], want)
		}
	}
}

func TestRunAddResolvesDefaultAdapter(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")

	if err := runAdd([]string{"cache"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	gomod := readFile(t, filepath.Join(dir, "go.mod"))
	if !strings.Contains(gomod, "require github.com/zenta-dev/zever/adapters/cache/memory v0.4.0") {
		t.Fatalf("go.mod lacks the default memory adapter require:\n%s", gomod)
	}

	yamlContent := readFile(t, filepath.Join(dir, "zever.yaml"))
	if !strings.Contains(yamlContent, "adapter: memory") {
		t.Fatalf("zever.yaml lacks the default memory stanza:\n%s", yamlContent)
	}
}

func TestRunAddDerivesLocalReplaces(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "/opt/zever")

	if err := runAdd([]string{"db/postgres"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	gomod := readFile(t, filepath.Join(dir, "go.mod"))

	for _, fragment := range []string{
		"require github.com/zenta-dev/zever/core/db v0.4.0",
		"require github.com/zenta-dev/zever/adapters/db/postgres v0.4.0",
		"replace github.com/zenta-dev/zever/core/db => /opt/zever/core/db",
		"replace github.com/zenta-dev/zever/adapters/db/postgres => /opt/zever/adapters/db/postgres",
	} {
		if !strings.Contains(gomod, fragment) {
			t.Fatalf("go.mod lacks %q:\n%s", fragment, gomod)
		}
	}
}

func TestRunAddMatchesPinnedVersion(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")

	gomodPath := filepath.Join(dir, "go.mod")

	data, err := os.ReadFile(gomodPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	pinned := strings.Replace(string(data), "require github.com/zenta-dev/zever v0.4.0", "require github.com/zenta-dev/zever v1.9.9", 1)

	if werr := os.WriteFile(gomodPath, []byte(pinned), 0o600); werr != nil { //nolint:gosec // test temp dir
		t.Fatalf("write go.mod: %v", werr)
	}

	if err := runAdd([]string{"queue/redis"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	gomod := readFile(t, gomodPath)

	for _, fragment := range []string{
		"require github.com/zenta-dev/zever/core/queue v1.9.9",
		"require github.com/zenta-dev/zever/adapters/queue/redis v1.9.9",
	} {
		if !strings.Contains(gomod, fragment) {
			t.Fatalf("go.mod lacks %q (must match the pinned root version):\n%s", fragment, gomod)
		}
	}
}

func TestRunAddErrors(t *testing.T) {
	t.Run("arity", func(t *testing.T) {
		withWorkingDir(t, t.TempDir())

		for _, args := range [][]string{nil, {}, {"a", "b"}} {
			if err := runAdd(args); err == nil {
				t.Fatalf("runAdd(%v) succeeded, want an arity error", args)
			}
		}
	})

	t.Run("bad battery", func(t *testing.T) {
		withWorkingDir(t, t.TempDir())

		if err := runAdd([]string{"kafka"}); err == nil {
			t.Fatal("runAdd(kafka) succeeded, want an unknown-battery error")
		}
	})

	t.Run("no go.mod", func(t *testing.T) {
		withWorkingDir(t, t.TempDir())

		if err := runAdd([]string{"cache"}); err == nil {
			t.Fatal("runAdd outside a module succeeded, want a no-go.mod error")
		}
	})

	t.Run("missing app.go", func(t *testing.T) {
		dir := t.TempDir()
		withWorkingDir(t, dir)

		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/shop\n"), 0o600); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}

		err := runAdd([]string{"cache"})
		if err == nil {
			t.Fatal("runAdd without internal/app/app.go succeeded, want an error")
		}

		if !strings.Contains(err.Error(), "app.go") {
			t.Fatalf("error does not point at app.go: %v", err)
		}
	})
}

func TestGoModRequireVersion(t *testing.T) {
	t.Parallel()

	single := "module m\n\nrequire github.com/zenta-dev/zever v1.2.3\nrequire github.com/zenta-dev/zever/core/cache v1.2.3\n"
	if got := goModRequireVersion(single, frameworkModulePath); got != "v1.2.3" {
		t.Fatalf("single-line version = %q, want v1.2.3", got)
	}

	block := "module m\n\nrequire (\n\tgithub.com/zenta-dev/zever v4.5.6\n)\n"
	if got := goModRequireVersion(block, frameworkModulePath); got != "v4.5.6" {
		t.Fatalf("block version = %q, want v4.5.6", got)
	}

	if got := goModRequireVersion(single, "example.com/other"); got != "" {
		t.Fatalf("missing require version = %q, want empty", got)
	}
}
