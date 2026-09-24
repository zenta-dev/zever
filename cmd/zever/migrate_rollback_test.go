package main

import (
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

// TestRollbackDialect pins the adapter-to-dialect mapping for inverse
// rendering.
func TestRollbackDialect(t *testing.T) {
	t.Parallel()

	for _, adapter := range []string{"sqlite", "postgres", "mysql"} {
		if _, err := rollbackDialect(adapter); err != nil {
			t.Errorf("rollbackDialect(%q): %v", adapter, err)
		}
	}

	if _, err := rollbackDialect("oracle"); err == nil {
		t.Error("rollbackDialect(oracle) = nil, want error")
	}
}

// TestRollbackOptionsRequiresDSN pins the --dsn requirement.
func TestRollbackOptionsRequiresDSN(t *testing.T) {
	t.Parallel()

	if _, err := rollbackOptions("sqlite", ""); err == nil {
		t.Error("sqlite without dsn = nil, want error")
	}

	opts, err := rollbackOptions("sqlite", " app.db ")
	if err != nil {
		t.Fatalf("sqlite options: %v", err)
	}

	if opts.Path != "app.db" {
		t.Fatalf("Path = %q, want trimmed %q", opts.Path, "app.db")
	}
}

// TestRunDBRollbackRejectsBadCount pins the -n guard.
func TestRunDBRollbackRejectsBadCount(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"-n", "0"}, {"-n", "-2"}} {
		if err := runDBRollback(args); err == nil {
			t.Fatalf("runDBRollback(%q) = nil, want error", args)
		}
	}
}

// TestRunDBRollbackDryRun prints inverses without executing: newest-first
// chain order means the single bootstrap batch rolls back as skips (CREATE
// TABLE has no inverse) and the database is untouched.
func TestRunDBRollbackSkipsCreateTable(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	var runErr error

	_ = captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, schemaPath})
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate: %v", runErr)
	}

	output := captureZeverStdout(t, func() {
		runErr = runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath, "-n", "100", "--dry-run"})
	})

	if runErr != nil {
		t.Fatalf("runDBRollback: %v", runErr)
	}

	// CREATE TABLE rows have no inverse: reported and skipped, never executed.
	if !strings.Contains(output, "leaving it recorded") {
		t.Fatalf("expected skip notices for CREATE TABLE rows, got:\n%s", output)
	}

	names := zeverSQLiteObjects(t, dbPath)
	if !names["users"] {
		t.Fatalf("dry-run rollback must not drop tables, got %v", names)
	}
}

// TestRunDBRollbackSkipsLegacyRows proves a pre-tracking row (checksum only,
// no metadata) is reported and left in place while genuinely reversible rows
// behind it still roll back.
func TestRunDBRollbackSkipsLegacyRows(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")
	schemaPath := writeZeverFixture(t, dir, "schema.zen", migrateSchema)

	var runErr error

	_ = captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, schemaPath})
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate: %v", runErr)
	}

	ctx := t.Context()

	conn, err := openZeverDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if _, execErr := conn.Exec(ctx, "INSERT INTO schema_migrations (checksum) VALUES ('fake-legacy-checksum')"); execErr != nil {
		t.Fatalf("insert legacy row: %v", execErr)
	}

	if closeErr := conn.Close(ctx); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	output := captureZeverStdout(t, func() {
		runErr = runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath, "-n", "100"})
	})

	if runErr != nil {
		t.Fatalf("runDBRollback: %v", runErr)
	}

	if !strings.Contains(output, "predates rollback tracking") {
		t.Fatalf("expected a warning about the legacy row, got:\n%s", output)
	}

	rows, err := readZeverMigrationChecksums(t, dbPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}

	found := false

	for _, sum := range rows {
		if sum == "fake-legacy-checksum" {
			found = true
		}
	}

	if !found {
		t.Fatalf("legacy row was removed from schema_migrations, want it left in place; rows: %v", rows)
	}
}

// readZeverMigrationChecksums returns every checksum currently recorded in
// schema_migrations.
func readZeverMigrationChecksums(t *testing.T, path string) ([]string, error) {
	t.Helper()

	ctx := t.Context()

	conn, err := openZeverDB("sqlite", path)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = conn.Close(ctx)
	}()

	rows, err := conn.Query(ctx, "SELECT checksum FROM schema_migrations")
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = rows.Close()
	}()

	var out []string

	for rows.Next() {
		var sum string
		if err := rows.Scan(&sum); err != nil {
			return nil, err
		}

		out = append(out, sum)
	}

	return out, nil
}

