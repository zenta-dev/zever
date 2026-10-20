package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/dsl/backend/atlas"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/orm/migrate"
)

// migrateHelp is appended to the flag usage of `zever db migrate`. It
// states the command's scope plainly: a create-only bootstrap plus a
// column-level diff — not Atlas-style migration planning.
//
// The actual diffing/apply engine lives in orm/migrate; this file is a thin
// CLI wrapper over its Plan/Apply API -- flag parsing, prompts, and output
// formatting only. See orm/migrate's package doc for the engine's own scope
// statement (dialect support, FTS5 status, etc).
const migrateHelp = `Compiles the given .zen files with the atlas backend, creates any tables
that don't exist yet in the target database, and diffs an already-existing
table's columns, indexes, unique constraints, foreign keys, and nullability
against what the schema declares.

For each declared table:
  * table does not exist yet  -> CREATE TABLE/INDEX IF NOT EXISTS (bootstrap)
  * table already exists      -> RENAME COLUMN for any field declaring
                                   @renamed_from("old_name"),
                                   ALTER TABLE ADD COLUMN for any schema
                                   column missing from the live table,
                                   ALTER COLUMN TYPE for any column whose
                                   type changed (postgres) / MODIFY COLUMN
                                   (mysql; sqlite warns and skips),
                                   ALTER COLUMN [SET|DROP] NOT NULL for any
                                   column whose nullability no longer matches
                                   (postgres) / MODIFY COLUMN (mysql; sqlite
                                   warns and skips),
                                   CREATE INDEX/CREATE UNIQUE INDEX for any
                                   declared index(...) block or @unique field
                                   missing live, DROP INDEX for any live
                                   index this tool recognizes as its own that
                                   is no longer declared,
                                   ALTER TABLE ADD CONSTRAINT ... FOREIGN KEY
                                   for any belongs_to/has_one relation
                                   missing live (postgres and mysql; sqlite
                                   warns and skips), and
                                   DROP COLUMN for any live column no longer
                                   declared -- but only under --drop-columns

RENAMING A COLUMN requires saying so in the schema. A rename is
indistinguishable from "drop the old column, add a new one" by comparing a
live database against a schema, so annotate the new field with
@renamed_from("old_name") and the migration emits one RENAME COLUMN --
preserving the data -- instead of an ADD (and, under --drop-columns, a DROP).
The annotation is harmless to leave in place: once applied, it is a no-op.

Every executed statement's checksum is recorded in a schema_migrations
table so re-running is a no-op for statements already applied, rather than
relying solely on each dialect's own "IF NOT EXISTS" support (ALTER TABLE
ADD COLUMN has no such support in SQLite). Alongside the checksum, enough
metadata is recorded (kind, table, column, prior type/name, object name,
prior index SQL) for "zever db rollback" to synthesize an inverse statement
later -- see "zever db rollback -h", and read its data-loss warnings before
using it.

DROPPING COLUMNS is off by default and enabled only by --drop-columns.
A DROP COLUMN destroys that column's data irrecoverably; this tool will
never emit one unless you ask for it explicitly. Dropping an INDEX is, by
contrast, always on: it destroys no data, only a query-performance or
uniqueness guarantee, and is limited to indexes matching this tool's own
naming convention so a hand-added DBA index is never touched.

LIMITATIONS — this is NOT full Atlas migration support:

  * SQLite type changes AND nullability changes are DETECTED AND WARNED
    ABOUT, never applied: SQLite has no ALTER COLUMN TYPE or ALTER COLUMN
    [SET|DROP] NOT NULL statement at all, and the copy-table rebuild that
    would emulate either is deliberately not attempted here
  * SQLite cannot ADD a foreign key constraint to an existing table under
    any syntax -- a missing declared foreign key is DETECTED AND WARNED
    ABOUT on sqlite, and only actually added on postgres and mysql
  * a live unique constraint or foreign key no longer declared is reported
    with a warning but never dropped automatically, on any dialect --
    distinguishing a genuine unique/foreign-key CONSTRAINT from a plain
    index needs additional catalog introspection this tool does not add,
    and an automatic DROP CONSTRAINT is a bigger commitment than DROP INDEX
  * no AUTOMATIC rename detection: an unannotated rename looks exactly like
    "drop old + add new" and will surface as an ADD COLUMN for the new name,
    with the old column left alone or, under --drop-columns, dropped along
    with its data -- use @renamed_from("old_name") to get a real RENAME
  * no default-value diffing (only names, types, nullability, indexes,
    unique constraints, and foreign keys are compared)
  * rollback ("zever db rollback") inverts recorded statements only,
    best-effort and lossy for some kinds -- it is not a down-migration system

SAFETY — writing a safe migration is an expand/contract exercise, and
--lint checks the plan for the known-unsafe shapes before anything runs:

  * DROP COLUMN / DROP TABLE (error): destroys data. Expand first (add the
    replacement, dual-write, backfill, move reads), then drop.
  * ADD COLUMN ... NOT NULL without DEFAULT (error): fails on non-empty
    tables. Add as nullable, backfill, then SET NOT NULL.
  * ALTER COLUMN TYPE (error): rewrites the table. Add the new column,
    backfill, swap reads, drop the old one.
  * CREATE INDEX without CONCURRENTLY on postgres (warning): blocks writes.
    Pass --concurrent-indexes to build without blocking (each build runs
    outside a transaction, since postgres forbids CONCURRENTLY inside one).

--lock-timeout/--statement-timeout bound every applied statement on
postgres via SET LOCAL inside the statement's own transaction (e.g.
--lock-timeout=10s --statement-timeout=30s); unset flags fall back to
db.options lock_timeout/statement_timeout from the config file. Waive one
rule on one statement with a '-- zever:allow:<code>' comment, or every rule
with a bare '-- zever:allow'.

Driving the real Atlas engine for full diff-based migrations is deferred.`

