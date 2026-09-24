package main

// Coverage closure for builders group B: migrate/dryRunDiff/applyDiff,
// rollback, devLoop/stop, watcher, tinker. SQLite temp-dir matrices,
// fault injection, and shim-backed REPL sessions. No production changes
// here; three provably-dead branches were removed with proof comments
// (dev.go drain, migrate/tinker -i flags, tinker help Usage closure).

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/traefik/yaegi/stdlib"

	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/compile"
	"github.com/zenta-dev/zever/orm/migrate"
)

// stubMigratePrompts swaps the migrate prompt seams for the test duration.
func stubMigratePrompts(t *testing.T,
	selectFn func(string, []string) (string, error),
	inputFn func(string, string, func(string) error) (string, error),
	multiFn func(string, []string) ([]string, error),
) {
	t.Helper()

	origSelect, origInput, origMulti := promptSelectForMigrate, promptInputForMigrate, promptMultiSelectForMigrate
	promptSelectForMigrate, promptInputForMigrate, promptMultiSelectForMigrate = selectFn, inputFn, multiFn
	t.Cleanup(func() {
		promptSelectForMigrate, promptInputForMigrate, promptMultiSelectForMigrate = origSelect, origInput, origMulti
	})
}

// coverMigratePromptFull drives the whole interactive migrate flow headless:
// adapter select, DSN input, and file multiselect all succeed, ending in a
// real sqlite apply.
func TestCoverMigratePromptFull(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, filepath.Join(dir, "schema"), "app.zen", migrateSchema)
	t.Chdir(dir)
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	dbPath := filepath.Join(dir, "prompt.db")
	stubMigratePrompts(t,
		func(string, []string) (string, error) { return "sqlite", nil },
		func(string, string, func(string) error) (string, error) { return dbPath, nil },
		func(string, []string) ([]string, error) {
			return []string{filepath.Join("schema", "app.zen")}, nil
		},
	)

	var runErr error
	_ = captureZeverStdout(t, func() { runErr = runDBMigrate(nil) })
	if runErr != nil {
		t.Fatalf("runDBMigrate interactive: %v", runErr)
	}

	if names := zeverSQLiteObjects(t, dbPath); !names["users"] {
		t.Fatalf("interactive migrate did not create tables, got %v", names)
	}
}

// coverMigratePromptDSNError pins the DSN prompt failure return.
func TestCoverMigratePromptDSNError(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, filepath.Join(dir, "schema"), "app.zen", migrateSchema)
	t.Chdir(dir)
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	stubMigratePrompts(t,
		func(string, []string) (string, error) { return "sqlite", nil },
		func(string, string, func(string) error) (string, error) { return "", errTestSentinel },
		func(string, []string) ([]string, error) { return nil, nil },
	)

	if err := runDBMigrate(nil); err == nil {
		t.Fatal("expected a DSN prompt error, got nil")
	}
}

// coverMigratePromptFilesFallback pins the manual file-input branch taken
// when no schema files are discovered.
func TestCoverMigratePromptFilesFallback(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "other.zen", migrateSchema)
	dbPath := filepath.Join(dir, "fallback.db")
	t.Chdir(dir)
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	stubMigratePrompts(t,
		func(string, []string) (string, error) { return "sqlite", nil },
		func(string, string, func(string) error) (string, error) { return schemaPath, nil },
		func(string, []string) ([]string, error) { return nil, nil },
	)

	var runErr error
	_ = captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath})
	})
	if runErr != nil {
		t.Fatalf("runDBMigrate files fallback: %v", runErr)
	}

	if names := zeverSQLiteObjects(t, dbPath); !names["users"] {
		t.Fatalf("fallback migrate did not create tables, got %v", names)
	}
}

