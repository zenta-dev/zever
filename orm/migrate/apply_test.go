package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/compile"
)

func compileSchema(t *testing.T, src string) *compile.Result {
	t.Helper()

	result, diags := compile.Compile(map[string]string{"schema.zen": src})
	if diags.HasErrors() {
		t.Fatalf("compile: %v", diags)
	}

	return result
}

var registerSQLiteOnce sync.Once

func openTestSQLite(t *testing.T, name string) db.DB {
	t.Helper()

	registerSQLiteOnce.Do(func() {
		_ = db.Register(db.SQLite, sqlite.New)
	})

	conn, err := db.Open(db.SQLite, db.Options{Path: filepath.Join(t.TempDir(), name)})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	return conn
}

const applyTestSchema = `entity User {
	id: uuid @primary
	email: string @unique
}
`

func TestChecksumOf(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256([]byte("SELECT 1;"))
	want := hex.EncodeToString(sum[:])

	if got := ChecksumOf("SELECT 1;"); got != want {
		t.Fatalf("ChecksumOf = %q, want %q", got, want)
	}

	if ChecksumOf("a") == ChecksumOf("b") {
		t.Fatal("ChecksumOf collision on distinct inputs")
	}
}

func TestEnsureMigrationsTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialect string
		wantErr bool
	}{
		{"sqlite", atlas.DialectSQLite, false},
		{"postgres", atlas.DialectPostgres, false},
		{"mysql", atlas.DialectMySQL, false},
		{"unsupported", "oracle", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			if tt.dialect == atlas.DialectSQLite {
				conn := openTestSQLite(t, "ensure.db")
				if err := EnsureMigrationsTable(ctx, conn, tt.dialect); err != nil {
					t.Fatalf("EnsureMigrationsTable: %v", err)
				}

				// Idempotent: a second call upgrades nothing and fails nothing.
				if err := EnsureMigrationsTable(ctx, conn, tt.dialect); err != nil {
					t.Fatalf("EnsureMigrationsTable rerun: %v", err)
				}

				return
			}

			var execs []string

			conn := &fakeDB{
				dialect: tt.dialect,
				onQuery: func(_ string, _ []any) (db.Rows, error) {
					return &fakeRows{}, nil
				},
				onExec: func(q string, _ []any) (int64, error) {
					execs = append(execs, q)
					return 1, nil
				},
			}

			err := EnsureMigrationsTable(ctx, conn, tt.dialect)
			if tt.wantErr {
				if err == nil {
					t.Fatal("EnsureMigrationsTable: want error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("EnsureMigrationsTable: %v", err)
			}

			if len(execs) == 0 {
				t.Fatal("no statements executed")
			}
		})
	}
}

func TestEnsureMigrationsTable_createError(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{
		dialect: atlas.DialectPostgres,
		onExec: func(_ string, _ []any) (int64, error) {
			return 0, errors.New("boom")
		},
	}

	if err := EnsureMigrationsTable(t.Context(), conn, atlas.DialectPostgres); err == nil {
		t.Fatal("EnsureMigrationsTable: want error, got nil")
	}
}

