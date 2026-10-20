package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAddFixture lays out a minimal calling project in dir: a go.mod
// requiring the framework container at v0.4.0 (plus a root replace when
// frameworkDir is set), a minimal internal/app/wiring.go, and a matching
// .env.example. It returns the starting file contents for later comparison.
func writeAddFixture(t *testing.T, dir, frameworkDir string) {
	t.Helper()

	gomod := "module example.com/shop\n\ngo 1.24\n\nrequire github.com/zenta-dev/zever/container v0.4.0\n"
	if frameworkDir != "" {
		gomod += "replace github.com/zenta-dev/zever => " + frameworkDir + "\n"
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	wiring, err := renderWiringContent("test", "example.com/shop", []batterySelection{
		{Battery: "db", Adapter: "sqlite"},
		{Battery: "log", Adapter: "slog"},
		{Battery: "observability", Adapter: "stdout"},
		{Battery: "ratelimit", Adapter: "memory"},
		{Battery: "router", Adapter: "stdhttp"},
	})
	if err != nil {
		t.Fatalf("renderWiringContent: %v", err)
	}

	appPath := filepath.Join(dir, "internal", "app", "wiring.go")

	if merr := os.MkdirAll(filepath.Dir(appPath), 0o750); merr != nil {
		t.Fatalf("mkdir app: %v", merr)
	}

	if werr := os.WriteFile(appPath, wiring, 0o600); werr != nil {
		t.Fatalf("write wiring.go: %v", werr)
	}

	example := renderEnvExample([]batterySelection{
		{Battery: "db", Adapter: "sqlite"},
		{Battery: "log", Adapter: "slog"},
		{Battery: "observability", Adapter: "stdout"},
		{Battery: "ratelimit", Adapter: "memory"},
		{Battery: "router", Adapter: "stdhttp"},
	})

	if err := os.WriteFile(filepath.Join(dir, ".env.example"), example, 0o600); err != nil {
		t.Fatalf("write .env.example: %v", err)
	}
}

// stubAddToolchain stubs `go get` and `go mod tidy` for unit tests,
// recording calls. The E2E build test below runs the real toolchain.
func stubAddToolchain(t *testing.T) (getCalls *[]string, tidyCalls *[]string) {
	t.Helper()

	getCalls = &[]string{}
	tidyCalls = &[]string{}

	prevGet := runGoGetFunc
	prevTidy := runGoModTidyFunc

	runGoGetFunc = func(_ string, _ string, paths []string, version string) error {
		for _, p := range paths {
			*getCalls = append(*getCalls, p+"@"+version)
		}

		return nil
	}
	runGoModTidyFunc = func(_ string, dir string) error {
		*tidyCalls = append(*tidyCalls, dir)

		return nil
	}

	t.Cleanup(func() { runGoGetFunc, runGoModTidyFunc = prevGet, prevTidy })

	return getCalls, tidyCalls
}

func TestParseAddTarget(t *testing.T) {
	t.Parallel()

	t.Run("battery only resolves default", func(t *testing.T) {
		t.Parallel()

		tgt, err := parseAddTarget("cache", "", "", "")
		if err != nil {
			t.Fatalf("parseAddTarget: %v", err)
		}

		if tgt.battery != "cache" || tgt.adapter != "memory" || tgt.name != "" || tgt.uses != "" {
			t.Fatalf("parseAddTarget = %+v, want cache/memory default", tgt)
		}
	})

	t.Run("explicit adapter", func(t *testing.T) {
		t.Parallel()

		tgt, err := parseAddTarget("cache/redis", "", "", "")
		if err != nil {
			t.Fatalf("parseAddTarget: %v", err)
		}

		if tgt.adapter != "redis" {
			t.Fatalf("adapter = %q, want redis", tgt.adapter)
		}
	})

	t.Run("flag adapter equals slash form", func(t *testing.T) {
		t.Parallel()

		tgt, err := parseAddTarget("cache/redis", "redis", "", "")
		if err != nil {
			t.Fatalf("parseAddTarget: %v", err)
		}

		if tgt.adapter != "redis" {
			t.Fatalf("adapter = %q, want redis", tgt.adapter)
		}
	})

	t.Run("adapter conflict", func(t *testing.T) {
		t.Parallel()

		if _, err := parseAddTarget("cache/redis", "memory", "", ""); err == nil {
			t.Fatal("conflicting adapters succeeded, want error")
		} else if !strings.Contains(err.Error(), "conflict") {
			t.Fatalf("error lacks conflict: %v", err)
		}
	})

	t.Run("name and uses", func(t *testing.T) {
		t.Parallel()

		tgt, err := parseAddTarget("db", "", "analytics", "cache")
		if err != nil {
			t.Fatalf("parseAddTarget: %v", err)
		}

		if tgt.name != "analytics" || tgt.uses != "cache" {
			t.Fatalf("parseAddTarget = %+v", tgt)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		if _, err := parseAddTarget("", "", "", ""); err == nil {
			t.Fatal("empty arg succeeded, want error")
		}
	})

	t.Run("extra slash", func(t *testing.T) {
		t.Parallel()

		if _, err := parseAddTarget("a/b/c", "", "", ""); err == nil {
			t.Fatal("extra slash succeeded, want error")
		}
	})

	t.Run("unknown battery hints", func(t *testing.T) {
		t.Parallel()

		if _, err := parseAddTarget("cach", "", "", ""); err == nil {
			t.Fatal("unknown battery succeeded, want error")
		} else if !strings.Contains(err.Error(), "cache") {
			t.Fatalf("error lacks did-you-mean hint: %v", err)
		}
	})

	t.Run("unknown adapter lists choices", func(t *testing.T) {
		t.Parallel()

		if _, err := parseAddTarget("db/oracle", "", "", ""); err == nil {
			t.Fatal("unknown adapter succeeded, want error")
		} else if !strings.Contains(err.Error(), "sqlite") {
			t.Fatalf("error lacks choices: %v", err)
		}
	})

	t.Run("blocked adapter fails closed", func(t *testing.T) {
		t.Parallel()

		if _, err := parseAddTarget("session/cookie", "", "", ""); err == nil {
			t.Fatal("session/cookie succeeded, want hand-wire error")
		}
	})

	t.Run("bad name", func(t *testing.T) {
		t.Parallel()

		if _, err := parseAddTarget("db", "", "a__b", ""); err == nil {
			t.Fatal("bad instance name succeeded, want error")
		}
	})
}

func TestRunAddEditsWiringFiles(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	getCalls, tidyCalls := stubAddToolchain(t)

	if err := runAdd([]string{"cache/redis", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	if len(*getCalls) != 2 {
		t.Fatalf("go get calls = %v, want the core+adapter requires", *getCalls)
	}

	if len(*tidyCalls) != 1 {
		t.Fatalf("go mod tidy calls = %v, want one", *tidyCalls)
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

	wiring := readGenerated(t, filepath.Join(dir, "internal", "app", "wiring.go"))

	for _, fragment := range []string{
		`cacheredis "github.com/zenta-dev/zever/adapters/cache/redis"`,
		"var Cache = cache.DefaultRef()",
		"container.Provide(Cache, cacheredis.New)",
		// The minimal stack survives alongside the addition.
		"var DB = db.DefaultRef()",
		"container.Provide(DB,",
	} {
		if !strings.Contains(wiring, fragment) {
			t.Fatalf("wiring.go lacks %q:\n%s", fragment, wiring)
		}
	}

	example := readFile(t, filepath.Join(dir, ".env.example"))

	for _, fragment := range []string{"# --- cache (adapter redis) ---", "CACHE_ADAPTER=redis", "# --- db (adapter sqlite) ---"} {
		if !strings.Contains(example, fragment) {
			t.Fatalf(".env.example lacks %q:\n%s", fragment, example)
		}
	}

	// Re-adding the same battery+adapter is an idempotent no-op: files are
	// byte-identical and the command still succeeds.
	before := map[string]string{
		"go.mod":       gomod,
		"wiring.go":    wiring,
		".env.example": example,
	}

	if err := runAdd([]string{"cache/redis", "--yes"}); err != nil {
		t.Fatalf("runAdd again: %v", err)
	}

	for file, want := range map[string]string{
		"go.mod":       readFile(t, filepath.Join(dir, "go.mod")),
		"wiring.go":    readGenerated(t, filepath.Join(dir, "internal", "app", "wiring.go")),
		".env.example": readFile(t, filepath.Join(dir, ".env.example")),
	} {
		if want != before[file] {
			t.Fatalf("%s changed on idempotent re-add:\n--- before ---\n%s\n--- after ---\n%s", file, before[file], want)
		}
	}
}

func TestRunAddResolvesDefaultAdapter(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	if err := runAdd([]string{"cache", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	gomod := readFile(t, filepath.Join(dir, "go.mod"))
	if !strings.Contains(gomod, "require github.com/zenta-dev/zever/adapters/cache/memory v0.4.0") {
		t.Fatalf("go.mod lacks the default memory adapter require:\n%s", gomod)
	}

	example := readFile(t, filepath.Join(dir, ".env.example"))
	if !strings.Contains(example, "CACHE_ADAPTER=memory") {
		t.Fatalf(".env.example lacks the default memory section:\n%s", example)
	}
}

func TestRunAddDerivesLocalReplaces(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "/opt/zever")
	stubAddToolchain(t)

	if err := runAdd([]string{"db/postgres", "--force", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	gomod := readFile(t, filepath.Join(dir, "go.mod"))

	for _, fragment := range []string{
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
	stubAddToolchain(t)

	gomodPath := filepath.Join(dir, "go.mod")

	data, err := os.ReadFile(gomodPath) // test fixture path
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	pinned := strings.Replace(string(data), "require github.com/zenta-dev/zever/container v0.4.0", "require github.com/zenta-dev/zever/container v1.9.9", 1)

	if werr := os.WriteFile(gomodPath, []byte(pinned), 0o600); werr != nil { //nolint:gosec // test temp dir
		t.Fatalf("write go.mod: %v", werr)
	}

	if err := runAdd([]string{"queue/redis", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	gomod := readFile(t, gomodPath)

	for _, fragment := range []string{
		"require github.com/zenta-dev/zever/core/queue v1.9.9",
		"require github.com/zenta-dev/zever/adapters/queue/redis v1.9.9",
	} {
		if !strings.Contains(gomod, fragment) {
			t.Fatalf("go.mod lacks %q (must match the pinned version):\n%s", fragment, gomod)
		}
	}
}

func TestRunAddSwapSqlitePostgres(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	// Swapping without --force names the current adapter instead.
	if err := runAdd([]string{"db/postgres", "--yes"}); err == nil {
		t.Fatal("swap without --force succeeded, want an error naming sqlite")
	} else if !strings.Contains(err.Error(), "sqlite") {
		t.Fatalf("swap error does not name the current adapter: %v", err)
	}

	if err := runAdd([]string{"db/postgres", "--force", "--yes"}); err != nil {
		t.Fatalf("runAdd --force: %v", err)
	}

	wiring := readGenerated(t, filepath.Join(dir, "internal", "app", "wiring.go"))

	// Exactly one DB provider line, on postgres; the sqlite import is gone.
	if got := strings.Count(wiring, "container.Provide(DB,"); got != 1 {
		t.Fatalf("wiring.go has %d DB provider entries, want 1:\n%s", got, wiring)
	}

	for _, fragment := range []string{
		"container.Provide(DB, dbpostgres.New)",
		`"github.com/zenta-dev/zever/adapters/db/postgres"`,
	} {
		if !strings.Contains(wiring, fragment) {
			t.Fatalf("wiring.go lacks %q:\n%s", fragment, wiring)
		}
	}

	if strings.Contains(wiring, "sqlite") {
		t.Fatalf("wiring.go still references sqlite after the swap:\n%s", wiring)
	}
}

func TestRunAddNamedInstance(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	if err := runAdd([]string{"db", "--name", "analytics", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	wiring := readGenerated(t, filepath.Join(dir, "internal", "app", "wiring.go"))

	for _, fragment := range []string{
		`var DBAnalytics = db.Ref("analytics")`,
		"container.Provide(DBAnalytics, dbsqlite.New)",
		// The default instance survives untouched.
		"var DB = db.DefaultRef()",
		"container.Provide(DB, dbsqlite.New)",
	} {
		if !strings.Contains(wiring, fragment) {
			t.Fatalf("wiring.go lacks %q:\n%s", fragment, wiring)
		}
	}

	example := readFile(t, filepath.Join(dir, ".env.example"))

	for _, fragment := range []string{
		"# --- db (adapter sqlite, instance analytics) ---",
		"DB__ANALYTICS__ADAPTER=sqlite",
		"DB__ANALYTICS__PATH=",
	} {
		if !strings.Contains(example, fragment) {
			t.Fatalf(".env.example lacks %q:\n%s", fragment, example)
		}
	}
}

func TestRunAddUsesEdge(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	if err := runAdd([]string{"cache", "--uses", "db", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	wiring := readGenerated(t, filepath.Join(dir, "internal", "app", "wiring.go"))

	if !strings.Contains(wiring, "container.Provide1(Cache, DB, func(o cache.Options, _ db.DB) (cache.Cache, error)") {
		t.Fatalf("wiring.go lacks the Provide1 dependency edge:\n%s", wiring)
	}

	// --uses an instance that is not wired fails with the fix.
	if err := runAdd([]string{"queue", "--uses", "db/reporting", "--yes"}); err == nil {
		t.Fatal("runAdd --uses unwired instance succeeded, want error")
	} else if !strings.Contains(err.Error(), "zever add db --name reporting") {
		t.Fatalf("uses error lacks the fix: %v", err)
	}
}

// TestRunAddDiffPrintsOnly is the R2 diffing-rule proof: --diff prints the
// unified wiring.go diff and changes nothing on disk.
func TestRunAddDiffPrintsOnly(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")

	before := snapshotDir(t, dir)

	out := captureStdout(t, func() {
		if err := runAdd([]string{"cache", "--diff"}); err != nil {
			t.Fatalf("runAdd --diff: %v", err)
		}
	})

	for _, fragment := range []string{
		"--- a/internal/app/wiring.go",
		"+++ b/internal/app/wiring.go",
		"@@",
		"+var Cache = cache.DefaultRef()",
		"+\t\tcontainer.Provide(Cache,",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("diff output lacks %q:\n%s", fragment, out)
		}
	}

	after := snapshotDir(t, dir)

	for path, want := range before {
		if after[path] != want {
			t.Fatalf("%s changed under --diff", path)
		}
	}
}

// snapshotDir reads every project file under dir into a map.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()

	out := map[string]string{}

	for _, rel := range []string{"go.mod", "internal/app/wiring.go", ".env.example"} {
		data, err := os.ReadFile(filepath.Join(dir, rel)) // test fixture path
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}

		out[rel] = string(data)
	}

	return out
}

// TestRunAddPromptRequiresYes proves the apply prompt never hangs headless:
// without a TTY and without --yes it fails with the exact next step.
func TestRunAddPromptRequiresYes(t *testing.T) {
	stubPromptTTY(t, false)

	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	// Stub the prompt to refuse, as a headless huh form would.
	if err := runAdd([]string{"cache"}); err == nil {
		t.Fatal("runAdd without TTY or --yes succeeded, want an error")
	} else if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error lacks the --yes fix: %v", err)
	}

	wiring := readGenerated(t, filepath.Join(dir, "internal", "app", "wiring.go"))
	if strings.Contains(wiring, "var Cache") {
		t.Fatal("wiring.go changed without confirmation")
	}
}

// TestAddPreservesUserCode is the R1 proof: hand-written Ref vars, provider
// entries, comments and imports survive `zever add` byte-for-byte outside
// the inserted lines.
func TestAddPreservesUserCode(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	const userNote = "// Hand-written: tuned pool for the reporting replica.\n"

	wiringPath := filepath.Join(dir, "internal", "app", "wiring.go")
	wiring := readFile(t, wiringPath)

	customized := strings.Replace(wiring,
		"var DB = db.DefaultRef()",
		userNote+"var DB = db.DefaultRef()\n\n// Reporting is a second hand-wired replica.\nvar Reporting = db.Ref(\"reporting\")",
		1)
	customized = strings.Replace(customized,
		"container.Provide(Router, routerstdhttp.New),",
		"container.Provide(Router, routerstdhttp.New),\n\t\tcontainer.Provide(Reporting, dbsqlite.New, container.WithOptions(func(o *db.Options) { o.MaxConns = 5 })),",
		1)

	if err := os.WriteFile(wiringPath, []byte(customized), 0o600); err != nil {
		t.Fatalf("write customized wiring.go: %v", err)
	}

	if err := runAdd([]string{"cache", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	after := readGenerated(t, wiringPath)

	for _, fragment := range []string{
		userNote,
		`var Reporting = db.Ref("reporting")`,
		"container.Provide(Reporting, dbsqlite.New, container.WithOptions(func(o *db.Options) { o.MaxConns = 5 }))",
		"container.Provide(Cache, cachememory.New)",
	} {
		if !strings.Contains(after, fragment) {
			t.Fatalf("wiring.go lost user or added code %q:\n%s", fragment, after)
		}
	}
}

// TestRunAddRecipes proves every opt-in recipe writes its files from
// templates, refuses to clobber without --force, and leaves go.mod alone
// except for worker/seed entrypoint modules.
func TestRunAddRecipes(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	for _, recipe := range []string{"docker", "ci", "lint", "worker", "seed", "k8s"} {
		if err := runAdd([]string{recipe, "--yes"}); err != nil {
			t.Fatalf("runAdd %s: %v", recipe, err)
		}
	}

	for _, rel := range []string{
		"Dockerfile",
		".dockerignore",
		filepath.Join(".github", "workflows", "ci.yml"),
		".golangci.yml",
		filepath.Join(defaultWorkerEntry, "main.go"),
		filepath.Join(defaultSeedEntry, "main.go"),
		filepath.Join("k8s", "app.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("expected recipe file %s: %v", rel, err)
		}
	}

	worker := readGenerated(t, filepath.Join(dir, defaultWorkerEntry, "main.go"))
	if !strings.Contains(worker, "serve.Worker(app.Providers())") {
		t.Fatalf("worker main.go does not call serve.Worker:\n%s", worker)
	}

	// Clobber protection: existing recipe files need --force.
	if err := runAdd([]string{"docker", "--yes"}); err == nil {
		t.Fatal("second docker recipe succeeded without --force, want error")
	}

	if err := runAdd([]string{"docker", "--yes", "--force"}); err != nil {
		t.Fatalf("runAdd docker --force: %v", err)
	}

	// compose with a local-only wiring fails closed with the fix.
	if err := runAdd([]string{"compose", "--yes"}); err == nil {
		t.Fatal("compose with local-only wiring succeeded, want error")
	} else if !strings.Contains(err.Error(), "postgres") && !strings.Contains(err.Error(), "redis") {
		t.Fatalf("compose error lacks guidance: %v", err)
	}
}

// TestRunAddComposePostgres proves the compose recipe renders external
// services from the wired selections.
func TestRunAddComposePostgres(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	if err := runAdd([]string{"cache/redis", "--yes"}); err != nil {
		t.Fatalf("runAdd cache/redis: %v", err)
	}

	if err := runAdd([]string{"compose", "--yes"}); err != nil {
		t.Fatalf("runAdd compose: %v", err)
	}

	compose := readFile(t, filepath.Join(dir, "compose.yaml"))
	if !strings.Contains(compose, "redis:") {
		t.Fatalf("compose.yaml lacks the redis service:\n%s", compose)
	}
}

func TestRunAddArityErrorStaysShort(t *testing.T) {
	withWorkingDir(t, t.TempDir())

	err := runAdd(nil)
	if err == nil {
		t.Fatal("runAdd(nil): got nil error, want an arity error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "expected exactly one <battery>[/<adapter>] or recipe") {
		t.Fatalf("error = %q, want the arity complaint", msg)
	}

	if !strings.Contains(msg, "usage: zever add <battery>[/<adapter>]") {
		t.Fatalf("error = %q, want the short usage line", msg)
	}

	if !strings.Contains(msg, "zever add -h") {
		t.Fatalf("error = %q, want the pointer to the full reference", msg)
	}

	// The 40-line reference stays behind `zever add -h`; the error itself
	// must stay readable in a terminal.
	if lines := strings.Count(msg, "\n") + 1; lines > 5 {
		t.Fatalf("arity error is %d lines, want at most 5:\n%s", lines, msg)
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
		stubAddToolchain(t)

		if err := runAdd([]string{"cache", "--yes"}); err == nil {
			t.Fatal("runAdd outside a module succeeded, want a no-go.mod error")
		}
	})

	t.Run("missing wiring.go", func(t *testing.T) {
		dir := t.TempDir()
		withWorkingDir(t, dir)
		stubAddToolchain(t)

		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/shop\n"), 0o600); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}

		err := runAdd([]string{"cache", "--yes"})
		if err == nil {
			t.Fatal("runAdd without internal/app/wiring.go succeeded, want an error")
		}

		if !strings.Contains(err.Error(), "wiring.go") {
			t.Fatalf("error does not point at wiring.go: %v", err)
		}
	})
}

func TestRunAddDuplicateSwapNeedsForce(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeAddFixture(t, dir, "")
	stubAddToolchain(t)

	if err := runAdd([]string{"cache/redis", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	// Same adapter re-add is a no-op success.
	if err := runAdd([]string{"cache/redis", "--yes"}); err != nil {
		t.Fatalf("idempotent re-add: %v", err)
	}

	// Different adapter needs --force.
	err := runAdd([]string{"cache/memory", "--yes"})
	if err == nil {
		t.Fatal("swap without --force succeeded, want error")
	}

	if !strings.Contains(err.Error(), "redis") {
		t.Fatalf("swap error does not name the current adapter: %v", err)
	}
}

func TestGoModRequireVersion(t *testing.T) {
	t.Parallel()

	single := "module m\n\nrequire github.com/zenta-dev/zever/container v1.2.3\n"
	if got := goModRequireVersion(single, frameworkModulePath+"/container"); got != "v1.2.3" {
		t.Fatalf("single-line version = %q, want v1.2.3", got)
	}

	block := "module m\n\nrequire (\n\tgithub.com/zenta-dev/zever/container v4.5.6\n)\n"
	if got := goModRequireVersion(block, frameworkModulePath+"/container"); got != "v4.5.6" {
		t.Fatalf("block version = %q, want v4.5.6", got)
	}

	if got := goModRequireVersion(single, "example.com/other"); got != "" {
		t.Fatalf("missing require version = %q, want empty", got)
	}
}

func TestUnifiedDiff(t *testing.T) {
	t.Parallel()

	t.Run("insertion", func(t *testing.T) {
		t.Parallel()

		got := unifiedDiff("f", "a\nb\nc\n", "a\nB\nb\nc\n", 3)
		for _, want := range []string{"--- a/f", "+++ b/f", "@@", "+B", " a"} {
			if !strings.Contains(got, want) {
				t.Fatalf("diff lacks %q:\n%s", want, got)
			}
		}
	})

	t.Run("identical is empty", func(t *testing.T) {
		t.Parallel()

		if got := unifiedDiff("f", "a\n", "a\n", 3); got != "" {
			t.Fatalf("identical diff = %q, want empty", got)
		}
	})

	t.Run("replacement", func(t *testing.T) {
		t.Parallel()

		got := unifiedDiff("f", "a\nold\nc\n", "a\nnew\nc\n", 1)
		for _, want := range []string{"-old", "+new"} {
			if !strings.Contains(got, want) {
				t.Fatalf("diff lacks %q:\n%s", want, got)
			}
		}
	})
}

func TestEnvExampleSectionIdempotent(t *testing.T) {
	t.Parallel()

	sel := batterySelection{Battery: "cache", Adapter: "redis"}

	first := renderEnvExample([]batterySelection{sel})

	section := envExampleSection(sel, "")

	if updated, changed := appendEnvExample(string(first), section, "cache", ""); changed {
		t.Fatalf("appending an existing section reported a change: %q", updated)
	}

	if updated, changed := appendEnvExample("", section, "cache", ""); !changed || updated != section {
		t.Fatalf("append to empty = (%q, %v), want section", updated, changed)
	}
}

// TestRunAddEndToEnd scaffolds the minimal project, adds a battery, and
// proves the edited tree still builds, vets and tests green.
func TestRunAddEndToEnd(t *testing.T) {
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

	// `go get` stays stubbed: inside a local checkout every framework
	// require resolves via replace, so the real call is a no-op; the unit
	// tests above pin its arguments.
	prevGet := runGoGetFunc
	runGoGetFunc = func(string, string, []string, string) error { return nil }
	t.Cleanup(func() { runGoGetFunc = prevGet })

	if err := runNew([]string{"shop"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	out := filepath.Join(workDir, "shop")

	withWorkingDir(t, out)

	if err := runAdd([]string{"cache/redis", "--yes"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	if err := runAdd([]string{"worker", "--yes"}); err != nil {
		t.Fatalf("runAdd worker: %v", err)
	}

	pointServeAtStub(t, repoRoot, out)

	runGoInDir(t, out, "mod", "tidy")
	runGoInDir(t, out, "build", "./...")
	runGoInDir(t, out, "vet", "./...")
	runGoInDir(t, out, "test", "./...")
}
