package main

import (
	"context"
	"encoding/base64"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

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
		t.Skip("no go toolchain on PATH")
	}

	// The scaffold resolves dependencies over the network, so a transient
	// proxy/DNS/TLS failure is retried; any other failure is a real build
	// break and fails immediately.
	const attempts = 3

	var out []byte

	for range attempts {
		ctx, cancel := context.WithTimeout(t.Context(), goCommandTimeout)
		cmd := exec.CommandContext(ctx, goBin, args...)
		cmd.Dir = dir

		out, err = cmd.CombinedOutput()

		cancel()

		if err == nil || !isTransientGoFailure(string(out)) {
			break
		}

		t.Logf("go %s: transient network failure, retrying: %v", strings.Join(args, " "), err)
	}

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

// serveStubGoMod is the minimal module file for the serve API stub used
// until the serve stream merges: it carries the exact spec'd surface
// (Module, Main, Run, Worker) so scaffold builds prove API compatibility.
const serveStubGoMod = `module github.com/zenta-dev/zever/serve

go 1.27.2

require (
	github.com/zenta-dev/zever/container v0.0.0
	github.com/zenta-dev/zever/core/router v0.0.0
)
`

// serveStubSource implements the spec'd serve surface against the real
// container: Main/Run build the container, mount every module on the
// resolved router and return; Worker builds the container and returns.
const serveStubSource = `package serve

import (
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/router"
)

// Module is one mountable unit of a generated server.
type Module interface {
	Name() string
	Mount(r router.Router, c *container.Container) error
}

// Option customizes one Main/Run/Worker call.
type Option func(*options)

type options struct{}

// Main runs a generated server to completion.
func Main(providers []container.Provider, modules func(*container.Container) []Module, opts ...Option) error {
	c := container.New(nil, providers...)

	r, err := c.Router()
	if err != nil {
		return err
	}

	for _, m := range modules(c) {
		if err := m.Mount(r, c); err != nil {
			return err
		}
	}

	return nil
}

// Run boots a generated server to completion.
func Run(providers []container.Provider, modules func(*container.Container) []Module, opts ...Option) error {
	return Main(providers, modules, opts...)
}

// Worker runs a queue/scheduler worker to completion.
func Worker(providers []container.Provider, opts ...Option) error {
	_ = container.New(nil, providers...)

	return nil
}
`

// writeServeStub materialises the serve API stub into dir, for scaffolds
// whose checkout predates the merged serve module.
func writeServeStub(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir serve stub: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(serveStubGoMod), 0o600); err != nil {
		t.Fatalf("write serve stub go.mod: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "serve.go"), []byte(serveStubSource), 0o600); err != nil {
		t.Fatalf("write serve stub source: %v", err)
	}
}

// pointServeAtStub rewrites the scaffold go.mod's serve replace to the stub
// dir, so offline tidy/build resolve the spec'd API without the merged
// serve module. It does nothing when the checkout already ships serve.
func pointServeAtStub(t *testing.T, repoRoot, scaffoldDir string) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(repoRoot, "serve", "go.mod")); err == nil {
		return
	}

	stubDir := filepath.Join(scaffoldDir, "servestub")
	writeServeStub(t, stubDir)

	gomodPath := filepath.Join(scaffoldDir, "go.mod")

	data, err := os.ReadFile(gomodPath) // test fixture path
	if err != nil {
		t.Fatalf("read scaffold go.mod: %v", err)
	}

	var out []string

	replaced := false

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "replace github.com/zenta-dev/zever/serve ") {
			rel, err := filepath.Rel(scaffoldDir, stubDir)
			if err != nil {
				t.Fatalf("rel stub: %v", err)
			}

			rel = filepath.ToSlash(rel)
			if !strings.HasPrefix(rel, ".") && !filepath.IsAbs(rel) {
				rel = "./" + rel
			}

			out = append(out, "replace github.com/zenta-dev/zever/serve => "+rel)
			replaced = true

			continue
		}

		out = append(out, line)
	}

	if !replaced {
		t.Fatal("scaffold go.mod has no serve replace to repoint")
	}

	if err := os.WriteFile(gomodPath, []byte(strings.Join(out, "\n")), 0o600); err != nil { //nolint:gosec // test temp dir
		t.Fatalf("write scaffold go.mod: %v", err)
	}
}

// --- runNew tests ---