func TestAddMigrationTrackingColumns(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("postgresAddsMissingWithoutIntrospection", func(t *testing.T) {
		t.Parallel()

		var execs []string

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onExec: func(q string, _ []any) (int64, error) {
				execs = append(execs, q)
				return 1, nil
			},
		}

		if err := addMigrationTrackingColumns(ctx, conn, atlas.DialectPostgres); err != nil {
			t.Fatalf("addMigrationTrackingColumns: %v", err)
		}

		if len(execs) != len(migrationTrackingColumns) {
			t.Fatalf("executed %d, want %d", len(execs), len(migrationTrackingColumns))
		}
	})

	t.Run("sqliteSkipsPresentColumns", func(t *testing.T) {
		t.Parallel()

		conn := openTestSQLite(t, "tracked.db")
		if err := EnsureMigrationsTable(ctx, conn, atlas.DialectSQLite); err != nil {
			t.Fatalf("EnsureMigrationsTable: %v", err)
		}

		var execs []string

		wrapped := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(q string, args []any) (db.Rows, error) {
				return conn.Query(ctx, q, args...)
			},
			onExec: func(q string, args []any) (int64, error) {
				execs = append(execs, q)
				return conn.Exec(ctx, q, args...)
			},
		}

		if err := addMigrationTrackingColumns(ctx, wrapped, atlas.DialectSQLite); err != nil {
			t.Fatalf("addMigrationTrackingColumns: %v", err)
		}

		if len(execs) != 0 {
			t.Fatalf("executed %v, want none (all present)", execs)
		}
	})

	t.Run("sqliteIntrospectError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		// Table name withour validation issues; introspection fails.
		if err := addMigrationTrackingColumns(ctx, conn, atlas.DialectSQLite); err == nil {
			t.Fatal("addMigrationTrackingColumns: want error, got nil")
		}
	})

	t.Run("mysqlIntrospectError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if err := addMigrationTrackingColumns(ctx, conn, atlas.DialectMySQL); err == nil {
			t.Fatal("addMigrationTrackingColumns: want error, got nil")
		}
	})

	t.Run("mysqlAddsMissing", func(t *testing.T) {
		t.Parallel()

		var execs []string

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				// One tracking column already present: skipped, not re-added.
				return &fakeRows{vals: [][]any{{"kind", "text", "YES", nil}}}, nil
			},
			onExec: func(q string, _ []any) (int64, error) {
				execs = append(execs, q)
				return 1, nil
			},
		}

		if err := addMigrationTrackingColumns(ctx, conn, atlas.DialectMySQL); err != nil {
			t.Fatalf("addMigrationTrackingColumns: %v", err)
		}

		if len(execs) != len(migrationTrackingColumns)-1 {
			t.Fatalf("executed %d, want %d", len(execs), len(migrationTrackingColumns)-1)
		}
	})

	t.Run("addColumnError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onExec: func(_ string, _ []any) (int64, error) {
				return 0, errors.New("boom")
			},
		}

		if err := addMigrationTrackingColumns(ctx, conn, atlas.DialectPostgres); err == nil {
			t.Fatal("addMigrationTrackingColumns: want error, got nil")
		}
	})

	t.Run("unsupportedDialect", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{dialect: "oracle"}

		if err := addMigrationTrackingColumns(ctx, conn, "oracle"); err == nil {
			t.Fatal("addMigrationTrackingColumns: want error, got nil")
		}
	})
}

func TestMigrationApplied_queryError(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{
		dialect: atlas.DialectSQLite,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return nil, errors.New("boom")
		},
	}

	if _, err := migrationApplied(t.Context(), conn, "abc"); err == nil {
		t.Fatal("migrationApplied: want error, got nil")
	}
}

func TestRecordMigration_execError(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{
		dialect: atlas.DialectSQLite,
		onExec: func(_ string, _ []any) (int64, error) {
			return 0, errors.New("boom")
		},
	}

	if err := recordMigration(t.Context(), conn, "abc", migrationMeta{}); err == nil {
		t.Fatal("recordMigration: want error, got nil")
	}
}