var migrateAdapters = []string{"sqlite", "postgres", "mysql"}

// dbAdaptersOnce registers the database adapters the CLI can open. The
// container package owns the full registration as the composition root; the
// CLI registers just the two adapters its commands can address so `zever db`
// works without booting a container. Duplicate errors are ignored, matching
// the container's convention.
var dbAdaptersOnce sync.Once

// ensureZeverDBAdapters registers the sqlite and postgres factories.
func ensureZeverDBAdapters() {
	dbAdaptersOnce.Do(func() {
		_ = db.Register(db.SQLite, dbsqlite.New)
		_ = db.Register(db.Postgres, dbpostgres.New)
	})
}

// MigrateConfig is the resolved configuration for one `zever db migrate` run.
type MigrateConfig struct {
	// Adapter names the database backend (sqlite, postgres, or mysql).
	Adapter string
	// DSN is the connection string: a file path for sqlite, a libpq DSN for
	// postgres, a go-sql-driver DSN for mysql.
	DSN string
	// Files are the .zen schema paths to compile and apply.
	Files []string
	// DryRun prints the plan without executing or recording anything.
	DryRun bool
	// DropColumns enables emitting DROP COLUMN for undeclared live columns.
	// Destructive: the dropped data is unrecoverable.
	DropColumns bool
	// Lint gates the run on the migration safety linter: error-severity
	// findings (drops, NOT NULL without default, type rewrites) fail the
	// run before anything is applied.
	Lint bool
	// LockTimeout caps how long one applied statement may wait for a lock
	// on postgres, via SET LOCAL inside the statement's own transaction.
	LockTimeout time.Duration
	// StatementTimeout caps how long one applied statement may run on
	// postgres, via SET LOCAL inside the statement's own transaction.
	StatementTimeout time.Duration
	// ConcurrentIndexes builds postgres indexes with CREATE INDEX
	// CONCURRENTLY (no write block; each build runs outside a transaction).
	ConcurrentIndexes bool
}

// MigrateSafetyOptions groups the safety-related migrate settings, so adding
// a new one does not mean adding another positional bool to
// resolveMigrateConfig.
type MigrateSafetyOptions struct {
	// Lint gates the run on the migration safety linter.
	Lint bool
	// LockTimeout caps one statement's lock wait on postgres (SET LOCAL).
	LockTimeout time.Duration
	// StatementTimeout caps one statement's run time on postgres (SET LOCAL).
	StatementTimeout time.Duration
	// ConcurrentIndexes builds postgres indexes without blocking writes.
	ConcurrentIndexes bool
}

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seams keep behavior identical while letting tests stub success/error
// paths for 100% statement coverage.
var (
	promptSelectForMigrate      = promptSelect
	promptInputForMigrate       = promptInput
	promptMultiSelectForMigrate = promptMultiSelect
)