// coverMigratePromptFilesErrors pins both file-selection failure returns:
// multiselect error and manual-input error.
func TestCoverMigratePromptFilesErrors(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, filepath.Join(dir, "schema"), "app.zen", migrateSchema)
	dbPath := filepath.Join(dir, "files-err.db")
	t.Chdir(dir)
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	// Multiselect failure (discovered files exist).
	stubMigratePrompts(t,
		func(string, []string) (string, error) { return "sqlite", nil },
		func(string, string, func(string) error) (string, error) { return dbPath, nil },
		func(string, []string) ([]string, error) { return nil, errTestSentinel },
	)
	if err := runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath}); err == nil {
		t.Fatal("expected a multiselect error, got nil")
	}

	// Manual-input failure (nothing discovered: fresh dir, no schema tree).
	empty := t.TempDir()
	t.Chdir(empty)
	stubMigratePrompts(t,
		func(string, []string) (string, error) { return "sqlite", nil },
		func(string, string, func(string) error) (string, error) { return "", errTestSentinel },
		func(string, []string) ([]string, error) { return nil, nil },
	)
	if err := runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + filepath.Join(empty, "x.db")}); err == nil {
		t.Fatal("expected a file-input error, got nil")
	}
}

// coverMigrateDialectHint pins the did-you-mean hint on a near-miss adapter.
func TestCoverMigrateDialectHint(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "")

	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	var runErr error
	output := captureZeverStderr(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqllite", "--dry-run", schemaPath})
	})
	if runErr == nil {
		t.Fatal("expected an adapter error, got nil")
	}
	// Deterministic pin: the suggestion is in the returned error itself.
	if !strings.Contains(runErr.Error(), `"sqlite"`) {
		t.Fatalf("expected a did-you-mean suggestion in the error, got: %v", runErr)
	}
	// Best-effort pin: the same suggestion is also hinted on stderr when
	// hints are enabled (kept for the stderr-hint branch, not for signal).
	if !strings.Contains(output, `"sqlite"`) {
		t.Fatalf("expected a did-you-mean hint, got:\n%s", output)
	}
}

// coverDryRunDiffWarns proves dry-run against a live DB prints the would-be
// plan AND the engine warnings without applying anything (sqlite type
// change: detected, warned, skipped).
func TestCoverDryRunDiffWarns(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")
	v1Path := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	var runErr error
	_ = captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, v1Path})
	})
	if runErr != nil {
		t.Fatalf("runDBMigrate (v1): %v", runErr)
	}

	v2Path := writeZeverFixture(t, dir, "schema.zen", migrateSchemaTypeChange)
	output := captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, "--dry-run", v2Path})
	})
	if runErr != nil {
		t.Fatalf("runDBMigrate (dry-run type change): %v", runErr)
	}
	if !strings.Contains(output, "warning: type change detected on orders.total_cents") {
		t.Fatalf("expected the type-change warning, got:\n%s", output)
	}

	// Dry-run applied nothing: re-running the v1 apply is a no-op.
	_ = captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, v1Path})
	})
	if runErr != nil {
		t.Fatalf("runDBMigrate (v1 again): %v", runErr)
	}
}

// coverMigratePromptPlaceholders pins the per-adapter DSN placeholders:
// postgres and mysql take distinct branches when the DSN prompt runs.
func TestCoverMigratePromptPlaceholders(t *testing.T) {
	for _, adapter := range []string{"postgres", "mysql"} {
		t.Run(adapter, func(t *testing.T) {
			dir := t.TempDir()
			schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)
			stubPromptTTY(t, true)
			setPromptInteractive(t)

			var gotPlaceholder string
			stubMigratePrompts(t,
				func(string, []string) (string, error) { return adapter, nil },
				func(_, placeholder string, _ func(string) error) (string, error) {
					gotPlaceholder = placeholder
					return "dummy-dsn", nil
				},
				func(string, []string) ([]string, error) { return nil, nil },
			)

			_ = runDBMigrate([]string{"--adapter=" + adapter, schemaPath})
			if !strings.Contains(gotPlaceholder, "postgres://") && !strings.Contains(gotPlaceholder, "tcp(") {
				t.Fatalf("adapter %q: unexpected DSN placeholder %q", adapter, gotPlaceholder)
			}
		})
	}
}