func TestApply(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("nilPlan", func(t *testing.T) {
		t.Parallel()

		if _, err := Apply(ctx, &fakeDB{dialect: atlas.DialectSQLite}, nil); err == nil {
			t.Fatal("Apply(nil): want error, got nil")
		}
	})

	t.Run("bootstrapThenRerunIsNoop", func(t *testing.T) {
		conn := openTestSQLite(t, "apply.db")
		v1 := compileSchema(t, applyTestSchema)

		plan, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}

		if len(plan.Statements()) == 0 {
			t.Fatal("expected a non-empty bootstrap plan")
		}

		n, err := Apply(ctx, conn, plan)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}

		if n != len(plan.Statements()) {
			t.Fatalf("Apply = %d, want %d", n, len(plan.Statements()))
		}

		// Re-running the same plan applies nothing.
		again, err := Apply(ctx, conn, plan)
		if err != nil {
			t.Fatalf("Apply rerun: %v", err)
		}

		if again != 0 {
			t.Fatalf("Apply rerun = %d, want 0", again)
		}
	})

	t.Run("ensureError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		plan := &MigrationPlan{Dialect: atlas.DialectSQLite}

		if _, err := Apply(ctx, conn, plan); err == nil {
			t.Fatal("Apply: want ensure error, got nil")
		}
	})

	t.Run("unguardedWithoutTransactor", func(t *testing.T) {
		t.Parallel()

		var execs []string

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				// Schema_migrations introspection: PRAGMA returns no rows;
				// applied check returns no rows (not applied).
				if strings.HasPrefix(q, "PRAGMA") {
					return &fakeRows{}, nil
				}

				return &fakeRows{}, nil
			},
			onExec: func(q string, _ []any) (int64, error) {
				execs = append(execs, q)
				return 1, nil
			},
		}

		plan := &MigrationPlan{
			Dialect: atlas.DialectSQLite,
			statements: []plannedStatement{{
				SQL:  "ALTER TABLE \"users\" ADD COLUMN \"note\" TEXT;",
				Meta: migrationMeta{Kind: kindAddColumn, Table: `"users"`, Column: "note"},
			}},
		}

		n, err := Apply(ctx, conn, plan)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}

		if n != 1 {
			t.Fatalf("Apply = %d, want 1", n)
		}
	})

	t.Run("appliedCheckError", func(t *testing.T) {
		t.Parallel()

		calls := 0

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				calls++
				if strings.Contains(q, "WHERE checksum") {
					return nil, errors.New("boom")
				}

				return &fakeRows{}, nil
			},
			onExec: func(_ string, _ []any) (int64, error) {
				return 1, nil
			},
		}

		plan := &MigrationPlan{
			Dialect: atlas.DialectSQLite,
			statements: []plannedStatement{{
				SQL:  "ALTER TABLE \"users\" ADD COLUMN \"note\" TEXT;",
				Meta: migrationMeta{Kind: kindAddColumn, Table: `"users"`, Column: "note"},
			}},
		}

		if _, err := Apply(ctx, conn, plan); err == nil {
			t.Fatal("Apply: want applied-check error, got nil")
		}
	})

	t.Run("statementExecError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{}, nil
			},
			onExec: func(q string, _ []any) (int64, error) {
				if strings.Contains(q, `"note"`) {
					return 0, errors.New("boom")
				}

				return 1, nil
			},
		}

		plan := &MigrationPlan{
			Dialect: atlas.DialectSQLite,
			statements: []plannedStatement{{
				SQL:  "ALTER TABLE \"users\" ADD COLUMN \"note\" TEXT;",
				Meta: migrationMeta{Kind: kindAddColumn, Table: `"users"`, Column: "note"},
			}},
		}

		if _, err := Apply(ctx, conn, plan); err == nil {
			t.Fatal("Apply: want exec error, got nil")
		}
	})

	t.Run("transactionalError", func(t *testing.T) {
		t.Parallel()

		inner := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{}, nil
			},
			onExec: func(q string, _ []any) (int64, error) {
				if strings.HasPrefix(q, "ALTER TABLE") {
					return 0, errors.New("boom")
				}

				return 1, nil
			},
		}
		conn := &txFakeDB{fakeDB: inner}

		plan := &MigrationPlan{
			Dialect: atlas.DialectSQLite,
			statements: []plannedStatement{{
				SQL:  "ALTER TABLE \"users\" ADD COLUMN \"note\" TEXT;",
				Meta: migrationMeta{Kind: kindAddColumn, Table: `"users"`, Column: "note"},
			}},
		}

		if _, err := Apply(ctx, conn, plan); err == nil {
			t.Fatal("Apply: want transactional exec error, got nil")
		}
	})
}

func TestApplyStatementInTx(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	stmt := plannedStatement{SQL: "ALTER TABLE x;", Meta: migrationMeta{Kind: kindAddColumn}}

	t.Run("commits", func(t *testing.T) {
		t.Parallel()

		inner := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{}, nil
			},
			onExec: func(_ string, _ []any) (int64, error) {
				return 1, nil
			},
		}
		conn := &txFakeDB{fakeDB: inner}

		if err := applyStatementInTx(ctx, conn, stmt, "abc"); err != nil {
			t.Fatalf("applyStatementInTx: %v", err)
		}

		if !conn.tx.committed {
			t.Fatal("transaction was not committed")
		}
	})

	t.Run("execErrorRollsBack", func(t *testing.T) {
		t.Parallel()

		inner := &fakeDB{
			dialect: atlas.DialectSQLite,
			onExec: func(_ string, _ []any) (int64, error) {
				return 0, errors.New("boom")
			},
		}
		conn := &txFakeDB{fakeDB: inner}

		if err := applyStatementInTx(ctx, conn, stmt, "abc"); err == nil {
			t.Fatal("applyStatementInTx: want error, got nil")
		}

		if !conn.tx.rolledBack {
			t.Fatal("transaction was not rolled back")
		}
	})

	t.Run("recordErrorRollsBack", func(t *testing.T) {
		t.Parallel()

		inner := &fakeDB{
			dialect: atlas.DialectSQLite,
			onExec: func(q string, _ []any) (int64, error) {
				if strings.HasPrefix(q, "INSERT INTO") {
					return 0, errors.New("boom")
				}

				return 1, nil
			},
		}
		conn := &txFakeDB{fakeDB: inner}

		if err := applyStatementInTx(ctx, conn, stmt, "abc"); err == nil {
			t.Fatal("applyStatementInTx: want record error, got nil")
		}
	})

	t.Run("beginError", func(t *testing.T) {
		t.Parallel()

		inner := &fakeDB{dialect: atlas.DialectSQLite}
		conn := &txFakeDB{fakeDB: inner, beginErr: errors.New("boom")}

		if err := applyStatementInTx(ctx, conn, stmt, "abc"); err == nil {
			t.Fatal("applyStatementInTx: want begin error, got nil")
		}
	})
}