// resolveMigrateSafetyOptions builds a MigrateSafetyOptions from parsed flag
// values without side effects.
func resolveMigrateSafetyOptions(lint bool, lock, stmt time.Duration, concurrent bool) MigrateSafetyOptions {
	return MigrateSafetyOptions{Lint: lint, LockTimeout: lock, StatementTimeout: stmt, ConcurrentIndexes: concurrent}
}

// resolveMigrateConfig builds a MigrateConfig from parsed flag values without
// side effects.
func resolveMigrateConfig(adapter, dsn string, files []string, dryRun, dropColumns bool, safety MigrateSafetyOptions) MigrateConfig {
	return MigrateConfig{
		Adapter: adapter, DSN: dsn, Files: files, DryRun: dryRun, DropColumns: dropColumns,
		Lint: safety.Lint, LockTimeout: safety.LockTimeout, StatementTimeout: safety.StatementTimeout,
		ConcurrentIndexes: safety.ConcurrentIndexes,
	}
}

func printMigrateUsage(fs *flag.FlagSet) {
	header := title("zever db migrate") + dim(" — schema → live database")
	usage := bold("Usage:") + "  " + cmd("zever db migrate") + dim(" [flags…] <files…>") + dim("  •  -i/--interactive for guided prompts")

	printBoxedUsage(fs, header, usage, migrateHelp, []usageExample{
		{command: "zever db migrate schema/app.zen --adapter=sqlite --dsn=data/app.db"},
		{command: "zever db migrate --dry-run schema/*.zen --adapter=postgres --dsn='postgres://localhost/db?sslmode=disable'"},
		{command: "zever db migrate --lint --dry-run schema/*.zen --adapter=postgres --dsn='postgres://localhost/db?sslmode=disable'"},
		{command: "zever db migrate schema/app.zen --adapter=postgres --dsn='postgres://localhost/db?sslmode=disable' --concurrent-indexes --lock-timeout=10s --statement-timeout=30s"},
		{command: "zever db migrate -i", comment: "  # guided: select files, adapter & DSN"},
	}, "use --dry-run to preview DDL without executing")
}