// TestRunNewScaffoldsAndBuilds is the strongest possible proof the minimal
// scaffold is genuinely buildable, not just syntactically plausible: it runs
// the real `go mod tidy` and `go build ./...` (plus vet and test) against
// the freshly scaffolded project.
func TestRunNewScaffoldsAndBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("go mod tidy + go build is slow")
	}

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"acme"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "acme")

	// The minimal tree: go.mod, schema, server main, typed wiring, the
	// sample module, env files, gitignore, README and the sqlite data dir.
	// Nothing heavier.
	for _, rel := range []string{
		"go.mod",
		filepath.Join(defaultSchemaDir, "app.zen"),
		filepath.Join(defaultServerEntry, "main.go"),
		filepath.Join("internal", "app", "wiring.go"),
		filepath.Join("internal", "service", "hello", "service.go"),
		".env",
		".env.example",
		".gitignore",
		"README.md",
		filepath.Join("data", ".gitkeep"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}

	// Heavier files are opt-in recipes now, never defaults.
	for _, rel := range []string{
		filepath.Join(defaultWorkerEntry, "main.go"),
		filepath.Join(defaultSeedEntry, "main.go"),
		"Dockerfile",
		"compose.yaml",
		"Makefile",
		filepath.Join(".github", "workflows", "ci.yml"),
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected no %s in the minimal scaffold", rel)
		}
	}

	gomod := readFile(t, filepath.Join(out, "go.mod"))

	for _, fragment := range []string{
		"module acme",
		"replace github.com/zenta-dev/zever/container => ",
		"replace github.com/zenta-dev/zever/serve => ",
		"require github.com/zenta-dev/zever/container ",
		"require github.com/zenta-dev/zever/config ",
		"require github.com/zenta-dev/zever/serve ",
		"require github.com/zenta-dev/zever/orm ",
		"require github.com/zenta-dev/zever/core/log ",
		"require github.com/zenta-dev/zever/core/router ",
		"require github.com/zenta-dev/zever/core/db ",
		"require github.com/zenta-dev/zever/core/observability ",
		"require github.com/zenta-dev/zever/core/ratelimit ",
		"require github.com/zenta-dev/zever/adapters/log/slog ",
		"require github.com/zenta-dev/zever/adapters/router/stdhttp ",
		"require github.com/zenta-dev/zever/adapters/db/sqlite ",
		"require github.com/zenta-dev/zever/adapters/observability/stdout ",
		"require github.com/zenta-dev/zever/adapters/ratelimit/memory ",
	} {
		if !strings.Contains(gomod, fragment) {
			t.Fatalf("go.mod lacks %q:\n%s", fragment, gomod)
		}
	}

	// Dropped with the worker entrypoint and friends: no job/authz/rag
	// requires in the minimal tree.
	for _, banned := range []string{
		"require github.com/zenta-dev/zever/core/job ",
		"require github.com/zenta-dev/zever/core/agent ",
		"require github.com/zenta-dev/zever/core/rag ",
		"require github.com/zenta-dev/zever/core/authz ",
		"require github.com/zenta-dev/zever/core/middleware ",
	} {
		if strings.Contains(gomod, banned) {
			t.Fatalf("go.mod must not contain %q:\n%s", banned, gomod)
		}
	}

	schema := readFile(t, filepath.Join(out, defaultSchemaDir, "app.zen"))
	if !strings.Contains(schema, "entity User {") {
		t.Fatalf("scaffolded schema lacks a starter entity:\n%s", schema)
	}

	wiring := readFile(t, filepath.Join(out, "internal", "app", "wiring.go"))
	for _, fragment := range []string{
		"func Providers() []container.Provider",
		"func Modules() func(*container.Container) []serve.Module",
		"container.Provide(DB,",
		"container.Provide(Log,",
		"container.Provide(Router,",
		"container.Provide(Observability,",
		"container.Provide(RateLimit,",
		"var DB = db.DefaultRef()",
		"var Log = log.DefaultRef()",
		"var Router = router.DefaultRef()",
		"var Observability = observability.DefaultRef()",
		"var RateLimit = ratelimit.DefaultRef()",
	} {
		if !strings.Contains(wiring, fragment) {
			t.Fatalf("wiring.go lacks %q:\n%s", fragment, wiring)
		}
	}

	mainSrc := readFile(t, filepath.Join(out, defaultServerEntry, "main.go"))
	if !strings.Contains(mainSrc, "serve.Main(app.Providers(), app.Modules(), opts...)") {
		t.Fatalf("server main.go does not call serve.Main with Providers and Modules:\n%s", mainSrc)
	}

	pointServeAtStub(t, repoRoot, out)

	runGoInDir(t, out, "mod", "tidy")
	runGoInDir(t, out, "build", "./...")
	// The scaffold must also vet and test clean, not just compile: this is
	// the CI gate that a fresh `zever new` project stays green.
	runGoInDir(t, out, "vet", "./...")
	runGoInDir(t, out, "test", "./...")
}

// TestScaffoldGoModMinimalRequires is the E3 audit: after `go mod tidy`,
// the scaffold's go.mod must require no adapter module that wiring.go does
// not import. Transitive adapter deps (stdhttp's default log/noop) resolve
// through the wired adapters' own modules, never as direct requires.
func TestScaffoldDepsMinimal(t *testing.T) {
	if testing.Short() {
		t.Skip("go mod tidy + go list is slow")
	}

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"auditapp", "--with", "cache"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "auditapp")
	pointServeAtStub(t, repoRoot, out)
	runGoInDir(t, out, "mod", "tidy")

	wiring := readFile(t, filepath.Join(out, "internal", "app", "wiring.go"))
	wf, err := parseWiringFile(wiring)
	if err != nil {
		t.Fatalf("parse scaffold wiring.go: %v", err)
	}

	imported := map[string]bool{}
	for _, imp := range wf.imports {
		if strings.HasPrefix(imp.path, frameworkModulePath+"/adapters/") {
			imported[imp.path] = true
		}
	}

	gomod := readFile(t, filepath.Join(out, "go.mod"))

	for _, line := range strings.Split(gomod, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "// indirect") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}

		if fields[0] == "require" {
			fields = fields[1:]
		}

		if len(fields) < 2 || !strings.HasPrefix(fields[0], frameworkModulePath+"/adapters/") {
			continue
		}

		if !imported[fields[0]] {
			t.Errorf("tidied go.mod directly requires adapter module %q not imported by wiring.go", fields[0])
		}
	}

	want := map[string]bool{
		frameworkModulePath + "/adapters/db/sqlite":            true,
		frameworkModulePath + "/adapters/log/slog":             true,
		frameworkModulePath + "/adapters/router/stdhttp":       true,
		frameworkModulePath + "/adapters/observability/stdout": true,
		frameworkModulePath + "/adapters/ratelimit/memory":     true,
		frameworkModulePath + "/adapters/cache/memory":         true,
	}

	for path := range imported {
		if !want[path] {
			t.Errorf("unexpected wiring import %q for --with cache scaffold", path)
		}

		if !strings.Contains(gomod, "\t"+path+" ") && !strings.Contains(gomod, "require "+path+" ") {
			t.Errorf("tidied go.mod lacks require for wired adapter %q", path)
		}
	}
}

// TestRunNewPrintsExactlyThreeNextSteps pins the summary contract: exactly
// three numbered next steps, no more.
func TestRunNewPrintsExactlyThreeNextSteps(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	out := captureStdout(t, func() {
		if err := runNew([]string{"stepapp", "--no-tidy"}); err != nil {
			t.Fatalf("runNew: %v", err)
		}
	})

	for _, want := range []string{"1.", "2.", "3."} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks step %q:\n%s", want, out)
		}
	}

	if strings.Contains(out, "4.") {
		t.Errorf("summary must print exactly 3 next steps, found a fourth:\n%s", out)
	}
}

