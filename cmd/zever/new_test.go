package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/zenta-dev/zever/config"
)

// Shared fixture helpers (withWorkingDir, readFile, readGenerated,
// validateGoSyntax, writeSchemaFile) and the golden workflow (updateGolden,
// assertGolden) are owned by the generate wave (generate_test.go and
// friends); these tests reuse them without redefining.

// runGoInDir runs `go <args...>` with dir as the working directory, failing
// the test with combined output on error. Used to prove a scaffold really
// builds, not just that it looks plausible.
func runGoInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("no go toolchain on PATH: %v", err)
	}

	cmd := exec.CommandContext(t.Context(), goBin, args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// repoRootAbs returns this repository's own root, the framework checkout
// every test below replaces against (there is no tagged release to depend on
// any other way).
func repoRootAbs(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}

	return root
}

// --- runNew tests ---

// TestRunNewScaffoldsAndBuilds is the strongest possible proof the scaffold
// is genuinely buildable, not just syntactically plausible: it runs the real
// `go mod tidy` and `go build ./...` against the freshly scaffolded project.
func TestRunNewScaffoldsAndBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("go mod tidy + go build is slow")
	}

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := runNew([]string{"acme", "--framework-path", repoRoot}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "acme")

	// Every file lands at exactly the location ProjectConfig's defaults
	// assume, so zero zever.yaml/.json overrides are needed.
	for _, rel := range []string{
		"go.mod",
		filepath.Join(defaultSchemaDir, "app.zen"),
		filepath.Join(defaultServerEntry, "main.go"),
		filepath.Join(defaultWorkerEntry, "main.go"),
		filepath.Join(defaultSeedEntry, "main.go"),
		filepath.Join("internal", "app", "app.go"),
		".gitignore",
		"README.md",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}

	gomod := readFile(t, filepath.Join(out, "go.mod"))

	for _, fragment := range []string{
		"module acme",
		"replace github.com/zenta-dev/zever => ",
		"require github.com/zenta-dev/zever ",
	} {
		if !strings.Contains(gomod, fragment) {
			t.Fatalf("go.mod lacks %q:\n%s", fragment, gomod)
		}
	}

	schema := readFile(t, filepath.Join(out, defaultSchemaDir, "app.zen"))
	if !strings.Contains(schema, "entity User {") {
		t.Fatalf("scaffolded schema lacks a starter entity:\n%s", schema)
	}

	runGoInDir(t, out, "mod", "tidy")
	runGoInDir(t, out, "build", "./...")
}

// TestRunNewRefusesNonEmptyDirWithoutForce covers the collision-protection
// convention shared with the generate scaffolds: no silent clobbering of an
// existing directory's contents.
func TestRunNewRefusesNonEmptyDirWithoutForce(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := os.MkdirAll("beta", 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join("beta", "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := runNew([]string{"beta", "--framework-path", repoRoot})
	if err == nil {
		t.Fatalf("expected an error scaffolding into a non-empty directory")
	}

	if !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("error %q does not mention the directory being non-empty", err)
	}

	if err := runNew([]string{"beta", "--framework-path", repoRoot, "--force"}); err != nil {
		t.Fatalf("runNew --force: %v", err)
	}

	if _, err := os.Stat(filepath.Join("beta", "go.mod")); err != nil {
		t.Fatalf("expected go.mod to be written with --force: %v", err)
	}

	if _, err := os.Stat(filepath.Join("beta", "keep.txt")); err != nil {
		t.Fatalf("--force should not remove pre-existing files it does not own: %v", err)
	}
}

// TestRunNewAutoDetectsFrameworkCheckout proves --framework-path can be
// omitted when the command runs from inside a clone of zever itself: it
// walks up from the working directory looking for a go.mod declaring
// "module github.com/zenta-dev/zever", exactly what this repository's own
// go.mod declares.
func TestRunNewAutoDetectsFrameworkCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("scaffolding under the module root is slow to clean up in short mode")
	}

	repoRoot := repoRootAbs(t)

	sandbox, err := os.MkdirTemp(repoRoot, "zever-new-test-") //nolint:usetesting // must live under the module root for replace-directive detection
	if err != nil {
		t.Fatalf("mkdtemp under %s: %v", repoRoot, err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(sandbox) })

	withWorkingDir(t, sandbox)

	if err := runNew([]string{"gamma"}); err != nil {
		t.Fatalf("runNew (auto-detect): %v", err)
	}

	gomod := readFile(t, filepath.Join(sandbox, "gamma", "go.mod"))
	if !strings.Contains(gomod, "replace github.com/zenta-dev/zever => ") {
		t.Fatalf("expected an auto-detected local replace directive:\n%s", gomod)
	}
}

