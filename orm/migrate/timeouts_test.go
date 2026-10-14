package migrate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/dsl/backend/atlas"
)

func TestPlanRejectsNegativeTimeouts(t *testing.T) {
	t.Parallel()

	conn := openTestSQLite(t, "neg-timeouts.db")
	schema := compileSchema(t, applyTestSchema).Schema

	if _, err := Plan(t.Context(), conn, schema, PlanOptions{LockTimeout: -time.Second}); err == nil {
		t.Fatalf("expected error for negative lock_timeout, got nil")
	}

	if _, err := Plan(t.Context(), conn, schema, PlanOptions{StatementTimeout: -time.Second}); err == nil {
		t.Fatalf("expected error for negative statement_timeout, got nil")
	}
}

func TestPlanCarriesTimeouts(t *testing.T) {
	t.Parallel()

	conn := openTestSQLite(t, "plan-timeouts.db")
	schema := compileSchema(t, applyTestSchema).Schema

	plan, err := Plan(t.Context(), conn, schema, PlanOptions{
		LockTimeout:      10 * time.Second,
		StatementTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if plan.LockTimeout != 10*time.Second || plan.StatementTimeout != 30*time.Second {
		t.Fatalf("plan did not carry timeouts: lock=%v stmt=%v", plan.LockTimeout, plan.StatementTimeout)
	}
}

func newTimeoutFakeDB() *fakeDB {
	return &fakeDB{
		dialect: atlas.DialectPostgres,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return &fakeRows{}, nil
		},
		onExec: func(_ string, _ []any) (int64, error) {
			return 0, nil
		},
	}
}

func TestApplySetsLocalTimeoutsOnPostgres(t *testing.T) {
	t.Parallel()

	inner := newTimeoutFakeDB()
	conn := &txFakeDB{fakeDB: inner}

	stmts := []plannedStatement{
		{SQL: `ALTER TABLE "public"."users" ADD COLUMN IF NOT EXISTS "nick" text;`},
	}

	if _, err := applyMigrationPlan(context.Background(), conn, stmts, 10*time.Second, 30*time.Second); err != nil {
		t.Fatalf("applyMigrationPlan: %v", err)
	}

	joined := strings.Join(inner.execs, "\n")
	if !strings.Contains(joined, "lock_timeout") || !strings.Contains(joined, "statement_timeout") {
		t.Fatalf("expected SET LOCAL timeouts before DDL, execs:\n%s", joined)
	}
}

func TestApplySkipsTimeoutsWhenZero(t *testing.T) {
	t.Parallel()

	inner := newTimeoutFakeDB()
	conn := &txFakeDB{fakeDB: inner}

	stmts := []plannedStatement{
		{SQL: `ALTER TABLE "public"."users" ADD COLUMN IF NOT EXISTS "nick" text;`},
	}

	if _, err := applyMigrationPlan(context.Background(), conn, stmts, 0, 0); err != nil {
		t.Fatalf("applyMigrationPlan: %v", err)
	}

	for _, q := range inner.execs {
		if strings.Contains(strings.ToLower(q), "lock_timeout") ||
			strings.Contains(strings.ToLower(q), "statement_timeout") {
			t.Fatalf("zero timeouts must not emit SET LOCAL, got %q", q)
		}
	}
}

func TestConcurrentIndexRunsOutsideTx(t *testing.T) {
	t.Parallel()

	inner := newTimeoutFakeDB()
	conn := &txFakeDB{fakeDB: inner}

	stmts := []plannedStatement{
		{SQL: `CREATE INDEX CONCURRENTLY IF NOT EXISTS "u" ON "public"."t" ("c");`},
	}

	if _, err := applyMigrationPlan(context.Background(), conn, stmts, 10*time.Second, 0); err != nil {
		t.Fatalf("applyMigrationPlan: %v", err)
	}

	if conn.tx != nil && conn.tx.committed {
		t.Fatalf("CONCURRENTLY index must not run inside a transaction")
	}
}

func TestConcurrentIndexEmittedForPostgres(t *testing.T) {
	t.Parallel()

	conn := openTestSQLite(t, "concurrent-idx.db")
	_ = conn

	inner := newTimeoutFakeDB()
	pg := &txFakeDB{fakeDB: inner}
	_ = pg

	sql, err := createIndexSQL(atlas.DialectPostgres, `"public"."users"`, "users_nick_idx", []string{"nick"}, false, true)
	if err != nil {
		t.Fatalf("createIndexSQL: %v", err)
	}

	if !strings.Contains(sql, "CONCURRENTLY") {
		t.Fatalf("expected CONCURRENTLY in %q", sql)
	}

	plain, err := createIndexSQL(atlas.DialectPostgres, `"public"."users"`, "users_nick_idx", []string{"nick"}, false, false)
	if err != nil {
		t.Fatalf("createIndexSQL: %v", err)
	}

	if strings.Contains(plain, "CONCURRENTLY") {
		t.Fatalf("plain index must not contain CONCURRENTLY: %q", plain)
	}
}