// TestRunNewWritesEnvFiles proves `zever new` writes .env with a random dev
// secret and .env.example generated from the wired Options structs.
func TestRunNewWritesEnvFiles(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"envapp", "--no-tidy"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "envapp")

	env := readFile(t, filepath.Join(out, ".env"))
	secret := ""
	for _, line := range strings.Split(env, "\n") {
		if v, ok := strings.CutPrefix(line, "AUTH_JWT_SECRET="); ok {
			secret = v
		}
	}

	if len(secret) < 32 {
		t.Fatalf("AUTH_JWT_SECRET must carry at least 32 random bytes, got %q", secret)
	}

	if _, err := base64.StdEncoding.DecodeString(secret); err != nil {
		t.Fatalf("AUTH_JWT_SECRET must be base64, got %q: %v", secret, err)
	}

	cryptoKey := ""
	for _, line := range strings.Split(env, "\n") {
		if v, ok := strings.CutPrefix(line, "CRYPTO_KEY="); ok {
			cryptoKey = v
		}
	}

	key, err := base64.StdEncoding.DecodeString(cryptoKey)
	if err != nil || len(key) != 32 {
		t.Fatalf("CRYPTO_KEY must be 32 random bytes as base64, got %q: %v", cryptoKey, err)
	}

	if strings.Contains(env, "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=") {
		t.Fatal(".env must not contain the deterministic dev crypto key")
	}

	example := readFile(t, filepath.Join(out, ".env.example"))
	for _, fragment := range []string{
		"# --- db (adapter sqlite) ---",
		"DB_ADAPTER=sqlite",
		"# --- log (adapter slog) ---",
		"LOG_ADAPTER=slog",
		"# --- router (adapter stdhttp) ---",
		"ROUTER_ADAPTER=stdhttp",
		"DB_MAX_CONNS=",
	} {
		if !strings.Contains(example, fragment) {
			t.Errorf(".env.example lacks %q:\n%s", fragment, example)
		}
	}
}

// TestRunNewRefusesNonEmptyDirWithoutForce covers the collision-protection
// convention shared with the generate scaffolds: no silent clobbering of an
// existing directory's contents.
func TestRunNewRefusesNonEmptyDirWithoutForce(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	if err := os.MkdirAll("beta", 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join("beta", "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := runNew([]string{"beta"})
	if err == nil {
		t.Fatalf("expected an error scaffolding into a non-empty directory")
	}

	if !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("error %q does not mention the directory being non-empty", err)
	}

	if err := runNew([]string{"beta", "--force"}); err != nil {
		t.Fatalf("runNew --force: %v", err)
	}

	if _, err := os.Stat(filepath.Join("beta", "go.mod")); err != nil {
		t.Fatalf("expected go.mod to be written with --force: %v", err)
	}

	if _, err := os.Stat(filepath.Join("beta", "keep.txt")); err != nil {
		t.Fatalf("--force should not remove pre-existing files it does not own: %v", err)
	}
}

// TestRunNewAutoDetectsFrameworkCheckout proves the scaffold correctly
// detects it is running from inside a clone of zever itself: it walks up
// from the working directory looking for a go.mod declaring "module
// github.com/zenta-dev/zever", exactly what this repository's own go.mod
// declares. There is no flag for this -- it only ever applies to framework
// development.
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

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"gamma"}); err != nil {
		t.Fatalf("runNew (auto-detect): %v", err)
	}

	gomod := readFile(t, filepath.Join(sandbox, "gamma", "go.mod"))
	if !strings.Contains(gomod, "replace github.com/zenta-dev/zever/container => ") {
		t.Fatalf("expected an auto-detected local replace directive:\n%s", gomod)
	}
}

// TestRunNewNoFrameworkCheckoutFound covers the zero-flag onboarding path:
// outside any zever clone, with no --framework-version given, `zever new
// <name>` must still succeed by defaulting to the published module version,
// rather than blocking scaffolding on a checkout it can't find.
func TestRunNewNoFrameworkCheckoutFound(t *testing.T) {
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"delta"}); err != nil {
		t.Fatalf("expected zever new to default to cwd and succeed, got: %v", err)
	}

	gomod := readFile(t, filepath.Join(workDir, "delta", "go.mod"))
	if strings.Contains(gomod, "replace github.com/zenta-dev/zever/container => ") {
		t.Fatalf("did not expect replace for upstream default:\n%s", gomod)
	}
	if !strings.Contains(gomod, "require github.com/zenta-dev/zever/container v"+buildVersion) {
		t.Fatalf("want upstream require:\n%s", gomod)
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

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"zeta", "--framework-version", "v1.2.3"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	gomod := readFile(t, filepath.Join(workDir, "zeta", "go.mod"))

	if strings.Contains(gomod, "replace ") {
		t.Fatalf("did not expect a replace directive:\n%s", gomod)
	}

	for _, fragment := range []string{"module zeta", "require github.com/zenta-dev/zever/container v1.2.3"} {
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
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	err := runNew([]string{
		"eta",
		"--module", "example.com/eta",
		"--dir", "custom-out",
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

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"sub"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "sub")

	if _, err := os.Stat(filepath.Join(out, "go.mod")); !os.IsNotExist(err) {
		t.Fatalf("expected no go.mod inside an existing project scaffold")
	}

	wiring := readGenerated(t, filepath.Join(out, "internal", "app", "wiring.go"))
	if !strings.Contains(wiring, `"example.com/existing/internal/service/hello"`) {
		t.Fatalf("wiring.go does not import the existing module's hello package:\n%s", wiring)
	}
}

// TestRunNewScaffoldsMinimalWiring proves the default (flag-driven)
// `zever new` wires exactly the minimal stack (db, log, router) into
// internal/app/wiring.go -- not the full ~36-service config.Default() map.
func TestRunNewScaffoldsMinimalWiring(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"zeta"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	// Config is env-only: no config files are scaffolded.
	for _, name := range []string{"zever.yaml", "zever.schema.json"} {
		if _, err := os.Stat(filepath.Join(workDir, "zeta", name)); !os.IsNotExist(err) {
			t.Fatalf("%s must not be scaffolded (err = %v)", name, err)
		}
	}

	wiring := readFile(t, filepath.Join(workDir, "zeta", "internal", "app", "wiring.go"))

	for _, b := range []string{"db", "log", "observability", "ratelimit", "router"} {
		if !strings.Contains(wiring, "/"+b+"/") {
			t.Errorf("wiring.go missing minimal service %q", b)
		}
	}

	// A service outside the minimal set (e.g. "search") must not appear by
	// default -- proves the scaffold is minimal, not "all of them".
	if strings.Contains(wiring, "/search/") {
		t.Fatalf("wiring.go unexpectedly wires non-core service \"search\"")
	}
}

// TestRunNewWritesNoConfigFiles proves `zever new` scaffolds no config
// files at all: service configuration is env-only, so neither zever.yaml
// nor zever.schema.json is emitted.
func TestRunNewWritesNoConfigFiles(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"zeta"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "zeta")

	for _, name := range []string{"zever.yaml", "zever.schema.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
			t.Fatalf("%s must not be scaffolded (err = %v)", name, err)
		}
	}
}