// TestRunNewNoFrameworkCheckoutFound covers the zero-flag onboarding path:
// outside any zever clone, with neither --framework-path nor
// --framework-version given, `zever new <name>` must still succeed by
// defaulting the local replace directive to the working directory, rather
// than blocking scaffolding on a checkout it can't find.
func TestRunNewNoFrameworkCheckoutFound(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := runNew([]string{"delta"}); err != nil {
		t.Fatalf("expected zever new to default to cwd and succeed, got: %v", err)
	}

	gomod := readFile(t, filepath.Join(workDir, "delta", "go.mod"))
	if !strings.Contains(gomod, "replace github.com/zenta-dev/zever => ") {
		t.Fatalf("expected a cwd-defaulted local replace directive:\n%s", gomod)
	}
}

// TestRunNewMutuallyExclusiveFrameworkFlags: --framework-path picks a local
// replace directive, --framework-version picks a real dependency version;
// giving both is a contradiction the command should reject up front.
func TestRunNewMutuallyExclusiveFrameworkFlags(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	err := runNew([]string{"eps", "--framework-path", ".", "--framework-version", "v1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected a mutually-exclusive-flags error, got %v", err)
	}
}

// TestRunNewInvalidName rejects an app name that would produce a broken
// default directory or module path.
func TestRunNewInvalidName(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := runNew([]string{"bad/name"}); err == nil {
		t.Fatalf("expected an error for an invalid app name")
	}
}

// TestRunNewRequiresExactlyOneName pins the arity: zero or two positionals
// print usage and fail, with no prompting fallback (non-interactive only).
func TestRunNewRequiresExactlyOneName(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	for _, args := range [][]string{nil, {}, {"a", "b"}} {
		if err := runNew(args); err == nil {
			t.Fatalf("expected an error for args %v", args)
		}
	}
}