func runDBMigrate(args []string) error {
	args = peelInteractive(args)
	fs := flag.NewFlagSet("db migrate", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print the generated Atlas HCL and DDL to stdout without executing anything; with --dsn it connects read-only to introspect live tables and diff against them, without --dsn it renders the full bootstrap DDL fully offline")
	adapter := fs.String("adapter", "sqlite", "db adapter to migrate against (sqlite, postgres, or mysql)")
	dsn := fs.String("dsn", "", "connection string: a file path for sqlite, a libpq DSN for postgres, a go-sql-driver DSN for mysql")
	dropColumns := fs.Bool("drop-columns", false,
		"DESTRUCTIVE: emit ALTER TABLE DROP COLUMN for live columns no longer declared in the schema (their data is lost)")
	lint := fs.Bool("lint", false,
		"lint the planned DDL for unsafe patterns (drops, NOT NULL without default, non-concurrent index, type rewrite); error findings fail the run before anything is applied")
	lockTimeout := fs.Duration("lock-timeout", 0,
		"postgres SET LOCAL lock_timeout per statement (e.g. 10s); zero disables; falls back to db.options.lock_timeout from the config file")
	statementTimeout := fs.Duration("statement-timeout", 0,
		"postgres SET LOCAL statement_timeout per statement (e.g. 30s); zero disables; falls back to db.options.statement_timeout from the config file")
	concurrentIndexes := fs.Bool("concurrent-indexes", false,
		"build postgres indexes with CREATE INDEX CONCURRENTLY (no write block; each build runs outside a transaction)")
	// coverageProof: no local -i/--interactive flags (cf. db.go's removed arm).
	// runDBMigrate peels -i/--interactive via peelInteractive before Parse, so
	// the FlagSet can never see them; interactiveMode is already set globally
	// and isInteractiveTerminal gates the prompts below. Defining them here
	// would only shadow dead branches.

	fs.Usage = func() {
		printMigrateUsage(fs)
	}

	posArgs, err := flexibleParse(fs, args)
	if err != nil {
		return err
	}

	// Interactive prompts for adapter/DSN/files.
	if isInteractiveTerminal() {
		if *adapter == "sqlite" && !flagWasSet(fs, "adapter") {
			// Offer choice even when default would otherwise apply.
			if sel, selErr := promptSelectForMigrate("Database adapter", migrateAdapters); selErr == nil {
				*adapter = sel
			}
		}

		if strings.TrimSpace(*dsn) == "" {
			placeholder := "data/app.db"

			switch *adapter {
			case "postgres":
				placeholder = "postgres://user:pass@localhost/db?sslmode=disable" //nolint:gosec // G101 example placeholder, not credential
			case "mysql":
				placeholder = "user:pass@tcp(127.0.0.1:3306)/dbname"
			}

			if val, valErr := promptInputForMigrate("DSN / database path", placeholder, nil); valErr == nil && val != "" {
				*dsn = val
			} else if valErr != nil {
				return valErr
			}
		}
	}

	rawFiles := posArgs
	if len(rawFiles) == 0 && isInteractiveTerminal() {
		discovered := discoverZenFiles()
		if len(discovered) > 0 {
			sel, multiErr := promptMultiSelectForMigrate("Select schema files", discovered)
			if multiErr != nil {
				return multiErr
			}

			if len(sel) > 0 {
				rawFiles = sel
			}
		} else {
			val, filesErr := promptInputForMigrate("Schema files (space-separated)", "schema/app.zen", nil)
			if filesErr != nil {
				return filesErr
			}

			if val != "" {
				rawFiles = strings.Fields(val)
			}
		}
	}

	// Non-interactive (or interactive-with-nothing-selected) callers with no
	// explicit files still get the same recursive schema-dir auto-discovery
	// every other file-consuming command uses, instead of failing outright.
	resolvedFiles, err := resolveInputFiles(rawFiles)
	if err != nil {
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("try 'zever db migrate -i' for guided file selection"))
		}

		return fmt.Errorf("zever db migrate: %w", err)
	}

	if len(rawFiles) == 0 {
		reportAutoDiscovery(resolvedFiles)
	}

	files, err := loadFiles(resolvedFiles)
	if err != nil {
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("try 'zever db migrate -i' for guided file selection"))
		}

		return fmt.Errorf("zever db migrate: %w", err)
	}

	// Env-first connection: flags win, unset flags fall back to the
	// effective env-derived DB config (DB_ADAPTER plus the adapter's DSN or
	// Path), best-effort. Values are never echoed: failures name the field,
	// never the DSN.
	adapterVal, dsnVal := migrateConnFromEnv(*adapter, *dsn, flagWasSet(fs, "adapter"))

	cfg := resolveMigrateConfig(adapterVal, dsnVal, nil, *dryRun, *dropColumns,
		resolveMigrateSafetyOptions(*lint, *lockTimeout, *statementTimeout, *concurrentIndexes))

	// Config-file fallback for timeouts the flags left unset: db.options
	// lock_timeout/statement_timeout (file or DB_* env) become the plan's
	// timeouts. Best-effort: no config file, or an unloadable one, keeps the
	// flag values (zero disables).
	applyConfigTimeoutFallback(&cfg)

	result, diags := compile.Compile(files, atlas.New())

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		return fmt.Errorf("zever db migrate: %d file(s) failed to compile", len(files))
	}

	dialect, err := migrateDialect(cfg.Adapter)
	if err != nil {
		if shouldShowHint() {
			if s := closest(cfg.Adapter, migrateAdapters); s != "" {
				_, _ = fmt.Fprintln(os.Stderr, formatHint(fmt.Sprintf("did you mean %q?", s)))
			}
		}

		return err
	}

	trimmedDSN := strings.TrimSpace(cfg.DSN)

	// Type-change detection is always on: on postgres it emits the ALTER,
	// on sqlite it only ever warns, so neither needs its own opt-in.
	planOpts := migrate.PlanOptions{
		DropColumns:       cfg.DropColumns,
		DetectTypeChanges: true,
		LockTimeout:       cfg.LockTimeout,
		StatementTimeout:  cfg.StatementTimeout,
		ConcurrentIndexes: cfg.ConcurrentIndexes,
	}

	if cfg.DryRun {
		if trimmedDSN == "" {
			// No database to introspect: fall back to the full bootstrap
			// render, exactly as before diffing existed. This path never
			// touches orm/migrate at all.
			stmts, err := atlas.RenderSchemaDDLWithOptions(dialect, result.Schema, atlas.DDLOptions{ConcurrentIndexes: cfg.ConcurrentIndexes})
			if err != nil {
				return fmt.Errorf("zever db migrate: %w", err)
			}

			if cfg.Lint {
				if err := enforceMigrateLint(dialect, stmts); err != nil {
					return err
				}
			}

			printDryRun(result.Outputs["atlas"], stmts)

			return nil
		}

		return dryRunDiff(cfg.Adapter, trimmedDSN, result, planOpts, cfg.Lint)
	}

	return applyDiff(cfg.Adapter, cfg.DSN, result.Schema, planOpts, cfg.Lint)
}