// TestRunNewWiringImportsMatchBatteries proves wiring.go's adapter imports
// are exactly the selected batteries, because both are rendered from the
// same NewConfig.Batteries.
func TestRunNewWiringImportsMatchBatteries(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"theta"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	wiring := readFile(t, filepath.Join(workDir, "theta", "internal", "app", "wiring.go"))

	for _, sel := range []batterySelection{
		{Battery: "db", Adapter: "sqlite"},
		{Battery: "log", Adapter: "slog"},
		{Battery: "observability", Adapter: "stdout"},
		{Battery: "ratelimit", Adapter: "memory"},
		{Battery: "router", Adapter: "stdhttp"},
	} {
		alias, path := adapterBinding(sel)

		for _, marker := range []string{`"` + path + `"`, alias + ` "` + path + `"`, "container.Provide("} {
			if !strings.Contains(wiring, marker) {
				t.Errorf("wiring.go missing %q for service %q:\n%s", marker, sel.Battery, wiring)
			}
		}
	}

	// cache is not in the minimal set and nothing generated resolves
	// c.Cache(), so it must not be imported by default.
	if strings.Contains(wiring, "adapters/cache/memory") {
		t.Fatalf("wiring.go unexpectedly imports adapters/cache/memory with no cache service selected:\n%s", wiring)
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

// TestDefaultBatteriesAreMinimalStack pins the default set: db, log and
// router, nothing more.
func TestDefaultBatteriesAreMinimalStack(t *testing.T) {
	t.Parallel()

	got := batteriesFor(nil, "")

	want := []string{"db", "log", "observability", "ratelimit", "router"}
	if len(got) != len(want) {
		t.Fatalf("batteriesFor(nil, \"\") = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("batteriesFor(nil, \"\") = %v, want %v", got, want)
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

// TestNestedModuleDirsMatchCheckout pins nestedModuleDirs against the
// checkout: every adapters/<battery>/<adapter> go.mod must be listed, and
// every listed directory must contain a go.mod. Without this, scaffolded
// projects silently lose module-graph replaces for newly extracted
// adapters. tools/ stays excluded (separate release line, never a scaffold
// dependency).
func TestNestedModuleDirsMatchCheckout(t *testing.T) {
	t.Parallel()

	root := repoRootAbs(t)

	found := map[string]bool{}
	for _, dir := range nestedModuleDirs {
		if _, err := os.Stat(filepath.Join(root, dir, "go.mod")); err != nil {
			t.Errorf("nestedModuleDirs[%q] has no go.mod: %v", dir, err)
		}

		found[dir] = true
	}

	batteries, err := os.ReadDir(filepath.Join(root, "adapters"))
	if err != nil {
		t.Fatalf("read adapters: %v", err)
	}

	for _, battery := range batteries {
		if !battery.IsDir() || battery.Name() == "tools" || strings.HasPrefix(battery.Name(), ".") {
			continue
		}

		impls, ierr := os.ReadDir(filepath.Join(root, "adapters", battery.Name()))
		if ierr != nil {
			t.Fatalf("read adapters/%s: %v", battery.Name(), ierr)
		}

		for _, impl := range impls {
			if !impl.IsDir() {
				continue
			}

			dir := "adapters/" + battery.Name() + "/" + impl.Name()

			if _, serr := os.Stat(filepath.Join(root, dir, "go.mod")); serr != nil {
				continue
			}

			if !found[dir] {
				t.Errorf("nested module %q missing from nestedModuleDirs", dir)
			}
		}
	}

	// sharedModuleDirs gets the same drift guard: every shared go.mod must
	// be listed, and every listed directory must contain one.
	sharedFound := map[string]bool{}

	for _, dir := range sharedModuleDirs {
		if _, serr := os.Stat(filepath.Join(root, dir, "go.mod")); serr != nil {
			t.Errorf("sharedModuleDirs[%q] has no go.mod: %v", dir, serr)
		}

		sharedFound[dir] = true
	}

	shared, err := os.ReadDir(filepath.Join(root, "shared"))
	if err != nil {
		t.Fatalf("read shared: %v", err)
	}

	for _, entry := range shared {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		dir := "shared/" + entry.Name()

		if _, err := os.Stat(filepath.Join(root, dir, "go.mod")); err != nil {
			continue
		}

		if !sharedFound[dir] {
			t.Errorf("shared module %q missing from sharedModuleDirs", dir)
		}
	}
}

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

// TestScaffoldReplaceBlockCoversEveryFrameworkModule pins the local-checkout
// replace set of a scaffolded project, in four directions:
//
//  1. every framework module the checkout ships under the runtime roots
//     (adapters/<battery>/<adapter>, core/<battery>, shared/<helper>) is
//     replaced, so the project's module graph resolves every sibling module
//     against the checkout instead of the module proxy (which cannot serve a
//     version that is not published yet), and a module the framework adds
//     later is covered as soon as the checkout ships it;
//  2. every framework module the scaffold requires is replaced, so the
//     project's own imports never reach the proxy;
//  3. the set is closed under the checkout's intra-repo requires: a replaced
//     module that requires another framework module has that one replaced
//     too -- this is the direction that used to break (container requires
//     core/agent, serve requires core/authz);
//  4. no replace points at a module the checkout does not ship (that breaks
//     `go mod tidy` outright), and no replace points at a directory whose
//     go.mod declares another module.
//
// The expected set is enumerated from the checkout's directories, not from
// the code that renders the replaces, so a module the framework adds later
// fails this test until the scaffold covers it.
func TestScaffoldReplaceBlockCoversEveryFrameworkModule(t *testing.T) {
	t.Parallel()

	root := repoRootAbs(t)

	data, err := renderNewGoMod("zever new", NewConfig{
		ModulePath:       "example.com/acme",
		GoVersion:        "1.27",
		FrameworkVersion: "v0.0.0",
		FrameworkDir:     root,
		OutDir:           t.TempDir(),
	})
	if err != nil {
		t.Fatalf("renderNewGoMod: %v", err)
	}

	replaced, required := scaffoldGoModModules(t, string(data))

	shipped := checkoutModuleDirs(t, root)

	// 1. the whole runtime surface of the checkout is replaced.
	for path := range checkoutRuntimeModules(t, root) {
		if !replaced[path] {
			t.Errorf("framework module %q has no replace in the scaffolded go.mod", path)
		}
	}

	// 2. the modules the project itself requires are replaced.
	for path := range required {
		if !replaced[path] {
			t.Errorf("required framework module %q has no replace in the scaffolded go.mod", path)
		}
	}

	// 3. the set is closed under the checkout's intra-repo requires.
	for path := range replaced {
		dir, ok := shipped[path]
		if !ok {
			continue // not a module of this checkout (checked in 4)
		}

		for _, dep := range checkoutModuleRequires(t, filepath.Join(root, dir, "go.mod")) {
			if !strings.HasPrefix(dep, frameworkModulePath+"/") {
				continue // an external dependency: the proxy serves it
			}

			if !replaced[dep] {
				t.Errorf("framework module %q is replaced but requires %q, which is not replaced", path, dep)
			}
		}
	}

	// 4. every replace points at the module it claims to.
	for path := range replaced {
		dir, ok := shipped[path]
		if !ok {
			t.Errorf("replace %q points at a module the checkout does not ship", path)

			continue
		}

		if got := checkoutModulePath(t, filepath.Join(root, dir, "go.mod")); got != path {
			t.Errorf("replace %q points at %q, whose go.mod declares %q", path, dir, got)
		}
	}
}

// scaffoldGoModModules parses the framework modules a rendered go.mod
// replaces and requires, ignoring every other module path.
func scaffoldGoModModules(t *testing.T, gomod string) (replaced, required map[string]bool) {
	t.Helper()

	replaced = map[string]bool{}
	required = map[string]bool{}

	for _, line := range strings.Split(gomod, "\n") {
		fields := strings.Fields(line)

		switch {
		case len(fields) >= 3 && fields[0] == "replace" && strings.HasPrefix(fields[1], frameworkModulePath):
			replaced[fields[1]] = true
		case len(fields) >= 2 && fields[0] == "require" && strings.HasPrefix(fields[1], frameworkModulePath):
			required[fields[1]] = true
		}
	}

	return replaced, required
}

// checkoutRuntimeModules returns every framework module the checkout ships
// under the runtime roots (the batteries, the adapters and the shared
// helpers), keyed by module path. It walks the filesystem directly, so it
// stays independent of the scaffold's own module-graph scan.
func checkoutRuntimeModules(t *testing.T, root string) map[string]string {
	t.Helper()

	return checkoutModulesUnder(t, root, "adapters", "core", "shared")
}

// checkoutModuleDirs returns every framework module the checkout ships,
// keyed by module path, with its repo-relative directory.
func checkoutModuleDirs(t *testing.T, root string) map[string]string {
	t.Helper()

	return checkoutModulesUnder(t, root, "")
}

// checkoutModulesUnder returns every framework module the checkout ships
// under the given repo-relative directories (the whole checkout when the
// only directory is ""), keyed by module path, with its repo-relative
// directory.
func checkoutModulesUnder(t *testing.T, root string, tops ...string) map[string]string {
	t.Helper()

	mods := map[string]string{}

	for _, top := range tops {
		base := filepath.Join(root, filepath.FromSlash(top))

		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}

			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}

				return nil
			}

			if d.Name() != "go.mod" {
				return nil
			}

			path := checkoutModulePath(t, p)
			if path == "" || !strings.HasPrefix(path, frameworkModulePath+"/") {
				return nil
			}

			rel, rerr := filepath.Rel(root, filepath.Dir(p))
			if rerr != nil {
				return rerr
			}

			mods[path] = filepath.ToSlash(rel)

			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}

	return mods
}

// checkoutModulePath reads the module path a go.mod declares, or "" when it
// declares none.
func checkoutModulePath(t *testing.T, gomodPath string) string {
	t.Helper()

	data, err := os.ReadFile(gomodPath)
	if err != nil {
		t.Fatalf("read %s: %v", gomodPath, err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module ")
		if !ok {
			continue
		}

		return strings.Trim(strings.TrimSpace(rest), `"`)
	}

	return ""
}

// checkoutModuleRequires returns the module paths a go.mod requires, in
// both the single-line and the block form, ignoring every other path.
func checkoutModuleRequires(t *testing.T, gomodPath string) []string {
	t.Helper()

	data, err := os.ReadFile(gomodPath)
	if err != nil {
		t.Fatalf("read %s: %v", gomodPath, err)
	}

	var out []string

	inBlock := false

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		if inBlock {
			if trimmed == ")" {
				inBlock = false

				continue
			}

			if fields := strings.Fields(trimmed); len(fields) > 0 {
				out = append(out, fields[0])
			}

			continue
		}

		if trimmed == "require (" || trimmed == "require(" {
			inBlock = true

			continue
		}

		if rest, ok := strings.CutPrefix(trimmed, "require "); ok {
			if fields := strings.Fields(rest); len(fields) > 0 {
				out = append(out, fields[0])
			}
		}
	}

	return out
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
	readme := renderNewReadme("acme")

	if strings.Contains(readme, "zengo") {
		t.Fatalf("README still mentions zengo:\n%s", readme)
	}

	for _, fragment := range []string{"# acme", "zever compile --backend=zenorm,proto,atlas", "zever serve", "curl localhost:8080/hello"} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("README lacks %q:\n%s", fragment, readme)
		}
	}

	if strings.Count(readme, "```bash") != 1 {
		t.Fatalf("README must hold exactly one 4-command quickstart block:\n%s", readme)
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
		Batteries:        minimalScaffoldBatteries,
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

// --- opt-in picker tests ---

// TestParseBatteriesFlag pins --batteries parsing: sorted de-duplicated
// output, empty means none, unknown names fail closed.
func TestParseBatteriesFlag(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := parseBatteriesFlag("queue,db,db")
		if err != nil {
			t.Fatalf("parseBatteriesFlag: %v", err)
		}

		if len(got) != 2 || got[0] != "db" || got[1] != "queue" {
			t.Fatalf("parseBatteriesFlag = %v, want [db queue]", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		for _, raw := range []string{"", "  ", " , "} {
			got, err := parseBatteriesFlag(raw)
			if err != nil {
				t.Fatalf("parseBatteriesFlag(%q): %v", raw, err)
			}

			if len(got) != 0 {
				t.Fatalf("parseBatteriesFlag(%q) = %v, want empty", raw, got)
			}
		}
	})

	t.Run("unknown", func(t *testing.T) {
		if _, err := parseBatteriesFlag("db,nope"); err == nil {
			t.Fatal("expected an error for an unknown battery")
		} else if !strings.Contains(err.Error(), "nope") {
			t.Fatalf("error %q does not name the bad battery", err)
		}
	})
}

// TestParseAdaptersFlag pins --adapters parsing: pairs, malformed entries,
// unknown batteries and unknown adapters all fail closed.
func TestParseAdaptersFlag(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := parseAdaptersFlag("db=postgres,router=fiber")
		if err != nil {
			t.Fatalf("parseAdaptersFlag: %v", err)
		}

		if got["db"] != "postgres" || got["router"] != "fiber" {
			t.Fatalf("parseAdaptersFlag = %v, want db=postgres router=fiber", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got, err := parseAdaptersFlag("")
		if err != nil {
			t.Fatalf("parseAdaptersFlag: %v", err)
		}

		if len(got) != 0 {
			t.Fatalf("parseAdaptersFlag = %v, want empty", got)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		for _, raw := range []string{"db", "db=", "=postgres", "db-postgres"} {
			if _, err := parseAdaptersFlag(raw); err == nil {
				t.Errorf("expected an error for %q", raw)
			}
		}
	})

	t.Run("unknown battery", func(t *testing.T) {
		if _, err := parseAdaptersFlag("nope=memory"); err == nil {
			t.Fatal("expected an error for an unknown battery")
		}
	})

	t.Run("unknown adapter", func(t *testing.T) {
		if _, err := parseAdaptersFlag("db=oracle"); err == nil {
			t.Fatal("expected an error for an unknown adapter")
		}
	})
}

// TestParseWithFlag pins --with parsing: bare names and battery=adapter
// pins unify, sorted and de-duplicated, unknown names fail closed with
// hints.
func TestParseWithFlag(t *testing.T) {
	t.Run("mixed", func(t *testing.T) {
		batteries, overrides, err := parseWithFlag("queue,db=postgres,db")
		if err != nil {
			t.Fatalf("parseWithFlag: %v", err)
		}

		if len(batteries) != 2 || batteries[0] != "db" || batteries[1] != "queue" {
			t.Fatalf("parseWithFlag batteries = %v, want [db queue]", batteries)
		}

		if overrides["db"] != "postgres" {
			t.Fatalf("parseWithFlag overrides = %v, want db=postgres", overrides)
		}
	})

	t.Run("empty", func(t *testing.T) {
		batteries, overrides, err := parseWithFlag("")
		if err != nil {
			t.Fatalf("parseWithFlag: %v", err)
		}

		if len(batteries) != 0 || len(overrides) != 0 {
			t.Fatalf("parseWithFlag = (%v, %v), want empty", batteries, overrides)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		if _, _, err := parseWithFlag("db="); err == nil {
			t.Fatal("expected an error for a malformed entry")
		}
	})

	t.Run("unknown battery hints", func(t *testing.T) {
		if _, _, err := parseWithFlag("cach"); err == nil {
			t.Fatal("expected an error for an unknown battery")
		} else if !strings.Contains(err.Error(), "cache") {
			t.Fatalf("error %q lacks a did-you-mean hint", err)
		}
	})

	t.Run("unknown adapter hints", func(t *testing.T) {
		if _, _, err := parseWithFlag("db=oracle"); err == nil {
			t.Fatal("expected an error for an unknown adapter")
		} else if !strings.Contains(err.Error(), "sqlite") && !strings.Contains(err.Error(), "postgres") {
			t.Fatalf("error %q lacks choices", err)
		}
	})
}

// TestProfileReplacementHint pins the --profile deprecation: each preset
// names its --with replacement.
func TestProfileReplacementHint(t *testing.T) {
	t.Parallel()

	for _, profile := range []string{"minimal", "api", "full"} {
		hint := profileReplacementHint(profile)
		if !strings.Contains(hint, "--with") {
			t.Errorf("profileReplacementHint(%q) = %q, want a --with replacement", profile, hint)
		}
	}
}

// TestRunNewProfileAliasPrintsReplacement proves --profile still works but
// announces its --with replacement on stderr.
func TestRunNewProfileAliasPrintsReplacement(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	errOut := captureStderr(t, func() {
		if err := runNew([]string{"profapp", "--profile", "api", "--no-tidy"}); err != nil {
			t.Fatalf("runNew --profile api: %v", err)
		}
	})

	if !strings.Contains(errOut, "--with") {
		t.Fatalf("expected a --with replacement notice on stderr, got:\n%s", errOut)
	}

	wiring := readFile(t, filepath.Join(workDir, "profapp", "internal", "app", "wiring.go"))
	if !strings.Contains(wiring, "adapters/queue/memory") {
		t.Fatalf("profile api did not wire the queue battery:\n%s", wiring)
	}
}

// TestBatteryAdapters pins the picker's adapter universe: derived from
// nestedModuleDirs, default first, covering every known service, with the
// password dir exception mapped back to argon2id.
func TestBatteryAdapters(t *testing.T) {
	all := batteryAdapters()

	if len(all) != len(allServiceAdapters) {
		t.Fatalf("batteryAdapters covers %d batteries, want %d", len(all), len(allServiceAdapters))
	}

	for battery, def := range allServiceAdapters {
		choices := all[battery]
		if len(choices) == 0 {
			t.Errorf("batteryAdapters[%q] empty", battery)
			continue
		}

		if choices[0] != def {
			t.Errorf("batteryAdapters[%q][0] = %q, want default %q", battery, choices[0], def)
		}

		if !sort.StringsAreSorted(choices[1:]) {
			t.Errorf("batteryAdapters[%q] tail not sorted: %v", battery, choices)
		}
	}

	db := all["db"]
	if !batteryPicked(db, "sqlite") || !batteryPicked(db, "postgres") {
		t.Fatalf("batteryAdapters[db] = %v, want sqlite and postgres", db)
	}

	// Consolidated DB-backed batteries offer canonical "db" first (the
	// default) with legacy aliases after, all routing to the same db dir.
	for battery, want := range map[string][]string{
		"search":      {"db", "postgres", "sqlite", "meilisearch"},
		"vectorstore": {"db", "pgvector", "sqlite", "qdrant"},
		"workflow":    {"db", "postgres", "memory"},
	} {
		for _, adapter := range want {
			if !batteryPicked(all[battery], adapter) {
				t.Errorf("batteryAdapters[%q] = %v, want %q", battery, all[battery], adapter)
			}
		}
	}

	if pw := all["password"]; !batteryPicked(pw, "argon2id") {
		t.Fatalf("batteryAdapters[password] = %v, want PHC-canonical argon2id", pw)
	}
}

// TestRunNewWithFlagScaffoldsExtras proves --with extends the minimal set
// in both wiring.go and go.mod without any prompting.
func TestRunNewWithFlagScaffoldsExtras(t *testing.T) {
	resetInteractiveMode(t)

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"flagapp", "--with", "cache"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	wiring := readFile(t, filepath.Join(workDir, "flagapp", "internal", "app", "wiring.go"))

	for _, want := range []string{"log", "router", "db", "cache"} {
		if !strings.Contains(wiring, "/"+want+"/") {
			t.Errorf("wiring.go missing service %q", want)
		}
	}

	gomod := readFile(t, filepath.Join(workDir, "flagapp", "go.mod"))

	for _, fragment := range []string{
		"require github.com/zenta-dev/zever/adapters/db/sqlite ",
		"require github.com/zenta-dev/zever/adapters/cache/memory ",
	} {
		if !strings.Contains(gomod, fragment) {
			t.Errorf("go.mod lacks %q:\n%s", fragment, gomod)
		}
	}
}

// TestRunNewWithAdapterPinOverrides proves --with steers the adapter pick
// and implies its battery, so --with db=postgres alone scaffolds db on
// postgres.
func TestRunNewWithAdapterPinOverrides(t *testing.T) {
	resetInteractiveMode(t)

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"adaptapp", "--with", "db=postgres"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	wiring := readFile(t, filepath.Join(workDir, "adaptapp", "internal", "app", "wiring.go"))

	if !strings.Contains(wiring, "adapters/db/postgres") {
		t.Fatalf("wiring.go lacks the postgres adapter wiring:\n%s", wiring)
	}

	gomod := readFile(t, filepath.Join(workDir, "adaptapp", "go.mod"))

	if !strings.Contains(gomod, "require github.com/zenta-dev/zever/adapters/db/postgres ") {
		t.Fatalf("go.mod lacks the postgres adapter require:\n%s", gomod)
	}

	if strings.Contains(gomod, "require github.com/zenta-dev/zever/adapters/db/sqlite ") {
		t.Fatalf("go.mod unexpectedly requires sqlite alongside postgres:\n%s", gomod)
	}
}

// TestRunNewBatteriesFlagStillWorks proves the legacy --batteries flag keeps
// working (unified under --with internally).
func TestRunNewBatteriesFlagStillWorks(t *testing.T) {
	resetInteractiveMode(t)

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"legacyapp", "--batteries", "cache", "--adapters", "cache=redis"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	wiring := readFile(t, filepath.Join(workDir, "legacyapp", "internal", "app", "wiring.go"))

	if !strings.Contains(wiring, "adapters/cache/redis") {
		t.Fatalf("wiring.go lacks the redis adapter wiring:\n%s", wiring)
	}
}

// TestListBatteries prints the name+default table to stdout and exits 0
// without needing an app name or scaffolding anything.
func TestListBatteries(t *testing.T) {
	resetInteractiveMode(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	out := captureStdout(t, func() {
		if err := runNew([]string{"--list-batteries"}); err != nil {
			t.Fatalf("runNew --list-batteries: %v", err)
		}
	})

	for _, want := range []string{"BATTERY", "db sqlite", "log slog", "router stdhttp"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}

	if entries, _ := os.ReadDir(workDir); len(entries) != 0 {
		t.Fatalf("--list-batteries scaffolded files: %v", entries)
	}
}

// TestRunNewYesSkipsPicker proves -y/--yes stays on the flags path even
// when interactive input was requested globally.
func TestRunNewYesSkipsPicker(t *testing.T) {
	resetInteractiveMode(t)
	interactiveMode = true

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	stubPickerFuncs(t, nil, nil, true)

	if err := runNew([]string{"yesapp", "-y"}); err != nil {
		t.Fatalf("runNew -y: %v", err)
	}

	wiring := readFile(t, filepath.Join(workDir, "yesapp", "internal", "app", "wiring.go"))

	for _, b := range []string{"db", "log", "observability", "ratelimit", "router"} {
		if !strings.Contains(wiring, "/"+b+"/") {
			t.Errorf("-y wiring.go missing minimal service %q", b)
		}
	}
}

// TestRunNewInteractiveWithoutTTYFails proves the off-TTY picker path
// errors (TTY, never a hang) and leaves no scaffold behind.
func TestRunNewInteractiveWithoutTTYFails(t *testing.T) {
	stubPromptTTY(t, false)
	setPromptInteractive(t)

	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	err := runNew([]string{"ttyapp", "--interactive"})
	if err == nil {
		t.Fatal("expected a TTY error, got nil")
	}

	if !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("error = %q, want mention of TTY", err)
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "ttyapp")); !os.IsNotExist(statErr) {
		t.Fatal("--interactive without TTY left a scaffold behind")
	}
}

// TestRunNewOffTTYArityFailsFast proves the bare off-TTY misuse path is
// untouched: usage + error, no prompt, no hang.
func TestRunNewOffTTYArityFailsFast(t *testing.T) {
	stubPromptTTY(t, false)

	workDir := t.TempDir()
	withWorkingDir(t, workDir)

	if err := runNew(nil); err == nil {
		t.Fatal("expected an arity error, got nil")
	}
}

// stubPickerFuncs replaces the picker seams for a test: batteries/adapters
// are the canned answers (nil = prompt reached only when the test stubs a
// failure), failOnCall turns any prompt attempt into a test failure for
// flag-parity proofs.
func stubPickerFuncs(t *testing.T, batteries []string, adapters map[string]string, failOnCall bool) {
	t.Helper()

	prevBatteries := pickBatteriesFunc
	prevAdapter := pickAdapterFunc
	t.Cleanup(func() { pickBatteriesFunc, pickAdapterFunc = prevBatteries, prevAdapter })

	pickBatteriesFunc = func(_, _ []string) ([]string, error) {
		if failOnCall {
			t.Fatal("battery prompt ran despite flags covering it")
		}

		return batteries, nil
	}
	pickAdapterFunc = func(battery string, options []string) (string, error) {
		if failOnCall {
			t.Fatalf("adapter prompt ran for %q despite flags covering it", battery)
		}

		if a, ok := adapters[battery]; ok {
			return a, nil
		}

		return options[0], nil
	}
}

// TestRunNewPickerStubbed proves the wizard feeds the minimal set plus
// picks: canned picks land in wiring.go and go.mod.
func TestRunNewPickerStubbed(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)
	stubPickerFuncs(t, []string{"log", "router", "db"}, map[string]string{"db": "postgres"}, false)

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"pickapp", "--interactive"}); err != nil {
		t.Fatalf("runNew --interactive: %v", err)
	}

	wiring := readFile(t, filepath.Join(workDir, "pickapp", "internal", "app", "wiring.go"))

	if !strings.Contains(wiring, "adapters/db/postgres") {
		t.Fatalf("wiring.go lacks the picked postgres adapter wiring:\n%s", wiring)
	}

	gomod := readFile(t, filepath.Join(workDir, "pickapp", "go.mod"))

	if !strings.Contains(gomod, "require github.com/zenta-dev/zever/adapters/db/postgres ") {
		t.Fatalf("go.mod lacks the picked postgres adapter:\n%s", gomod)
	}
}