// TestRunNewFrameworkVersionSkipsReplace covers the "opt into a real
// version" path: no replace directive, a plain require of the given version,
// and the default module path is the bare app name (there is no existing
// go.mod to derive one from, unlike zever extract).
func TestRunNewFrameworkVersionSkipsReplace(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := runNew([]string{"zeta", "--framework-version", "v1.2.3"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	gomod := readFile(t, filepath.Join(workDir, "zeta", "go.mod"))

	if strings.Contains(gomod, "replace ") {
		t.Fatalf("did not expect a replace directive:\n%s", gomod)
	}

	for _, fragment := range []string{"module zeta", "require github.com/zenta-dev/zever v1.2.3"} {
		if !strings.Contains(gomod, fragment) {
			t.Fatalf("go.mod lacks %q:\n%s", fragment, gomod)
		}
	}
}

// TestRunNewModuleAndDirOverrides proves --module and --dir are honoured
// independently of the app name.
func TestRunNewModuleAndDirOverrides(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	err := runNew([]string{
		"eta",
		"--module", "example.com/eta",
		"--dir", "custom-out",
		"--framework-path", repoRoot,
	})
	if err != nil {
		t.Fatalf("runNew: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "eta")); !os.IsNotExist(err) {
		t.Fatalf("expected no directory at the default ./<name> location when --dir overrides it")
	}

	gomod := readFile(t, filepath.Join(workDir, "custom-out", "go.mod"))
	if !strings.Contains(gomod, "module example.com/eta") {
		t.Fatalf("expected the overridden module path:\n%s", gomod)
	}
}

// TestRunNewExistingProjectSkipsGoMod proves scaffolding inside an existing
// Go module derives the module path, preserves the toolchain version, and
// writes no go.mod of its own.
func TestRunNewExistingProjectSkipsGoMod(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := os.WriteFile("go.mod", []byte("module example.com/existing\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	if err := runNew([]string{"sub"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "sub")

	if _, err := os.Stat(filepath.Join(out, "go.mod")); !os.IsNotExist(err) {
		t.Fatalf("expected no go.mod inside an existing project scaffold")
	}

	server := readGenerated(t, filepath.Join(out, defaultServerEntry, "main.go"))
	if !strings.Contains(server, `"example.com/existing/internal/app"`) {
		t.Fatalf("server main.go does not import the existing module's app package:\n%s", server)
	}

	if _, err := os.Stat(filepath.Join(out, "zever.yaml")); err != nil {
		t.Fatalf("expected zever.yaml to be written: %v", err)
	}
}

// TestRunNewScaffoldsMinimalZeverYaml proves the default (flag-driven)
// `zever new` writes exactly coreBatteries into zever.yaml -- not the full
// ~32-service config.Default() map -- and that the emitted file loads
// through the ported config.Load (the strict-merge coexistence proof).
func TestRunNewScaffoldsMinimalZeverYaml(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := runNew([]string{"zeta", "--framework-path", repoRoot}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	yamlContent := readFile(t, filepath.Join(workDir, "zeta", "zever.yaml"))

	var doc map[string]any

	if err := yaml.Unmarshal([]byte(yamlContent), &doc); err != nil {
		t.Fatalf("parse zever.yaml: %v", err)
	}

	if len(doc) != len(coreBatteries) {
		t.Fatalf("zever.yaml lists %d services, want exactly the %d core ones: %v", len(doc), len(coreBatteries), doc)
	}

	for _, b := range coreBatteries {
		if _, ok := doc[b]; !ok {
			t.Errorf("zever.yaml missing core service %q", b)
		}
	}

	// A service outside the core set (e.g. "search") must not appear by
	// default -- proves the scaffold is minimal, not "all of them".
	if _, ok := doc["search"]; ok {
		t.Fatalf("zever.yaml unexpectedly includes non-core service \"search\": %v", doc)
	}

	// The emitted file must load through the strict ported config: this is
	// the mapping proof (batteries-only YAML, no "project" table).
	t.Chdir(filepath.Join(workDir, "zeta"))

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load of generated zever.yaml: %v", err)
	}

	if cfg.DB.Adapter != "sqlite" {
		t.Fatalf("loaded db adapter = %q, want sqlite", cfg.DB.Adapter)
	}
}

// TestRunNewAppGoImportsMatchZeverYamlBatteries proves app.go's blank
// imports are exactly the services zever.yaml lists -- the two files can
// never drift, because both are rendered from the same NewConfig.Batteries.
func TestRunNewAppGoImportsMatchZeverYamlBatteries(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := runNew([]string{"theta", "--framework-path", repoRoot}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	app := readFile(t, filepath.Join(workDir, "theta", "internal", "app", "app.go"))

	for _, b := range coreBatteries {
		marker := "\"github.com/zenta-dev/zever/" + b + "/"
		if !strings.Contains(app, marker) {
			t.Errorf("app.go missing a blank import for core service %q:\n%s", b, app)
		}
	}

	// cache is not core and nothing generated resolves c.Cache(), so it must
	// not be imported by default.
	if strings.Contains(app, "cache/memory") {
		t.Fatalf("app.go unexpectedly imports cache/memory with no cache service selected:\n%s", app)
	}
}

// --- pure-unit tests ---

// TestIsValidAppName pins the traversal confinement: separators, dots and
// blanks are rejected; only [A-Za-z0-9-_] passes.
func TestIsValidAppName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "simple", input: "myapp", want: true},
		{name: "dashes", input: "my-app", want: true},
		{name: "underscores", input: "my_app", want: true},
		{name: "digits", input: "app2", want: true},
		{name: "empty", input: "", want: false},
		{name: "slash", input: "bad/name", want: false},
		{name: "dotdot", input: "..", want: false},
		{name: "dot", input: ".", want: false},
		{name: "backslash", input: `a\b`, want: false},
		{name: "space", input: "a b", want: false},
		{name: "dot-in-name", input: "a.b", want: false},
		{name: "traversal", input: "../evil", want: false},
		{name: "absolute", input: "/evil", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidAppName(tt.input); got != tt.want {
				t.Fatalf("isValidAppName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestEnsureTargetDir pins the existing-dir refusal semantics: missing dirs
// are created, empty dirs pass, non-empty dirs refuse without --force.
func TestEnsureTargetDir(t *testing.T) {
	t.Run("missing creates", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "newdir")
		if err := ensureTargetDir("tag", dir, false); err != nil {
			t.Fatalf("ensureTargetDir: %v", err)
		}

		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected %q to be created: %v", dir, err)
		}
	})

	t.Run("empty passes", func(t *testing.T) {
		dir := t.TempDir()
		if err := ensureTargetDir("tag", dir, false); err != nil {
			t.Fatalf("ensureTargetDir on empty dir: %v", err)
		}
	})

	t.Run("non-empty refuses", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		err := ensureTargetDir("tag", dir, false)
		if err == nil || !strings.Contains(err.Error(), "not empty") {
			t.Fatalf("expected a not-empty refusal, got %v", err)
		}
	})

	t.Run("force allows non-empty", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		if err := ensureTargetDir("tag", dir, true); err != nil {
			t.Fatalf("ensureTargetDir --force: %v", err)
		}
	})
}

// TestMergeBatteries pins the sorted de-duplicated union.
func TestMergeBatteries(t *testing.T) {
	got := mergeBatteries([]string{"db", "auth"}, []string{"db", "cache"})

	want := []string{"auth", "cache", "db"}
	if len(got) != len(want) {
		t.Fatalf("mergeBatteries = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mergeBatteries = %v, want %v", got, want)
		}
	}
}

// TestBatteriesForAddsCacheOnOverride pins the cache-adapter implication:
// a real cache override pulls "cache" into the set even when unpicked.
func TestBatteriesForAddsCacheOnOverride(t *testing.T) {
	got := batteriesFor(nil, "redis")

	found := false

	for _, b := range got {
		if b == "cache" {
			found = true
		}
	}

	if !found {
		t.Fatalf("batteriesFor(nil, redis) = %v, want cache included", got)
	}

	plain := batteriesFor(nil, "")

	for _, b := range plain {
		if b == "cache" {
			t.Fatalf("batteriesFor(nil, \"\") = %v, want no cache", plain)
		}
	}
}

// TestNonCoreBatteryNamesExcludesCore pins the wizard offer list: every core
// service absent, sorted, and covering the rest of the service universe.
func TestNonCoreBatteryNamesExcludesCore(t *testing.T) {
	got := nonCoreBatteryNames()

	if !sort.StringsAreSorted(got) {
		t.Fatalf("nonCoreBatteryNames not sorted: %v", got)
	}

	for _, b := range coreBatteries {
		if sort.SearchStrings(got, b) < len(got) && got[sort.SearchStrings(got, b)] == b {
			t.Fatalf("nonCoreBatteryNames contains core service %q", b)
		}
	}

	if len(got)+len(coreBatteries) != len(allServiceAdapters) {
		t.Fatalf("non-core (%d) + core (%d) != all (%d)", len(got), len(coreBatteries), len(allServiceAdapters))
	}
}

// TestAllServiceAdaptersMatchDefault is the drift guard: every service the
// runtime knows must appear in allServiceAdapters with the live default
// adapter, and vice versa.
func TestAllServiceAdaptersMatchDefault(t *testing.T) {
	live := config.Default().RedactedServices()

	for name, want := range live {
		got, ok := allServiceAdapters[name]
		if !ok {
			t.Errorf("allServiceAdapters missing runtime service %q", name)
			continue
		}

		if got != want.Adapter {
			t.Errorf("allServiceAdapters[%q] = %q, want live default %q", name, got, want.Adapter)
		}
	}

	for name := range allServiceAdapters {
		if _, ok := live[name]; !ok {
			t.Errorf("allServiceAdapters[%q] has no runtime service", name)
		}
	}
}

// TestQuickstartBackends pins the default sample and the override join.
func TestQuickstartBackends(t *testing.T) {
	if got := quickstartBackends(NewConfig{}); got != "zenorm,proto,atlas" {
		t.Fatalf("default quickstartBackends = %q", got)
	}

	cfg := NewConfig{Backends: []string{"gogen", "openapi"}}
	if got := quickstartBackends(cfg); got != "gogen,openapi" {
		t.Fatalf("override quickstartBackends = %q", got)
	}
}

// TestRenderZeverYamlGolden pins the exact bytes of the emitted zever.yaml.
func TestRenderZeverYamlGolden(t *testing.T) {
	cfg := NewConfig{
		Batteries: []string{"auth", "db", "cache"},
		DBAdapter: "postgres",
	}

	data, err := renderZeverYaml(cfg)
	if err != nil {
		t.Fatalf("renderZeverYaml: %v", err)
	}

	assertGolden(t, "zever_yaml", data)
}

// TestRenderNewGoModGolden pins the exact bytes of a version-pinned go.mod
// (no checkout paths, so the output is deterministic).
func TestRenderNewGoModGolden(t *testing.T) {
	cfg := NewConfig{
		ModulePath:       "example.com/acme",
		GoVersion:        "1.24",
		FrameworkVersion: "v1.2.3",
	}

	data, err := renderNewGoMod("zever new", cfg)
	if err != nil {
		t.Fatalf("renderNewGoMod: %v", err)
	}

	assertGolden(t, "new_gomod", data)
}

// TestRenderNewReadmeMentionsZever pins the rename: no "zengo" may survive
// in user-facing scaffold text.
func TestRenderNewReadmeMentionsZever(t *testing.T) {
	readme := renderNewReadme("acme", "zenorm,proto,atlas")

	if strings.Contains(readme, "zengo") {
		t.Fatalf("README still mentions zengo:\n%s", readme)
	}

	for _, fragment := range []string{"# acme", "zever compile --backend=zenorm,proto,atlas", "zever serve"} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("README lacks %q:\n%s", fragment, readme)
		}
	}
}