// TestRunDBRollbackBadFlag proves unknown flags surface as errors.
func TestRunDBRollbackBadFlag(t *testing.T) {
	if err := runDBRollback([]string{"--bogus-flag"}); err == nil {
		t.Fatal("expected a flag error, got nil")
	}
}

// TestRunDBRollbackMissingDSN pins the --dsn requirement before any DB work.
func TestRunDBRollbackMissingDSN(t *testing.T) {
	if err := runDBRollback([]string{"--adapter=sqlite"}); err == nil {
		t.Fatal("expected a missing-dsn error, got nil")
	}
}

// TestRunDBRollbackHelp proves -h prints usage (including the data-loss
// warning) to stderr and returns flag.ErrHelp without touching a database.
func TestRunDBRollbackHelp(t *testing.T) {
	var runErr error

	output := captureZeverStderr(t, func() {
		runErr = runDBRollback([]string{"-h"})
	})

	if !errors.Is(runErr, flag.ErrHelp) {
		t.Fatalf("runDBRollback -h error = %v, want flag.ErrHelp", runErr)
	}

	if !strings.Contains(output, "ROLLING BACK CAN DESTROY DATA") {
		t.Fatalf("help output missing data-loss warning:\n%s", output)
	}
}

// TestRollbackOptionsAdapters pins the per-adapter option mapping.
func TestRollbackOptionsAdapters(t *testing.T) {
	t.Parallel()

	opts, err := rollbackOptions("postgres", "postgres://localhost/db")
	if err != nil {
		t.Fatalf("postgres options: %v", err)
	}

	if opts.DSN != "postgres://localhost/db" {
		t.Fatalf("DSN = %q", opts.DSN)
	}

	if _, err := rollbackOptions("mysql", "u:p@tcp(h)/d"); err == nil {
		t.Error("mysql = nil, want gap error (no adapter registered)")
	}
}

// TestResolveRollbackConfig pins the XConfig mapping.
func TestResolveRollbackConfig(t *testing.T) {
	t.Parallel()

	cfg := resolveRollbackConfig("sqlite", "app.db", 3, true)

	if cfg.Adapter != "sqlite" || cfg.DSN != "app.db" || cfg.Count != 3 || !cfg.DryRun {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

// TestRunDBRollbackUnknownAdapter pins the dialect error before any DB work.
func TestRunDBRollbackUnknownAdapter(t *testing.T) {
	if err := runDBRollback([]string{"--adapter=oracle", "--dsn=x"}); err == nil {
		t.Fatal("expected a dialect error, got nil")
	}
}

// TestOpenRollbackDBFailures pins both the adapter and open error branches.
func TestOpenRollbackDBFailures(t *testing.T) {
	ensureZeverDBAdapters()

	if _, err := openRollbackDB("oracle", "x"); err == nil {
		t.Fatal("expected an adapter error, got nil")
	}

	badPath := filepath.Join(t.TempDir(), "no-such-dir", "x.db")
	if _, err := openRollbackDB("sqlite", badPath); err == nil {
		t.Fatal("expected an open error, got nil")
	}
}

// TestRunDBRollbackEmptyTable proves rolling back with no recorded rows
// reports zero without error.
func TestRunDBRollbackEmptyTable(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	output := captureZeverStdout(t, func() {
		if err := runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath, "-n", "1"}); err != nil {
			t.Fatalf("runDBRollback: %v", err)
		}
	})

	if !strings.Contains(output, "rolled back 0 statement(s)") {
		t.Fatalf("expected zero rollback, got:\n%s", output)
	}
}

// migrateSchemaV2 is migrateSchema with one extra column on Order
// ("shipped: bool"), used to exercise the add-only ALTER TABLE path and its
// rollback against a database already migrated from migrateSchema.
const migrateSchemaV2 = `entity User {
	id: uuid @primary
	email: string @unique
}

entity Order {
	id: uuid @primary
	user_id: uuid
	total_cents: int64
	shipped: bool

	belongs_to user: User @foreign_key(user_id) @on_delete(cascade)

	index(user_id)
}
`