// poisonedCompileResult compiles migrateSchema then renames the first entity
// to an identifier the engine refuses, forcing migrate.Plan to fail after
// the database is already open.
func poisonedCompileResult(t *testing.T, dir string) *compile.Result {
	t.Helper()

	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)
	files, err := loadFiles([]string{schemaPath})
	if err != nil {
		t.Fatalf("loadFiles: %v", err)
	}
	result, diags := compile.Compile(files, atlas.New())
	if diags.HasErrors() {
		t.Fatal("valid schema failed to compile")
	}
	result.Schema.Modules[0].Entities[0].Name = `bad"name`

	return result
}

// coverPlanErrors pins both Plan failure returns (dry-run and apply) with a
// database that opens fine but whose plan the engine refuses.
func TestCoverPlanErrors(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")
	result := poisonedCompileResult(t, dir)

	if err := dryRunDiff("sqlite", dbPath, result, migrate.PlanOptions{}); err == nil {
		t.Fatal("expected a dry-run plan error, got nil")
	}
	if err := applyDiff("sqlite", dbPath, result.Schema, migrate.PlanOptions{}); err == nil {
		t.Fatal("expected an apply plan error, got nil")
	}
}

// coverApplyDiffApplyError pins the Apply failure return: schema_migrations
// is a full-column view over a missing table, so Ensure passes vacuously
// (CREATE TABLE IF NOT EXISTS is a no-op; PRAGMA sees every column) while
// the bookkeeping read fails.
func TestCoverApplyDiffApplyError(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	fullView := `CREATE VIEW schema_migrations AS SELECT 1 AS id, '' AS checksum, '' AS applied_at, '' AS kind, '' AS table_name, '' AS column_name, '' AS statement, '' AS prior_type, '' AS prior_name, '' AS object_name, '' AS prior_sql FROM zever_missing_table`
	openAndExec(t, dbPath, fullView)

	if err := runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, schemaPath}); err == nil {
		t.Fatal("expected an apply error over a view-shadowed ledger, got nil")
	}
}

// coverRollbackViewLedgerError pins the ledger-shadow failure: when
// schema_migrations is a view over a missing table, rollback surfaces an
// error instead of misreading the ledger. (The failure lands in
// EnsureMigrationsTable's introspection: PRAGMA executes the view body.)
func TestCoverRollbackViewLedgerError(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	fullView := `CREATE VIEW schema_migrations AS SELECT 1 AS id, '' AS checksum, '' AS applied_at, '' AS kind, '' AS table_name, '' AS column_name, '' AS statement, '' AS prior_type, '' AS prior_name, '' AS object_name, '' AS prior_sql FROM zever_missing_table`
	openAndExec(t, dbPath, fullView)

	if err := runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath}); err == nil {
		t.Fatal("expected a compute error over a view-shadowed ledger, got nil")
	}
}

// coverRollbackOverCount proves -n larger than the ledger rolls back
// everything recorded (newest first), skipping CREATE TABLE rows with
// warnings.
func TestCoverRollbackOverCount(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	migrateToV2(t, dbPath)

	var runErr error
	output := captureZeverStdout(t, func() {
		runErr = runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath, "-n", "999"})
	})
	if runErr != nil {
		t.Fatalf("runDBRollback over-count: %v", runErr)
	}
	if !strings.Contains(output, "rolled back 2 statement(s)") {
		t.Fatalf("expected the single ADD COLUMN undo, got:\n%s", output)
	}
	if cols := zeverSQLiteColumns(t, dbPath); cols["shipped"] {
		t.Fatalf("over-count rollback did not drop shipped, got %v", cols)
	}
}