// TestRenderNewSchemaStarterEntity pins the starter schema shape.
func TestRenderNewSchemaStarterEntity(t *testing.T) {
	schema := renderNewSchema("acme")

	for _, fragment := range []string{"entity User {", "email: string @unique", "zever compile"} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema lacks %q:\n%s", fragment, schema)
		}
	}

	if strings.Contains(schema, "zengo") {
		t.Fatalf("schema still mentions zengo:\n%s", schema)
	}
}

// TestDefaultServiceAdapterUnknown pins the fallback: unknown service names
// resolve to "" rather than a invented adapter.
func TestDefaultServiceAdapterUnknown(t *testing.T) {
	if got := defaultServiceAdapter("does-not-exist"); got != "" {
		t.Fatalf("defaultServiceAdapter(unknown) = %q, want empty", got)
	}
}

// TestWriteNewProjectRefusesClobber pins the single-writer rule at the core
// layer: writeNewProject never overwrites an existing file without Force,
// so screens calling it cannot silently destroy hand-written code.
func TestWriteNewProjectRefusesClobber(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	out := filepath.Join(workDir, "acme")
	cfg := NewConfig{
		Name:             "acme",
		OutDir:           out,
		ModulePath:       "example.com/acme",
		GoVersion:        "1.24",
		FrameworkVersion: "v1.2.3",
		Batteries:        coreBatteries,
	}

	if err := os.MkdirAll(filepath.Join(out, "schema"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	const handwritten = "// hand-written\n"
	if err := os.WriteFile(filepath.Join(out, "schema", "app.zen"), []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := writeNewProject("test", cfg); err == nil {
		t.Fatal("expected a clobber refusal without Force")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error %q does not mention existing files", err)
	}

	cfg.Force = true

	if _, err := writeNewProject("test", cfg); err != nil {
		t.Fatalf("writeNewProject --force: %v", err)
	}

	if got := readFile(t, filepath.Join(out, "schema", "app.zen")); got == handwritten {
		t.Fatal("--force did not overwrite the stub schema")
	}
}