// TestRunNewFlagsSkipPickerPrompts is the flag-parity proof: with
// --with covering every prompt, the wizard must not prompt at all.
func TestRunNewFlagsSkipPickerPrompts(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)
	stubPickerFuncs(t, nil, nil, true)

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	args := []string{"parityapp", "--interactive", "--with", "db=postgres"}
	if err := runNew(args); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	wiring := readFile(t, filepath.Join(workDir, "parityapp", "internal", "app", "wiring.go"))

	if !strings.Contains(wiring, "adapters/db/postgres") {
		t.Fatalf("wiring.go lacks the postgres adapter wiring:\n%s", wiring)
	}
}

// TestBatterySelectionsAdaptersMap pins the render seam: the Adapters map
// steers every battery, and the legacy fields still win over it.
func TestBatterySelectionsAdaptersMap(t *testing.T) {
	cfg := NewConfig{
		Batteries: []string{"db", "router"},
		Adapters:  map[string]string{"db": "postgres", "router": "fiber"},
	}

	sel := cfg.batterySelections()
	byName := map[string]string{}

	for _, s := range sel {
		byName[s.Battery] = s.Adapter
	}

	if byName["db"] != "postgres" || byName["router"] != "fiber" {
		t.Fatalf("batterySelections = %v, want db=postgres router=fiber", byName)
	}

	cfg.DBAdapter = "sqlite"

	if got := cfg.batterySelections()[0].Adapter; got != "sqlite" {
		t.Fatalf("legacy DBAdapter lost to the map: %q", got)
	}
}

