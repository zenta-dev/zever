package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/backend/atlas"
)

func mustLintStatements(t *testing.T, dialect string, stmts []string) []LintFinding {
	t.Helper()

	return LintStatements(dialect, stmts)
}

func findingCodes(findings []LintFinding) map[string]LintSeverity {
	out := make(map[string]LintSeverity, len(findings))
	for _, f := range findings {
		out[f.Code] = f.Severity
	}

	return out
}

func TestLintStatementsFlagsDangerousDDL(t *testing.T) {
	t.Parallel()

	stmts := []string{
		`ALTER TABLE "public"."users" DROP COLUMN IF EXISTS "email";`,
		`DROP TABLE IF EXISTS "public"."legacy";`,
		`ALTER TABLE "public"."users" ADD COLUMN "nick" text NOT NULL;`,
		`CREATE INDEX IF NOT EXISTS "users_email_idx" ON "public"."users" ("email");`,
		`ALTER TABLE "public"."users" ALTER COLUMN "age" TYPE bigint;`,
	}

	got := findingCodes(mustLintStatements(t, atlas.DialectPostgres, stmts))

	tests := []struct {
		code string
		want LintSeverity
	}{
		{code: LintDropColumn, want: LintError},
		{code: LintDropTable, want: LintError},
		{code: LintNotNullNoDefault, want: LintError},
		{code: LintNonConcurrentIndex, want: LintWarn},
		{code: LintTypeRewrite, want: LintError},
	}

	for _, tt := range tests {
		if got[tt.code] != tt.want {
			t.Errorf("code %q: got severity %q, want %q (all findings: %v)", tt.code, got[tt.code], tt.want, got)
		}
	}
}

func TestLintStatementsCleanDDLPasses(t *testing.T) {
	t.Parallel()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS "public"."users" ("id" uuid NOT NULL, PRIMARY KEY ("id"));`,
		`ALTER TABLE "public"."users" ADD COLUMN IF NOT EXISTS "nick" text;`,
		`ALTER TABLE "public"."users" ADD COLUMN IF NOT EXISTS "role" text NOT NULL DEFAULT 'user';`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "users_email_idx" ON "public"."users" ("email");`,
	}

	if got := mustLintStatements(t, atlas.DialectPostgres, stmts); len(got) != 0 {
		t.Fatalf("expected no findings, got %v", got)
	}
}

func TestLintStatementsNonPostgresSkipsConcurrentRule(t *testing.T) {
	t.Parallel()

	stmts := []string{
		`CREATE INDEX IF NOT EXISTS "users_email_idx" ON "users" ("email");`,
	}

	if got := mustLintStatements(t, atlas.DialectSQLite, stmts); len(got) != 0 {
		t.Fatalf("sqlite should not flag plain CREATE INDEX, got %v", got)
	}
}

func TestLintStatementsAllowPragmaWaives(t *testing.T) {
	t.Parallel()

	stmts := []string{
		"-- zever:allow:drop_column\n" + `ALTER TABLE "public"."users" DROP COLUMN IF EXISTS "email";`,
		"-- zever:allow\n" + `DROP TABLE IF EXISTS "public"."legacy";`,
	}

	if got := mustLintStatements(t, atlas.DialectPostgres, stmts); len(got) != 0 {
		t.Fatalf("expected pragma-waived findings to be suppressed, got %v", got)
	}
}

func TestLintStatementsAllowPragmaIsScopedToListedCodes(t *testing.T) {
	t.Parallel()

	stmts := []string{
		"-- zever:allow:drop_column\n" + `DROP TABLE IF EXISTS "public"."legacy";`,
	}

	got := mustLintStatements(t, atlas.DialectPostgres, stmts)
	if len(got) != 1 || got[0].Code != LintDropTable {
		t.Fatalf("scoped pragma must not waive other codes, got %v", got)
	}
}

func TestLintGoldenDDL(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "lint_golden.sql"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	stmts := splitGoldenStatements(t, string(raw))
	got := findingCodes(mustLintStatements(t, atlas.DialectPostgres, stmts))

	for _, code := range []string{LintDropColumn, LintNonConcurrentIndex, LintTypeRewrite} {
		if _, ok := got[code]; !ok {
			t.Errorf("golden: missing finding %q (got %v)", code, got)
		}
	}

	if _, ok := got[LintDropTable]; ok {
		t.Errorf("golden: unexpected drop_table finding (got %v)", got)
	}
}

func splitGoldenStatements(t *testing.T, raw string) []string {
	t.Helper()

	var out []string

	var cur strings.Builder

	flush := func() {
		if strings.TrimSpace(cur.String()) != "" {
			out = append(out, cur.String())
		}

		cur.Reset()
	}

	for _, line := range strings.Split(raw, "\n") {
		cur.WriteString(line + "\n")

		// A statement ends at the first `;` outside a comment line;
		// comment lines (including `-- zever:allow` pragmas) attach to
		// the statement that follows them.
		if strings.Contains(line, ";") && !strings.HasPrefix(strings.TrimSpace(line), "--") {
			flush()
		}
	}

	flush()

	return out
}