// migrateConnFromEnv applies the env-first connection rule for db migrate:
// an explicitly-set --adapter/--dsn always wins; otherwise the effective
// env-derived DB config supplies the adapter (DB_ADAPTER) and the DSN or
// file path for that adapter. Resolution is best-effort: an unresolvable
// environment keeps the flag values.
func migrateConnFromEnv(adapter, dsn string, adapterSet bool) (string, string) {
	if adapterSet && strings.TrimSpace(dsn) != "" {
		return adapter, dsn
	}

	env, err := config.FromEnv()
	if err != nil {
		return adapter, dsn
	}

	resolved, err := config.Resolve(env)
	if err != nil || resolved == nil {
		return adapter, dsn
	}

	if !adapterSet && strings.TrimSpace(resolved.DB.Adapter) != "" {
		adapter = resolved.DB.Adapter
	}

	if strings.TrimSpace(dsn) == "" {
		switch adapter {
		case "postgres":
			if strings.TrimSpace(resolved.DB.Options.DSN) != "" {
				dsn = resolved.DB.Options.DSN
			}
		default:
			if strings.TrimSpace(resolved.DB.Options.Path) != "" {
				dsn = resolved.DB.Options.Path
			}
		}
	}

	return adapter, dsn
}

// flagWasSet reports whether a flag was explicitly set on the FlagSet.
func flagWasSet(fs *flag.FlagSet, name string) bool {
	found := false

	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})

	return found
}

// applyConfigTimeoutFallback fills timeouts the flags left unset from the
// resolved service config's db options (OUTBOX_* / DB_* env), best-effort:
// an unresolvable config keeps the flag values (zero disables).
func applyConfigTimeoutFallback(cfg *MigrateConfig) {
	if cfg.LockTimeout != 0 && cfg.StatementTimeout != 0 {
		return
	}

	env, err := config.FromEnv()
	if err != nil {
		return
	}
	fileCfg, loadErr := config.Resolve(env)
	if loadErr != nil || fileCfg == nil {
		return
	}

	if cfg.LockTimeout == 0 {
		cfg.LockTimeout = fileCfg.DB.Options.LockTimeout
	}

	if cfg.StatementTimeout == 0 {
		cfg.StatementTimeout = fileCfg.DB.Options.StatementTimeout
	}
}

// enforceMigrateLint lints stmts for the named dialect, prints every finding
// to stdout, and fails when any finding has error severity. A statement
// carrying a '-- zever:allow:<code>' pragma waives that rule (see
// orm/migrate's linter).
func enforceMigrateLint(dialect string, stmts []string) error {
	findings := migrate.LintStatements(dialect, stmts)

	for _, f := range findings {
		if f.Severity == migrate.LintError {
			// Error findings fail the command, so they go to stderr with an
			// error: prefix. Warnings stay warnings on stdout.
			_, _ = fmt.Fprintf(os.Stderr, "zever db migrate: error: lint [%s]: %s\n", f.Code, f.Message)

			continue
		}

		warnMigrate("lint %s [%s]: %s", f.Severity, f.Code, f.Message)
	}

	var codes []string

	for _, f := range findings {
		if f.Severity == migrate.LintError {
			codes = append(codes, f.Code)
		}
	}

	if len(codes) > 0 {
		sort.Strings(codes)

		return fmt.Errorf("zever db migrate: --lint failed: %s (waive with '-- zever:allow:<code>')", strings.Join(codes, ", "))
	}

	return nil
}

// dryRunDiff opens a read-only connection to introspect the live database,
// asks orm/migrate.Plan for the same statement plan applyDiff would apply,
// and prints it without executing or recording anything. With lint, error
// findings fail the run before anything is printed as executable.
func dryRunDiff(adapter, dsn string, result *compile.Result, planOpts migrate.PlanOptions, lint bool) error {
	conn, err := openZeverDB(adapter, dsn)
	if err != nil {
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("--dry-run with --dsn connects read-only to diff against live tables; omit --dsn to render the bootstrap DDL offline"))
		}

		return err
	}

	ctx := context.Background()

	defer func() {
		_ = conn.Close(ctx)
	}()

	plan, err := migrate.Plan(ctx, conn, result.Schema, planOpts)
	if err != nil {
		return err
	}

	for _, w := range plan.Warnings {
		warnMigrate("%s", w)
	}

	if lint {
		if lintErr := enforceMigrateLint(plan.Dialect, plan.Statements()); lintErr != nil {
			return lintErr
		}
	}

	printDryRun(result.Outputs["atlas"], plan.Statements())

	return nil
}