// TestRefVarName pins the Ref var naming: short batteries uppercase, named
// instances append the title-cased name.
func TestRefVarName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		battery, name, want string
	}{
		{"db", "", "DB"},
		{"db", "default", "DB"},
		{"db", "analytics", "DBAnalytics"},
		{"cache", "", "Cache"},
		{"cache", "sessions", "CacheSessions"},
		{"router", "default", "Router"},
		{"log", "", "Log"},
	}

	for _, tc := range cases {
		if got := refVarName(tc.battery, tc.name); got != tc.want {
			t.Errorf("refVarName(%q, %q) = %q, want %q", tc.battery, tc.name, got, tc.want)
		}
	}
}

// TestFactoryExprShapes pins the provider factory shapes: direct New for
// core-Options adapters, WrapNoErr/WrapNoOpts for infallible ctors,
// closures for embedded-DB adapters, and fail-closed for helpers.
func TestFactoryExprShapes(t *testing.T) {
	t.Parallel()

	direct, err := factoryExpr(batterySelection{Battery: "db", Adapter: "postgres"}, "dbpostgres", "\t\t")
	if err != nil || direct != "dbpostgres.New" {
		t.Errorf("db/postgres = %q, %v; want dbpostgres.New", direct, err)
	}

	noErr, err := factoryExpr(batterySelection{Battery: "log", Adapter: "slog"}, "logslog", "\t\t")
	if err != nil || noErr != "container.WrapNoErr(logslog.New)" {
		t.Errorf("log/slog = %q, %v", noErr, err)
	}

	noOpts, err := factoryExpr(batterySelection{Battery: "log", Adapter: "noop"}, "lognoop", "\t\t")
	if err != nil || noOpts != "container.WrapNoOpts(lognoop.New)" {
		t.Errorf("log/noop = %q, %v", noOpts, err)
	}

	embedded, err := factoryExpr(batterySelection{Battery: "cache", Adapter: "db"}, "cachedb", "\t\t")
	if err != nil || !strings.Contains(embedded, "cachedb.New(cachedb.Options{Options: o})") {
		t.Errorf("cache/db = %q, %v", embedded, err)
	}

	if _, err := factoryExpr(batterySelection{Battery: "session", Adapter: "cookie"}, "sessioncookie", "\t\t"); err == nil {
		t.Error("session/cookie must fail closed with a hand-wire hint")
	} else if !strings.Contains(err.Error(), "cookie") {
		t.Errorf("session/cookie error lacks guidance: %v", err)
	}
}

