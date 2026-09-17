package main

import (
	"context"
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/compile"
	"github.com/zenta-dev/zever/orm/migrate"
)

// migrateSchema is a small two-entity schema with a primary key, a unique
// column, a foreign key, and a declared index -- enough to exercise every
// statement kind the migrate path emits.
const migrateSchema = `entity User {
	id: uuid @primary
	email: string @unique
}

entity Order {
	id: uuid @primary
	user_id: uuid
	total_cents: int64

	belongs_to user: User @foreign_key(user_id) @on_delete(cascade)

	index(user_id)
}
`

func TestRunDBMigrateNoFiles(t *testing.T) {
	if err := runDBMigrate(nil); err == nil {
		t.Fatalf("expected error when no input files given, got nil")
	}
}

func TestRunDBMigrateUnknownAdapter(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	err := runDBMigrate([]string{"--adapter=oracle", "--dry-run", schemaPath})
	if err == nil {
		t.Fatalf("expected error for unsupported adapter, got nil")
	}
}

func TestRunDBMigrateCompileError(t *testing.T) {
	dir := t.TempDir()
	broken := `entity Broken {
		id: uuid @primary
		bad_field: nonexistent_type
	}`
	schemaPath := writeZeverFixture(t, dir, "broken.zen", broken)

	if err := runDBMigrate([]string{"--dry-run", schemaPath}); err == nil {
		t.Fatalf("expected error for broken schema, got nil")
	}
}

// TestMigrateDialect pins the adapter-to-dialect mapping, including the
// closest-match suggestion on typos.
func TestMigrateDialect(t *testing.T) {
	t.Parallel()

	for _, adapter := range []string{"sqlite", "postgres", "mysql"} {
		if _, err := migrateDialect(adapter); err != nil {
			t.Errorf("migrateDialect(%q): %v", adapter, err)
		}
	}

	if _, err := migrateDialect("oracle"); err == nil {
		t.Error("migrateDialect(oracle) = nil, want error")
	}

	if _, err := migrateDialect("sqllite"); err == nil {
		t.Error("migrateDialect(sqllite) = nil, want error with suggestion")
	} else if !strings.Contains(err.Error(), "sqlite") {
		t.Errorf("migrateDialect(sqllite) error should suggest sqlite, got: %v", err)
	}
}

// TestMigrateOptionsRequiresDSN pins the per-adapter DSN requirements. MySQL
// has no registered adapter in zever yet, so it fails with a gap error even
// though the dialect mapping knows it.
func TestMigrateOptionsRequiresDSN(t *testing.T) {
	t.Parallel()

	if _, err := migrateOptions("sqlite", ""); err == nil {
		t.Error("sqlite without dsn = nil, want error")
	}

	if _, err := migrateOptions("postgres", ""); err == nil {
		t.Error("postgres without dsn = nil, want error")
	}

	if _, err := migrateOptions("mysql", "u:p@tcp(h)/d"); err == nil {
		t.Error("mysql = nil, want gap error (no adapter registered)")
	}

	if _, err := migrateOptions("oracle", "x"); err == nil {
		t.Error("oracle = nil, want error")
	}

	opts, err := migrateOptions("sqlite", " app.db ")
	if err != nil {
		t.Fatalf("sqlite options: %v", err)
	}

	if opts.Path != "app.db" {
		t.Fatalf("Path = %q, want trimmed %q", opts.Path, "app.db")
	}
}