// applyDiff opens the named db adapter, computes the migration plan against
// live database state via orm/migrate.Plan, and applies it via
// orm/migrate.Apply, which ensures the schema_migrations bookkeeping table
// exists and records each newly-applied statement's checksum. With lint,
// error findings abort the run before anything is applied.
func applyDiff(adapter, dsn string, schema *ir.Schema, planOpts migrate.PlanOptions, lint bool) error {
	conn, err := openZeverDB(adapter, dsn)
	if err != nil {
		if h := migrateDSNHint(adapter, dsn); h != "" {
			return fmt.Errorf("%w\n%s", err, h)
		}

		return err
	}

	ctx := context.Background()

	defer func() {
		_ = conn.Close(ctx)
	}()

	plan, err := migrate.Plan(ctx, conn, schema, planOpts)
	if err != nil {
		return err
	}

	for _, w := range plan.Warnings {
		warnMigrate("%s", w)
	}

	if lint {
		if lintErr := enforceMigrateLint(plan.Dialect, plan.Statements()); lintErr != nil {
			return lintErr
		}
	}

	applied, err := migrate.Apply(ctx, conn, plan)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "%s %s %s %s\n", successMark(), green(fmt.Sprintf("applied %d statement(s)", applied)), dim("via"), cyan(adapter))
	if shouldShowHint() && applied == 0 {
		_, _ = fmt.Fprintln(os.Stderr, formatHint("no new statements — schema already in sync"))
	}

	return nil
}

// openZeverDB parses the adapter name, builds its options from the single
// --dsn flag, and opens the database. MySQL parses as a dialect but has no
// registered zever adapter yet, so it fails here with a gap error rather
// than deeper in the engine.
func openZeverDB(adapterName, dsn string) (db.DB, error) {
	ensureZeverDBAdapters()

	// MySQL parses as a dialect but has no registered zever adapter yet:
	// fail here with a gap error instead of an adapter-registry error.
	if adapterName == "mysql" {
		return nil, errors.New("zever db migrate: --adapter mysql has no registered zever db adapter yet (sqlite and postgres only)")
	}

	adapter, err := db.ParseAdapter(adapterName)
	if err != nil {
		if s := closest(adapterName, migrateAdapters); s != "" {
			return nil, fmt.Errorf("zever db migrate: unsupported --adapter %q (did you mean %q? want sqlite, postgres, or mysql): %w", adapterName, s, err)
		}

		return nil, fmt.Errorf("zever db migrate: unsupported --adapter %q (want sqlite, postgres, or mysql): %w", adapterName, err)
	}

	opts, err := migrateOptions(adapterName, dsn)
	if err != nil {
		return nil, err
	}

	conn, err := db.Open(adapter, opts)
	if err != nil {
		return nil, fmt.Errorf("zever db migrate: open %s: %w", adapterName, err)
	}

	return conn, nil
}

// migrateDSNHint returns the next step for a failed open with no DSN: the
// exact command to re-run, so a first-run failure is copy-pasteable
// instead of a bare "dsn required". It returns "" when the DSN was set
// (the failure is not a missing DSN) or the adapter has no example.
func migrateDSNHint(adapter, dsn string) string {
	if strings.TrimSpace(dsn) != "" {
		return ""
	}

	switch adapter {
	case "sqlite", "postgres":
	default:
		return ""
	}

	if !shouldShowHint() {
		return ""
	}

	return formatHint(fmt.Sprintf("run 'zever db migrate --adapter=%s --dsn=%s' (or -i for a guided DSN prompt)",
		adapter, migrateDSNExample(adapter)))
}

// migrateDSNExample returns the example DSN for an adapter: the sqlite
// file the scaffold creates, or a placeholder postgres connection string.
func migrateDSNExample(adapter string) string {
	switch adapter {
	case "postgres":
		return "postgres://user:pass@localhost/db?sslmode=disable"
	default:
		return DefaultDevDBPath
	}
}