// TestRenderWiringContentMinimal pins the wiring.go bytes for the minimal
// stack: three Ref vars, three Provide entries, hello import.
func TestRenderWiringContentMinimal(t *testing.T) {
	t.Parallel()

	content, err := renderWiringContent("test", "example.com/acme", []batterySelection{
		{Battery: "db", Adapter: "sqlite"},
		{Battery: "log", Adapter: "slog"},
		{Battery: "observability", Adapter: "stdout"},
		{Battery: "ratelimit", Adapter: "memory"},
		{Battery: "router", Adapter: "stdhttp"},
	})
	if err != nil {
		t.Fatalf("renderWiringContent: %v", err)
	}

	for _, fragment := range []string{
		"var DB = db.DefaultRef()",
		"var Log = log.DefaultRef()",
		"var Router = router.DefaultRef()",
		"var Observability = observability.DefaultRef()",
		"var RateLimit = ratelimit.DefaultRef()",
		"container.Provide(DB, dbsqlite.New)",
		"container.WrapNoErr(logslog.New)",
		"container.Provide(Router, routerstdhttp.New)",
		`"example.com/acme/internal/service/hello"`,
		"hello.New()",
	} {
		if !strings.Contains(string(content), fragment) {
			t.Errorf("wiring.go lacks %q:\n%s", fragment, content)
		}
	}
}

func TestIsTransientGoFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		out  string
		want bool
	}{
		{"dial tcp: lookup proxy.golang.org: no such host", true},
		{"read tcp: connection reset by peer", true},
		{"net/http: TLS handshake timeout", true},
		{"reading https://proxy.golang.org/x/@v/list: 503 Service Unavailable", true},
		{"undefined: foo", false},
		{"module lookup disabled by GOPROXY=off", false},
		{"", false},
	}

	for _, c := range cases {
		if got := isTransientGoFailure(c.out); got != c.want {
			t.Errorf("isTransientGoFailure(%q) = %v, want %v", c.out, got, c.want)
		}
	}
}
