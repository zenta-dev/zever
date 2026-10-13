package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// lintDropSchema declares a column the live database still has but the schema
// no longer does, so a --drop-columns plan contains a DROP COLUMN for --lint
// to flag.
const lintDropSchema = `entity User {
	id: uuid @primary
	email: string @unique
}
`

func writeLintLiveDB(t *testing.T, dir string) string {
	t.Helper()

	dsn := filepath.Join(dir, "lint.db")

	if err := runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dsn, writeZeverFixture(t, dir, "schema.zen", lintDropSchema)}); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}

	return dsn
}

func TestMigrateLintFlagsDropColumn(t *testing.T) {
	dir := t.TempDir()
	dsn := writeLintLiveDB(t, dir)

	narrow := writeZeverFixture(t, dir, "narrow.zen", `entity User {
	id: uuid @primary
}
`)

	err := runDBMigrate([]string{
		"--adapter=sqlite", "--dsn=" + dsn,
		"--drop-columns", "--lint", "--dry-run", narrow,
	})
	if err == nil {
		t.Fatalf("expected --lint to fail on DROP COLUMN, got nil")
	}

	if !strings.Contains(err.Error(), "drop_column") {
		t.Fatalf("lint error should name the rule, got %v", err)
	}
}

func TestMigrateLintPassesCleanPlan(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "clean.db")
	schemaPath := writeZeverFixture(t, dir, "schema.zen", lintDropSchema)

	output := captureZeverStdout(t, func() {
		if err := runDBMigrate([]string{"--adapter=sqlite", "--dsn=" + dsn, "--lint", schemaPath}); err != nil {
			t.Fatalf("clean plan with --lint: %v", err)
		}
	})

	if !strings.Contains(output, "applied 1 statement(s)") {
		t.Fatalf("expected apply output, got:\n%s", output)
	}
}

func TestEnforceMigrateLintWarnOnlyPasses(t *testing.T) {
	t.Parallel()

	output := captureZeverStdout(t, func() {
		err := enforceMigrateLint("postgres", []string{
			`CREATE INDEX IF NOT EXISTS "users_nick_idx" ON "public"."users" ("nick");`,
		})
		if err != nil {
			t.Fatalf("warn-only lint should pass, got %v", err)
		}
	})

	if !strings.Contains(output, "non_concurrent_index") {
		t.Fatalf("expected warning printed, got:\n%s", output)
	}
}

func TestEnforceMigrateLintWaivedErrorPasses(t *testing.T) {
	t.Parallel()

	output := captureZeverStdout(t, func() {
		err := enforceMigrateLint("postgres", []string{
			"-- zever:allow:drop_column\n" + `ALTER TABLE "public"."users" DROP COLUMN IF EXISTS "email";`,
		})
		if err != nil {
			t.Fatalf("waived lint should pass, got %v", err)
		}
	})

	if strings.Contains(output, "drop_column") {
		t.Fatalf("waived finding should not print, got:\n%s", output)
	}
}

func TestResolveMigrateConfigCarriesSafetyOptions(t *testing.T) {
	t.Parallel()

	cfg := resolveMigrateConfig("postgres", "postgres://localhost/db", nil, false, false,
		resolveMigrateSafetyOptions(true, 10*time.Second, 30*time.Second, true))

	if !cfg.Lint || cfg.LockTimeout != 10*time.Second || cfg.StatementTimeout != 30*time.Second || !cfg.ConcurrentIndexes {
		t.Fatalf("safety options lost: %+v", cfg)
	}
}