// TestDevChildStopKillsWedged pins the SIGKILL fallback: a child ignoring
// SIGTERM is reaped past a short grace period.
func TestDevChildStopKillsWedged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals only")
	}

	// The shell ignores TERM; the sleeps die on it one at a time, so the
	// shell itself is still alive when the grace period expires.
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `trap '' TERM; sleep 30; sleep 30`)
	isolateProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start wedged child: %v", err)
	}

	child := &devChild{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(child.done)
	}()

	// Wait until the shell has armed its TERM trap instead of a fixed
	// sleep: the trap command runs before the first sleep, so a visible
	// sleep descendant proves setup completed. A TERM landing before
	// `trap` executes would kill the shell outright (graceful path),
	// making the kill-fallback branch flaky; with the trap armed, TERM
	// only kills the first sleep and the shell outlives the grace period
	// deterministically.
	pollFor(t, 10*time.Second, func() bool {
		//nolint:gosec // fixed ps argv, no shell; pid is our own test child.
		out, err := exec.CommandContext(t.Context(), "ps", "-o", "comm=", "--ppid", strconv.Itoa(cmd.Process.Pid)).Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), "sleep")
	})

	start := time.Now()
	child.stop(100 * time.Millisecond)
	elapsed := time.Since(start)
	if elapsed > 10*time.Second {
		t.Fatalf("stop took %v, SIGKILL fallback did not fire", elapsed)
	}
	if elapsed < 90*time.Millisecond {
		t.Fatalf("stop returned in %v without waiting out the grace period", elapsed)
	}
}

// TestCoverAddWatchTreeStatDenied pins the non-NotExist stat failure: a
// child of a chmod-000 directory fails with permission denied (uid 1000,
// so permissions are enforced).
func TestCoverAddWatchTreeStatDenied(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "locked")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	watcher := newTestWatcher(t)
	if err := addWatchTree(watcher, filepath.Join(parent, "child")); err == nil {
		t.Fatal("expected a stat error under a locked parent, got nil")
	} else if !strings.Contains(err.Error(), "stat") {
		t.Fatalf("expected a stat error, got: %v", err)
	}
}

// TestCoverAddWatchTreeWalkDenied pins the WalkDir failure: an unreadable
// subdirectory aborts the walk.
func TestCoverAddWatchTreeWalkDenied(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	watcher := newTestWatcher(t)
	if err := addWatchTree(watcher, root); err == nil {
		t.Fatal("expected a walk error over an unreadable subdir, got nil")
	}
}

func newTestWatcher(t *testing.T) *fsnotify.Watcher {
	t.Helper()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	t.Cleanup(func() { _ = watcher.Close() })

	return watcher
}

// openAndExec opens a sqlite DB at path, runs one statement, and closes it.
// Failure helper for ledger-shadowing setups (views over missing tables).
func openAndExec(t *testing.T, dbPath, stmt string) {
	t.Helper()
	ensureZeverDBAdapters()

	ctx := t.Context()
	conn, err := openZeverDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, stmt); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

// TestCoverTinkerDenylistLive proves the denylist branch executes: with a
// blocked import path injected into the symbol table, the entry is skipped
// (continue outer) and the source map is left unmutated (copy semantics).
// Sequential by design (no t.Parallel): stdlib.Symbols is a shared global,
// and sequential tests run exclusively of paused parallel tests.
func TestCoverTinkerDenylistLive(t *testing.T) {
	before := len(stdlib.Symbols)
	injected := map[string]map[string]reflect.Value{
		"os/exec/os_exec": {"Exec": reflect.ValueOf(1)},
		"plugin/plugin":   {"Open": reflect.ValueOf(1)},
	}
	for k, v := range injected {
		stdlib.Symbols[k] = v
	}
	t.Cleanup(func() {
		for k := range injected {
			delete(stdlib.Symbols, k)
		}
	})

	allowed := tinkerAllowedSymbols()
	for _, blocked := range []string{"os/exec/os_exec", "plugin/plugin"} {
		if _, ok := allowed[blocked]; ok {
			t.Fatalf("blocked %q survived the denylist", blocked)
		}
	}
	if len(stdlib.Symbols) != before+len(injected) {
		t.Fatal("tinkerAllowedSymbols must not mutate stdlib.Symbols")
	}
	if _, ok := stdlib.Symbols["os_exec"]; ok {
		t.Fatal("unexpected key shape in probe")
	}
}