// TestRunDBMigrateDryRun proves --dry-run prints both the Atlas HCL and the
// DDL and never opens a database (no --dsn is supplied, which the apply path
// would reject).
func TestRunDBMigrateDryRun(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	var runErr error

	output := captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--dry-run", schemaPath})
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate --dry-run: %v", runErr)
	}

	for _, want := range []string{
		"-- schema.hcl",
		`table "users" {`,
		`foreign_key "orders_user_id_fkey" {`,
		"-- DDL",
		`CREATE TABLE IF NOT EXISTS "users"`,
		`CREATE TABLE IF NOT EXISTS "orders"`,
		`CREATE INDEX IF NOT EXISTS "orders_user_id_idx"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, output)
		}
	}

	// No database file was named, so none may exist.
	matches, err := filepath.Glob(filepath.Join(dir, "*.db"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(matches) != 0 {
		t.Fatalf("dry-run created database files: %v", matches)
	}
}

func TestRunDBMigrateRequiresDSN(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	if err := runDBMigrate([]string{"--adapter=sqlite", schemaPath}); err == nil {
		t.Fatalf("expected error when --dsn is missing, got nil")
	}
}

// TestRunDBMigrateFlagsAfterPositional proves flags parse after positionals
// via flexibleParse, using --dry-run with a live DSN so the diff path runs
// without executing anything.
func TestRunDBMigrateFlagsAfterPositional(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)
	dbPath := filepath.Join(dir, "test.db")

	cases := []struct {
		name string
		args []string
	}{
		{"file-first flags after", []string{schemaPath, "--adapter=sqlite", "--dsn=" + dbPath, "--dry-run"}},
		{"flags-first file after", []string{"--adapter=sqlite", "--dsn=" + dbPath, "--dry-run", schemaPath}},
		{"interleaved", []string{schemaPath, "--dry-run", "--adapter=sqlite", "--dsn=" + dbPath}},
		{"file first with separate value flags", []string{schemaPath, "--adapter", "sqlite", "--dsn", dbPath, "--dry-run"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interactiveMode = false

			var runErr error

			output := captureZeverStdout(t, func() {
				runErr = runDBMigrate(tc.args)
			})

			if runErr != nil {
				t.Fatalf("runDBMigrate %q: %v", tc.name, runErr)
			}

			if !strings.Contains(output, "-- DDL") {
				t.Fatalf("expected dry-run DDL output for %q, got:\n%s", tc.name, output)
			}
		})
	}
}

// TestRunDBMigrateAppliesToSQLite runs a real migration against a temp sqlite
// file and asserts the tables and index exist, then re-runs to prove
// checksum idempotency makes the second run a no-op.
func TestRunDBMigrateAppliesToSQLite(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)
	dbPath := filepath.Join(dir, "migrate.db")

	args := []string{"--adapter=sqlite", "--dsn=" + dbPath, schemaPath}

	var runErr error

	_ = captureZeverStdout(t, func() {
		runErr = runDBMigrate(args)
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate: %v", runErr)
	}

	names := zeverSQLiteObjects(t, dbPath)

	for _, want := range []string{"users", "orders", "orders_user_id_idx"} {
		if !names[want] {
			t.Fatalf("sqlite_master missing %q, got %v", want, names)
		}
	}

	output := captureZeverStdout(t, func() {
		runErr = runDBMigrate(args)
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate (second run): %v", runErr)
	}

	if !strings.Contains(output, "applied 0 statement(s)") {
		t.Fatalf("expected idempotent re-run to apply nothing, got: %s", output)
	}
}

// TestRunDBMigrateDryRunWithDBReportsWithoutApplying proves --dry-run against
// a live database prints the would-be statements without executing or
// recording anything.
func TestRunDBMigrateDryRunWithDBReportsWithoutApplying(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)
	dbPath := filepath.Join(dir, "migrate.db")

	var runErr error

	output := captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, "--dry-run", schemaPath})
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate (dry-run): %v", runErr)
	}

	if !strings.Contains(output, `CREATE TABLE IF NOT EXISTS "users"`) {
		t.Fatalf("dry-run output missing the would-be DDL:\n%s", output)
	}

	names := zeverSQLiteObjects(t, dbPath)
	if names["users"] {
		t.Fatalf("dry-run must not create tables, got %v", names)
	}
}

// zeverSQLiteObjects returns the set of table and index names in a sqlite file.
func zeverSQLiteObjects(t *testing.T, path string) map[string]bool {
	t.Helper()

	ctx := context.Background()

	conn, err := openZeverDB("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	defer func() {
		_ = conn.Close(ctx)
	}()

	rows, err := conn.Query(ctx, "SELECT name FROM sqlite_master WHERE type IN ('table','index')")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}

	defer func() {
		_ = rows.Close()
	}()

	out := map[string]bool{}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}

		out[name] = true
	}

	return out
}

// zeverSQLiteColumns returns the set of column names for the orders table.
func zeverSQLiteColumns(t *testing.T, path string) map[string]bool {
	t.Helper()

	ctx := context.Background()

	conn, err := openZeverDB("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	defer func() {
		_ = conn.Close(ctx)
	}()

	rows, err := conn.Query(ctx, "SELECT name FROM pragma_table_info(?)", "orders")
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}

	defer func() {
		_ = rows.Close()
	}()

	out := map[string]bool{}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}

		out[name] = true
	}

	return out
}

var _ = db.SQLite

// TestRunDBMigrateHelp proves -h prints usage to stderr and returns
// flag.ErrHelp without touching the filesystem.
func TestRunDBMigrateHelp(t *testing.T) {
	var runErr error

	output := captureZeverStderr(t, func() {
		runErr = runDBMigrate([]string{"-h"})
	})

	if !errors.Is(runErr, flag.ErrHelp) {
		t.Fatalf("runDBMigrate -h error = %v, want flag.ErrHelp", runErr)
	}

	if !strings.Contains(output, "zever db migrate") {
		t.Fatalf("help output missing command name:\n%s", output)
	}
}

// TestRunDBMigrateBadFlag proves unknown flags surface as errors.
func TestRunDBMigrateBadFlag(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	if err := runDBMigrate([]string{"--bogus-flag", schemaPath}); err == nil {
		t.Fatal("expected a flag error, got nil")
	}
}

// TestFlagWasSet pins the explicit-set detection used for interactive defaulting.
func TestFlagWasSet(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	adapter := fs.String("adapter", "sqlite", "adapter")
	_ = adapter

	if flagWasSet(fs, "adapter") {
		t.Fatal("unset flag must report false")
	}

	if err := fs.Parse([]string{"--adapter", "postgres"}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !flagWasSet(fs, "adapter") {
		t.Fatal("set flag must report true")
	}

	if flagWasSet(fs, "missing") {
		t.Fatal("unknown flag must report false")
	}
}

// TestMigrateOptionsPostgres pins the DSN mapping for postgres.
func TestMigrateOptionsPostgres(t *testing.T) {
	t.Parallel()

	opts, err := migrateOptions("postgres", "postgres://localhost/db")
	if err != nil {
		t.Fatalf("postgres options: %v", err)
	}

	if opts.DSN != "postgres://localhost/db" {
		t.Fatalf("DSN = %q", opts.DSN)
	}
}

// TestOpenZeverDBRejectsUnknownAdapter pins the suggestion on typos.
func TestOpenZeverDBRejectsUnknownAdapter(t *testing.T) {
	ensureZeverDBAdapters()

	if _, err := openZeverDB("sqllite", "x"); err == nil {
		t.Fatal("expected an error, got nil")
	} else if !strings.Contains(err.Error(), "sqlite") {
		t.Fatalf("error should suggest sqlite, got: %v", err)
	}
}

// TestOpenZeverDBReportsOpenFailure pins driver failures with context.
func TestOpenZeverDBReportsOpenFailure(t *testing.T) {
	ensureZeverDBAdapters()

	badPath := filepath.Join(t.TempDir(), "no-such-dir", "x.db")
	if _, err := openZeverDB("sqlite", badPath); err == nil {
		t.Fatal("expected an open error, got nil")
	}
}

// TestPrintDryRunEmpty pins the already-in-sync rendering.
func TestPrintDryRunEmpty(t *testing.T) {
	output := captureZeverStdout(t, func() {
		printDryRun(map[string][]byte{"schema.hcl": []byte("table")}, nil)
	})

	if !strings.Contains(output, "already in sync") {
		t.Fatalf("expected in-sync message, got:\n%s", output)
	}
}

// migrateSchemaTypeChange flips Order.total_cents from int64 (INTEGER) to
// string (TEXT): a genuine affinity-visible type change.
const migrateSchemaTypeChange = `entity User {
	id: uuid @primary
	email: string @unique
}

entity Order {
	id: uuid @primary
	user_id: uuid
	total_cents: string

	belongs_to user: User @foreign_key(user_id) @on_delete(cascade)

	index(user_id)
}
`

// TestRunDBMigrateSQLiteTypeChangeWarns proves the sqlite limitation is real:
// a detected type change warns and applies nothing.
func TestRunDBMigrateSQLiteTypeChangeWarns(t *testing.T) {
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
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, v2Path})
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate (type change): %v", runErr)
	}

	for _, want := range []string{"warning: type change detected on orders.total_cents", "applied 0 statement(s)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q, got:\n%s", want, output)
		}
	}
}

// TestResolveMigrateConfig pins the XConfig mapping.
func TestResolveMigrateConfig(t *testing.T) {
	t.Parallel()

	cfg := resolveMigrateConfig("sqlite", "app.db", []string{"a.zen"}, true, true)

	if cfg.Adapter != "sqlite" || cfg.DSN != "app.db" || !cfg.DryRun || !cfg.DropColumns || len(cfg.Files) != 1 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

// TestRunDBMigrateMySQLEngineGap pins the documented gap: the dialect and
// engine know mysql, but no zever db adapter is registered for it.
func TestRunDBMigrateMySQLEngineGap(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	err := runDBMigrate([]string{"--adapter=mysql", "--dsn=u:p@tcp(h)/d", schemaPath})
	if err == nil {
		t.Fatal("expected a mysql gap error, got nil")
	}

	if !strings.Contains(err.Error(), "no registered zever db adapter") {
		t.Fatalf("expected a gap error, got: %v", err)
	}
}

// TestMigrateOptionsSuggestsClosest pins the typo suggestion.
func TestMigrateOptionsSuggestsClosest(t *testing.T) {
	if _, err := migrateOptions("sqllite", "x"); err == nil {
		t.Fatal("expected an error, got nil")
	} else if !strings.Contains(err.Error(), "sqlite") {
		t.Fatalf("error should suggest sqlite, got: %v", err)
	}

	if _, err := migrateOptions("xyz", "x"); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestOpenZeverDBUnknownWithoutSuggestion pins the suggestion-free branch.
func TestOpenZeverDBUnknownWithoutSuggestion(t *testing.T) {
	ensureZeverDBAdapters()

	if _, err := openZeverDB("xyz", "x"); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestDryRunDiffOpenFailure pins the connection error branch directly.
func TestDryRunDiffOpenFailure(t *testing.T) {
	ensureZeverDBAdapters()

	result := &compile.Result{Schema: nil, Outputs: map[string]map[string][]byte{}}

	if err := dryRunDiff("oracle", "x", result, migrate.PlanOptions{}); err == nil {
		t.Fatal("expected an open error, got nil")
	}
}
