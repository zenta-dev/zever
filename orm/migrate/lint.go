package migrate

import (
	"regexp"
	"strings"

	"github.com/zenta-dev/zever/dsl/backend/atlas"
)

// This file implements the migration safety linter behind `zever db migrate
// --lint`: a pure function over already-rendered DDL statements (golden DDL
// in tests, a live MigrationPlan in production) that flags statements known
// to lock, block, or destroy data on a production database.
//
// The linter never connects to a database and never parses the .zen schema:
// every rule is a syntactic match on one statement's text, so its findings
// are stable across runs and explainable next to the statement that caused
// them. A finding is suppressed when the statement carries a
// `-- zever:allow:<code>` pragma (see allowedCodes); anything else fails the
// lint step when its severity is LintError.

// LintSeverity ranks a LintFinding: LintError fails `zever db migrate
// --lint`, LintWarn is reported but still lets the migration proceed.
type LintSeverity string

// Lint severities returned by LintStatements.
const (
	// LintError marks a finding that fails the lint step (destructive or
	// long-blocking DDL): DROP COLUMN/TABLE, NOT NULL without a default,
	// and column type rewrites.
	LintError LintSeverity = "error"
	// LintWarn marks a finding that is reported but does not fail the lint
	// step: a non-concurrent index build on postgres.
	LintWarn LintSeverity = "warn"
)

// Lint rule codes emitted by LintStatements. Each code is also the pragma
// token that waives it: `-- zever:allow:drop_column` suppresses only the
// drop_column finding on that statement, while a bare `-- zever:allow`
// suppresses every finding on it.
const (
	// LintDropTable flags DROP TABLE: destroying a table destroys its data.
	LintDropTable = "drop_table"
	// LintDropColumn flags ALTER TABLE DROP COLUMN: destroying a column
	// destroys its data.
	LintDropColumn = "drop_column"
	// LintNotNullNoDefault flags ADD COLUMN ... NOT NULL without a DEFAULT:
	// on a non-empty table postgres must rewrite every row and the
	// statement fails outright when existing rows have no value.
	LintNotNullNoDefault = "not_null_without_default"
	// LintNonConcurrentIndex flags a postgres CREATE INDEX without
	// CONCURRENTLY: the build takes a lock that blocks writes for its whole
	// duration. Emit the index with PlanOptions.ConcurrentIndexes instead.
	LintNonConcurrentIndex = "non_concurrent_index"
	// LintTypeRewrite flags ALTER COLUMN TYPE / MODIFY COLUMN: postgres
	// rewrites the whole table and the conversion can fail or truncate.
	LintTypeRewrite = "type_rewrite"
)

// LintFinding is one linter hit: which rule fired, on which statement, and
// how severe it is.
type LintFinding struct {
	// Severity is LintError (fails --lint) or LintWarn (reported only).
	Severity LintSeverity
	// Code is one of the Lint* rule codes above.
	Code string
	// Statement is the offending DDL statement's text.
	Statement string
	// Message is the human-readable explanation printed by the CLI.
	Message string
}

var (
	lintDropTablePattern    = regexp.MustCompile(`(?i)\bdrop\s+table\b`)
	lintDropColumnPattern   = regexp.MustCompile(`(?i)\bdrop\s+column\b`)
	lintAddColumnPattern    = regexp.MustCompile(`(?i)\badd\s+column\b`)
	lintNotNullPattern      = regexp.MustCompile(`(?i)\bnot\s+null\b`)
	lintDefaultPattern      = regexp.MustCompile(`(?i)\bdefault\b`)
	lintCreateIndexPattern  = regexp.MustCompile(`(?i)\bcreate\s+(unique\s+)?index\b`)
	lintConcurrentPattern   = regexp.MustCompile(`(?i)\bconcurrently\b`)
	lintAlterTypePattern    = regexp.MustCompile(`(?i)\balter\s+column\b.*\btype\b`)
	lintModifyColumnPattern = regexp.MustCompile(`(?i)\bmodify\s+column\b`)
)

// LintStatements lints already-rendered DDL for the named dialect, returning
// one LintFinding per rule hit, in statement order. Findings whose rule code
// is waived by the statement's `-- zever:allow` pragma are suppressed before
// returning. It is pure: no IO, no database, deterministic.
func LintStatements(dialect string, stmts []string) []LintFinding {
	var out []LintFinding

	for _, stmt := range stmts {
		allowed, allowAll := allowedCodes(stmt)

		add := func(sev LintSeverity, code, msg string) {
			if allowAll || allowed[code] {
				return
			}

			out = append(out, LintFinding{Severity: sev, Code: code, Statement: stmt, Message: msg})
		}

		upper := stmt

		if lintDropTablePattern.MatchString(upper) {
			add(LintError, LintDropTable, "DROP TABLE destroys the table and its data; use expand/contract (create the replacement, backfill, then drop) instead")
		}

		if lintDropColumnPattern.MatchString(upper) {
			add(LintError, LintDropColumn, "DROP COLUMN destroys the column and its data; use expand/contract (stop writing, backfill, then drop) instead")
		}

		if lintAddColumnPattern.MatchString(upper) && lintNotNullPattern.MatchString(upper) && !lintDefaultPattern.MatchString(upper) {
			add(LintError, LintNotNullNoDefault, "ADD COLUMN ... NOT NULL without DEFAULT fails on non-empty tables and rewrites every row; add as nullable, backfill, then SET NOT NULL")
		}

		if dialect == atlas.DialectPostgres && lintCreateIndexPattern.MatchString(upper) && !lintConcurrentPattern.MatchString(upper) {
			add(LintWarn, LintNonConcurrentIndex, "CREATE INDEX without CONCURRENTLY blocks writes on postgres; build with ConcurrentIndexes (CREATE INDEX CONCURRENTLY) instead")
		}

		if lintAlterTypePattern.MatchString(upper) || lintModifyColumnPattern.MatchString(upper) {
			add(LintError, LintTypeRewrite, "column type rewrite locks and rewrites the table; use expand/contract (add the new column, backfill, swap reads, drop the old) instead")
		}
	}

	return out
}

// LintPlan lints a computed MigrationPlan's statements against its own
// dialect, for previewing (`--dry-run --lint`) or gating (apply with
// `--lint`) without applying anything.
func LintPlan(plan *MigrationPlan) []LintFinding {
	if plan == nil {
		return nil
	}

	return LintStatements(plan.Dialect, plan.Statements())
}

// allowedCodes parses a statement's `-- zever:allow` pragma: a bare
// `-- zever:allow` waives every rule on that statement (allowAll), while
// `-- zever:allow:<code>[,<code>...]` waives only the listed codes. Matching
// is case-insensitive on the marker; codes compare exact (lowercase).
func allowedCodes(stmt string) (map[string]bool, bool) {
	allowed := map[string]bool{}

	lower := strings.ToLower(stmt)
	idx := strings.Index(lower, "zever:allow")

	if idx < 0 {
		return allowed, false
	}

	rest := strings.TrimSpace(lower[idx+len("zever:allow"):])
	rest, _, _ = strings.Cut(rest, "\n")

	if rest == "" {
		return allowed, true
	}

	if !strings.HasPrefix(rest, ":") {
		return allowed, true
	}

	rest = strings.TrimPrefix(rest, ":")

	for _, code := range strings.Split(rest, ",") {
		code = strings.TrimSpace(code)
		code, _, _ = strings.Cut(code, " ")
		code = strings.Trim(code, "-;")

		if code != "" {
			allowed[code] = true
		}
	}

	return allowed, false
}