// TestCoverNewTinkerInterpStdlibError pins the symbol-load failure return by
// injecting a slashless key, which yaegi rejects as missing a package name.
func TestCoverNewTinkerInterpStdlibError(t *testing.T) {
	stdlib.Symbols["zzbogus"] = map[string]reflect.Value{"X": reflect.ValueOf(1)}
	t.Cleanup(func() { delete(stdlib.Symbols, "zzbogus") })

	if _, err := newTinkerInterp(&tinkerClient{}); err == nil {
		t.Fatal("expected a stdlib-load error, got nil")
	}
}

// TestCoverNewTinkerInterpPreludeTimeout pins the prelude failure return by
// expiring the per-line context before evaluation starts.
func TestCoverNewTinkerInterpPreludeTimeout(t *testing.T) {
	orig := tinkerEvalTimeout
	tinkerEvalTimeout = time.Nanosecond
	t.Cleanup(func() { tinkerEvalTimeout = orig })

	if _, err := newTinkerInterp(&tinkerClient{}); err == nil {
		t.Fatal("expected a prelude error under an expired timeout, got nil")
	}
}

// tinkerFakeShim is a stdlib-only shim module speaking just enough protocol
// for the ping: one framed {"result":{}} per ping, then silence until EOF.
const tinkerFakeShim = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req struct {
			Verb string          ` + "`json:\"verb\"`" + `
			Args json.RawMessage ` + "`json:\"args,omitempty\"`" + `
		}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		if req.Verb == "ping" {
			fmt.Fprintln(out, "@@zever-tinker@@{\"result\":{}}")
			_ = out.Flush()
		}
	}
}
`

func writeTinkerShimModule(t *testing.T, dir, mainSrc string) {
	t.Helper()

	writeZeverFixture(t, dir, "go.mod", "module fakeshim\n\ngo 1.24\n")
	writeZeverFixture(t, filepath.Join(dir, "shim"), "main.go", mainSrc)
}

// TestCoverRunTinkerPingFailure pins the wedged-shim branch: the entry
// exists and `go run` starts, but the build fails, so the ping errors and
// the client is torn down.
func TestCoverRunTinkerPingFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes the go toolchain")
	}

	dir := t.TempDir()
	writeTinkerShimModule(t, dir, "package main\n\nfunc broken( {\n")
	t.Chdir(dir)

	if err := runTinker([]string{"--entry", "shim"}); err == nil {
		t.Fatal("expected a shim-start error, got nil")
	} else if !strings.Contains(err.Error(), "shim did not start") {
		t.Fatalf("expected a ping failure, got: %v", err)
	}
}

// TestCoverRunTinkerInterpFailure pins the interpreter-build error return
// inside runTinker: the shim answers the ping, but the prelude context is
// already expired, so the session tears down instead of opening the REPL.
func TestCoverRunTinkerInterpFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes the go toolchain")
	}

	dir := t.TempDir()
	writeTinkerShimModule(t, dir, tinkerFakeShim)
	t.Chdir(dir)

	orig := tinkerEvalTimeout
	tinkerEvalTimeout = time.Nanosecond
	t.Cleanup(func() { tinkerEvalTimeout = orig })

	if err := runTinker([]string{"--entry", "shim"}); err == nil {
		t.Fatal("expected an interpreter error, got nil")
	}
}

// TestCoverRunTinkerFullSession drives the whole REPL path against a fake
// shim: ping, interpreter build, banner, one :exit line, teardown.
func TestCoverRunTinkerFullSession(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes the go toolchain")
	}

	dir := t.TempDir()
	writeTinkerShimModule(t, dir, tinkerFakeShim)
	t.Chdir(dir)

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString(":exit\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = r.Close()
	})

	var runErr error
	var out, errOut string
	errOut = captureZeverStderr(t, func() {
		out = captureZeverStdout(t, func() {
			runErr = runTinker([]string{"--entry", "shim"})
		})
	})
	if runErr != nil {
		t.Fatalf("runTinker full session: %v\nstdout:\n%s\nstderr:\n%s", runErr, out, errOut)
	}
	if !strings.Contains(errOut, "development-only") {
		t.Fatalf("expected the dev warning on stderr, got:\n%s", errOut)
	}
}