// migrateToV2 applies migrateSchema then migrateSchemaV2 to dbPath, proving
// the second run applies exactly the one ADD COLUMN statement.
func migrateToV2(t *testing.T, dbPath string) {
	t.Helper()

	dir := filepath.Dir(dbPath)
	v1Path := writeZeverFixture(t, dir, "schema-v1.zen", migrateSchema)

	var runErr error

	_ = captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, v1Path})
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate (v1): %v", runErr)
	}

	v2Path := writeZeverFixture(t, dir, "schema-v2.zen", migrateSchemaV2)

	output := captureZeverStdout(t, func() {
		runErr = runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dbPath, v2Path})
	})

	if runErr != nil {
		t.Fatalf("runDBMigrate (v2): %v", runErr)
	}

	if !strings.Contains(output, "applied 1 statement(s)") {
		t.Fatalf("expected exactly one applied statement, got: %s", output)
	}

	if cols := zeverSQLiteColumns(t, dbPath); !cols["shipped"] {
		t.Fatalf("precondition failed: orders should have %q, got %v", "shipped", cols)
	}
}

// TestRunDBRollbackDryRunPlansInverse proves --dry-run prints the inverse
// without executing: newest-first order means the single ADD COLUMN row
// plans a DROP COLUMN plus its bookkeeping DELETE.
func TestRunDBRollbackDryRunPlansInverse(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	migrateToV2(t, dbPath)

	var runErr error

	output := captureZeverStdout(t, func() {
		runErr = runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath, "-n", "1", "--dry-run"})
	})

	if runErr != nil {
		t.Fatalf("runDBRollback: %v", runErr)
	}

	if !strings.Contains(output, `DROP COLUMN "shipped"`) {
		t.Fatalf("expected the DROP COLUMN inverse in the plan, got:\n%s", output)
	}

	// Dry-run executes nothing: the column and its tracking row survive.
	if cols := zeverSQLiteColumns(t, dbPath); !cols["shipped"] {
		t.Fatalf("dry-run must not drop the column, got %v", cols)
	}
}

// TestRunDBRollbackReversesLastAddColumn proves rolling back an ADD COLUMN
// drops that column from the live schema and deletes its schema_migrations
// row, leaving every other column untouched.
func TestRunDBRollbackReversesLastAddColumn(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	migrateToV2(t, dbPath)

	var runErr error

	output := captureZeverStdout(t, func() {
		runErr = runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath, "-n", "1"})
	})

	if runErr != nil {
		t.Fatalf("runDBRollback: %v", runErr)
	}

	// Rolling back one row emits two statements (the inverse DDL, then the
	// DELETE removing that row from schema_migrations).
	if !strings.Contains(output, "rolled back 2 statement(s)") {
		t.Fatalf("expected two statements (inverse + delete), got: %s", output)
	}

	cols := zeverSQLiteColumns(t, dbPath)
	if cols["shipped"] {
		t.Fatalf("rollback did not drop %q, got %v", "shipped", cols)
	}

	for _, want := range []string{"id", "user_id", "total_cents"} {
		if !cols[want] {
			t.Fatalf("rollback disturbed unrelated column %q, got %v", want, cols)
		}
	}
}

// TestRunDBRollbackComputeFailure pins the engine error branch: a
// schema_migrations VIEW shadows the tracking table, so EnsureMigrationsTable
// passes vacuously while ComputeRollback cannot read rows.
func TestRunDBRollbackComputeFailure(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	ctx := t.Context()

	conn, err := openZeverDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if _, execErr := conn.Exec(ctx, "CREATE VIEW schema_migrations AS SELECT 1 AS id"); execErr != nil {
		t.Fatalf("create view: %v", execErr)
	}

	if closeErr := conn.Close(ctx); closeErr != nil {
		t.Fatalf("close: %v", err)
	}

	if err := runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath}); err == nil {
		t.Fatal("expected a compute error, got nil")
	}
}

// TestRunDBRollbackEnsureFailure pins the bookkeeping error branch: the DSN
// names a directory, so opening is lazy but the first statement fails.
func TestRunDBRollbackEnsureFailure(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()

	if err := runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dir}); err == nil {
		t.Fatal("expected an ensure error, got nil")
	}
}

// TestRunDBRollbackApplyFailure pins the apply error branch: the recorded
// ADD COLUMN inverse targets a table dropped after migrating.
func TestRunDBRollbackApplyFailure(t *testing.T) {
	ensureZeverDBAdapters()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	migrateToV2(t, dbPath)

	ctx := t.Context()

	conn, err := openZeverDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if _, execErr := conn.Exec(ctx, `DROP TABLE "orders"`); execErr != nil {
		t.Fatalf("drop orders: %v", execErr)
	}

	if closeErr := conn.Close(ctx); closeErr != nil {
		t.Fatalf("close: %v", err)
	}

	if err := runDBRollback([]string{"--adapter=sqlite", "--dsn=" + dbPath, "-n", "1"}); err == nil {
		t.Fatal("expected an apply error, got nil")
	}
}
