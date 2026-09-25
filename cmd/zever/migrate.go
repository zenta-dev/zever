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

	"github.com/zenta-dev/zever/db"
	dbpostgres "github.com/zenta-dev/zever/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/db/sqlite"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/compile"
	"github.com/zenta-dev/zever/internal/dsl/ir"
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
}

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seams keep behavior identical while letting tests stub success/error
// paths for 100% statement coverage.
var (
	promptSelectForMigrate      = promptSelect
	promptInputForMigrate       = promptInput
	promptMultiSelectForMigrate = promptMultiSelect
)

// resolveMigrateConfig builds a MigrateConfig from parsed flag values without
// side effects.
func resolveMigrateConfig(adapter, dsn string, files []string, dryRun, dropColumns bool) MigrateConfig {
	return MigrateConfig{Adapter: adapter, DSN: dsn, Files: files, DryRun: dryRun, DropColumns: dropColumns}
}

func printMigrateUsage(fs *flag.FlagSet) {
	header := title("zever db migrate") + dim(" — schema → live database")
	usage := bold("Usage:") + "  " + cmd("zever db migrate") + dim(" [flags…] <files…>") + dim("  •  -i/--interactive for guided prompts")

	printBoxedUsage(fs, header, usage, migrateHelp, []usageExample{
		{command: "zever db migrate schema/app.zen --adapter=sqlite --dsn=data/app.db"},
		{command: "zever db migrate --dry-run schema/*.zen --adapter=postgres --dsn='postgres://localhost/db?sslmode=disable'"},
		{command: "zever db migrate -i", comment: "  # guided: select files, adapter & DSN"},
	}, "use --dry-run to preview DDL without executing")
}

func runDBMigrate(args []string) error {
	args = peelInteractive(args)
	fs := flag.NewFlagSet("db migrate", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print the generated Atlas HCL and DDL to stdout without connecting to a database")
	adapter := fs.String("adapter", "sqlite", "db adapter to migrate against (sqlite, postgres, or mysql)")
	dsn := fs.String("dsn", "", "connection string: a file path for sqlite, a libpq DSN for postgres, a go-sql-driver DSN for mysql")
	dropColumns := fs.Bool("drop-columns", false,
		"DESTRUCTIVE: emit ALTER TABLE DROP COLUMN for live columns no longer declared in the schema (their data is lost)")
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

		return err
	}

	if len(rawFiles) == 0 {
		reportAutoDiscovery(resolvedFiles)
	}

	files, err := loadFiles(resolvedFiles)
	if err != nil {
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("try 'zever db migrate -i' for guided file selection"))
		}

		return err
	}

	cfg := resolveMigrateConfig(*adapter, *dsn, nil, *dryRun, *dropColumns)

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
	planOpts := migrate.PlanOptions{DropColumns: cfg.DropColumns, DetectTypeChanges: true}

	if cfg.DryRun {
		if trimmedDSN == "" {
			// No database to introspect: fall back to the full bootstrap
			// render, exactly as before diffing existed. This path never
			// touches orm/migrate at all.
			stmts, err := atlas.RenderSchemaDDL(dialect, result.Schema)
			if err != nil {
				return fmt.Errorf("zever db migrate: %w", err)
			}

			printDryRun(result.Outputs["atlas"], stmts)

			return nil
		}

		return dryRunDiff(cfg.Adapter, trimmedDSN, result, planOpts)
	}

	return applyDiff(cfg.Adapter, cfg.DSN, result.Schema, planOpts)
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

// dryRunDiff opens a read-only connection to introspect the live database,
// asks orm/migrate.Plan for the same statement plan applyDiff would apply,
// and prints it without executing or recording anything.
func dryRunDiff(adapter, dsn string, result *compile.Result, planOpts migrate.PlanOptions) error {
	conn, err := openZeverDB(adapter, dsn)
	if err != nil {
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

	printDryRun(result.Outputs["atlas"], plan.Statements())

	return nil
}

// applyDiff opens the named db adapter, computes the migration plan against
// live database state via orm/migrate.Plan, and applies it via
// orm/migrate.Apply, which ensures the schema_migrations bookkeeping table
// exists and records each newly-applied statement's checksum.
func applyDiff(adapter, dsn string, schema *ir.Schema, planOpts migrate.PlanOptions) error {
	conn, err := openZeverDB(adapter, dsn)
	if err != nil {
		if shouldShowHint() {
			_, _ = fmt.Fprintln(os.Stderr, formatHint("use -i for guided DSN prompt"))
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
		return nil, errors.New("zever db: --adapter mysql has no registered zever db adapter yet (sqlite and postgres only)")
	}

	adapter, err := db.ParseAdapter(adapterName)
	if err != nil {
		if s := closest(adapterName, migrateAdapters); s != "" {
			return nil, fmt.Errorf("zever db: unsupported --adapter %q (did you mean %q? want sqlite, postgres, or mysql): %w", adapterName, s, err)
		}

		return nil, fmt.Errorf("zever db: unsupported --adapter %q (want sqlite, postgres, or mysql): %w", adapterName, err)
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