// migrateDialect maps a db adapter name to the SQL dialect the DDL
// renderer targets.
func migrateDialect(adapter string) (string, error) {
	switch adapter {
	case "sqlite":
		return atlas.DialectSQLite, nil
	case "postgres":
		return atlas.DialectPostgres, nil
	case "mysql":
		return atlas.DialectMySQL, nil
	default:
		if s := closest(adapter, migrateAdapters); s != "" {
			return "", fmt.Errorf("zever db migrate: unsupported --adapter %q (did you mean %q? want sqlite, postgres, or mysql)", adapter, s)
		}

		return "", fmt.Errorf("zever db migrate: unsupported --adapter %q (want sqlite, postgres, or mysql)", adapter)
	}
}

// printDryRun writes the generated Atlas HCL followed by the DDL
// statements the non-dry-run path would execute, touching no database.
func printDryRun(hcl map[string][]byte, stmts []string) {
	names := make([]string, 0, len(hcl))
	for name := range hcl {
		names = append(names, name)
	}

	sort.Strings(names)

	if len(names) > 0 {
		_, _ = fmt.Fprintln(os.Stdout, dim("── ")+bold("Atlas HCL")+dim(" ──"))

		for _, name := range names {
			_, _ = fmt.Fprintln(os.Stdout, dim("-- ")+cyan(name))
			_, _ = fmt.Fprintln(os.Stdout, dim(string(hcl[name])))
		}
	}

	// Keep legacy "-- DDL" marker for test compatibility, styled when color enabled.
	_, _ = fmt.Fprintln(os.Stdout, dim("-- DDL"))

	if colorEnabled && len(stmts) > 0 {
		_, _ = fmt.Fprintln(os.Stdout, box(dim("── ")+bold("DDL")+dim(" ──")))
	}

	if len(stmts) == 0 {
		msg := dim("no DDL statements — schema already in sync")
		if colorEnabled {
			_, _ = fmt.Fprintln(os.Stdout, box(msg))
		} else {
			_, _ = fmt.Fprintln(os.Stdout, msg)
		}

		return
	}

	for _, stmt := range stmts {
		_, _ = fmt.Fprintln(os.Stdout, cyan(stmt))
	}

	preview := joinLines(
		dim("dry run — no changes applied"),
		dim(fmt.Sprintf("%d statement(s) would be executed", len(stmts))),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(os.Stdout, box(preview))
	} else {
		_, _ = fmt.Fprintln(os.Stdout, preview)
	}
}

// migrateOptions builds the db.Options an adapter expects from the single
// --dsn flag: sqlite reads a file "path", postgres reads a libpq "dsn".
func migrateOptions(adapter, dsn string) (db.Options, error) {
	dsn = strings.TrimSpace(dsn)

	switch adapter {
	case "sqlite":
		if dsn == "" {
			return db.Options{}, errors.New("zever db migrate: --dsn is required (sqlite database file path)")
		}

		return db.Options{Path: dsn}, nil
	case "postgres":
		if dsn == "" {
			return db.Options{}, errors.New("zever db migrate: --dsn is required (postgres connection string)")
		}

		return db.Options{DSN: dsn}, nil
	case "mysql":
		return db.Options{}, errors.New("zever db migrate: --adapter mysql has no registered zever db adapter yet (sqlite and postgres only)")
	default:
		if s := closest(adapter, migrateAdapters); s != "" {
			return db.Options{}, fmt.Errorf("zever db migrate: unsupported --adapter %q (did you mean %q? want sqlite, postgres, or mysql)", adapter, s)
		}

		return db.Options{}, fmt.Errorf("zever db migrate: unsupported --adapter %q (want sqlite, postgres, or mysql)", adapter)
	}
}

// warnMigrate reports a non-fatal diffing decision (from a
// orm/migrate.MigrationPlan or orm/migrate.RollbackPlan's Warnings) to the
// user on stdout, the same stream applyDiff reports "applied N statement(s)"
// on.
func warnMigrate(format string, args ...any) {
	warnCommand("zever db migrate", format, args...)
}

// warnRollback is warnMigrate's sibling for `zever db rollback`: same stream,
// same shape, correct command name in the prefix.
func warnRollback(format string, args ...any) {
	warnCommand("zever db rollback", format, args...)
}

// warnCommand is the single stdout warning path shared by both commands.
func warnCommand(command, format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stdout, command+": warning: "+format+"\n", args...)
}