func TestApplyMigrationPlan_skipsApplied(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	conn := &fakeDB{
		dialect: atlas.DialectSQLite,
		onQuery: func(q string, _ []any) (db.Rows, error) {
			if strings.Contains(q, "WHERE checksum") {
				return &fakeRows{vals: [][]any{{1}}}, nil
			}

			return &fakeRows{}, nil
		},
		onExec: func(q string, _ []any) (int64, error) {
			if strings.HasPrefix(q, "ALTER TABLE") {
				t.Error("already-applied statement was executed")
			}

			return 1, nil
		},
	}

	n, err := applyMigrationPlan(ctx, conn, []plannedStatement{{
		SQL:  "ALTER TABLE \"users\" ADD COLUMN \"note\" TEXT;",
		Meta: migrationMeta{Kind: kindAddColumn},
	}})
	if err != nil {
		t.Fatalf("applyMigrationPlan: %v", err)
	}

	if n != 0 {
		t.Fatalf("applyMigrationPlan = %d, want 0 (already applied)", n)
	}
}

// injectingTx fails Exec for matching queries, forcing the bookkeeping
// INSERT to fail after its DDL succeeded inside one transaction.
type injectingTx struct {
	db.Tx
}

func (t *injectingTx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if strings.Contains(query, "INSERT INTO "+schemaMigrationsTable) {
		return 0, errors.New("injected failure: bookkeeping insert")
	}

	return t.Tx.Exec(ctx, query, args...)
}

type injectingDB struct {
	db.DB
	transactor db.Transactor
}

func (d *injectingDB) BeginTx(ctx context.Context, opts *db.TxOptions) (db.Tx, error) {
	tx, err := d.transactor.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &injectingTx{Tx: tx}, nil
}

func TestApply_rollsBackDDLWhenBookkeepingFails(t *testing.T) {
	ctx := t.Context()
	realConn := openTestSQLite(t, "rollback.db")

	transactor, ok := realConn.(db.Transactor)
	if !ok {
		t.Fatal("sqlite adapter does not implement db.Transactor")
	}

	wrapped := &injectingDB{DB: realConn, transactor: transactor}
	v1 := compileSchema(t, applyTestSchema)

	plan, err := Plan(ctx, wrapped, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.Statements()) == 0 {
		t.Fatal("expected a non-empty bootstrap plan")
	}

	if _, err = Apply(ctx, wrapped, plan); err == nil {
		t.Fatal("Apply: want injected failure, got nil")
	}

	// The DDL rolled back with the failed insert: users must not exist.
	rows, err := realConn.Query(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='users'")
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	defer func() { _ = rows.Close() }()

	if rows.Next() {
		t.Fatal("users table exists despite rolled-back Apply")
	}
}

func TestSupportsTransactionalDDL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dialect string
		want    bool
	}{
		{atlas.DialectPostgres, true},
		{atlas.DialectSQLite, true},
		{atlas.DialectMySQL, false},
		{"oracle", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.dialect, func(t *testing.T) {
			t.Parallel()

			if got := supportsTransactionalDDL(tt.dialect); got != tt.want {
				t.Fatalf("supportsTransactionalDDL(%q) = %v, want %v", tt.dialect, got, tt.want)
			}
		})
	}
}

func TestDialectOf(t *testing.T) {
	t.Parallel()

	if got := dialectOf(nil); got != "" {
		t.Fatalf("dialectOf(nil) = %q, want empty", got)
	}

	conn := &fakeDB{dialect: atlas.DialectSQLite}
	if got := dialectOf(conn); got != atlas.DialectSQLite {
		t.Fatalf("dialectOf = %q", got)
	}
}

func TestFirstLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"ALTER TABLE x;\nADD COLUMN y;", "ALTER TABLE x;"},
		{"single", "single"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := firstLine(tt.in); got != tt.want {
			t.Fatalf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestApplyStatementUnguarded_recordError(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{
		dialect: atlas.DialectSQLite,
		onExec: func(q string, _ []any) (int64, error) {
			if strings.HasPrefix(q, "INSERT INTO") {
				return 0, errors.New("boom")
			}

			return 1, nil
		},
	}

	stmt := plannedStatement{SQL: "ALTER TABLE x;", Meta: migrationMeta{Kind: kindAddColumn}}

	if err := applyStatementUnguarded(t.Context(), conn, stmt, "abc"); err == nil {
		t.Fatal("applyStatementUnguarded: want record error, got nil")
	}
}
