# Dialect capability matrix

This table is generated from the code: every row mirrors the capability
methods on `orm/dialect/sqlite` (`sqlite.New()`, pinned to the bundled
3.46.0 library) and `orm/dialect/postgres` (`postgres.New()`, defaulting
to server 16.0). `TestDialectMatrixMatchesDoc` in `orm/dialects_test.go`
fails if the table drifts from those methods -- edit the table and the
code together.

Conventions: `yes` / `no` are the answers on the defaults above;
`yes (FLOOR+)` means version-gated, true on the defaults. Anything a
dialect lacks fails closed at render time with the typed
`dialect.ErrUnsupportedByDialect`, never silently-wrong SQL. `UNION`,
`UNION ALL`, plain `EXPLAIN`, and the `->>` JSON fallback are universal
SQL and deliberately ungated by any capability interface.

The query builder renders `sqlite` and `postgres` only: there is no mysql
rendering and no mysql driver in `db/`. The migration stack additionally
live-diffs `mysql` (see Migration workflow below).

## Capability matrix

| Capability | sqlite | postgres | Notes |
|---|---|---|---|
| WITH (plain CTE) | yes | yes | |
| WITH RECURSIVE | yes | yes | |
| UNION / UNION ALL | yes | yes | Universal SQL, ungated |
| INTERSECT / EXCEPT | yes | yes | |
| RETURNING on DML | yes | yes | |
| EXPLAIN | yes | yes | Universal SQL, ungated |
| EXPLAIN ANALYZE | no | yes | No SQLite version has EXPLAIN ANALYZE |
| MERGE | no | yes (15+) | Arrived in Postgres 15 |
| JSON arrow-text extraction | yes | yes | Ungated fallback, neither dialect version-gates it |
| json_each / json_tree TVF | yes | no | SQLite json1 only |
| jsonb_array_elements set-returning | no | yes | Postgres only |
| RIGHT JOIN | yes (3.39+) | yes | Core SQLite syntax since 3.39.0 |
| FULL JOIN | yes (3.39+) | yes | Core SQLite syntax since 3.39.0 |
| UPDATE FROM join | yes | yes | |
| DELETE USING join | no | yes | No SQLite version has DELETE USING |
| LEFT JOIN inside UPDATE/DELETE | no | no | FROM/USING forms express inner joins only |
| UPDATE with ORDER BY/LIMIT | no | no | Neither grammar allows it |
| DELETE with ORDER BY/LIMIT | no | no | Neither grammar allows it |
| UPDATE/DELETE with OFFSET | no | no | Neither grammar allows it |
| Joined UPDATE/DELETE with ORDER BY/LIMIT | no | no | Neither grammar allows it |
| ON CONFLICT target WHERE | yes | yes | Partial-index arbitration predicate |
| ON CONFLICT DO UPDATE WHERE | yes | yes | Condition on the upsert update clause |
| SELECT FOR UPDATE | no | yes | SQLite has no row locking |
| SELECT FOR SHARE | no | yes | SQLite has no row locking |
| NOWAIT suffix | no | yes | SQLite has no row locking |
| SKIP LOCKED suffix | no | yes | SQLite has no row locking |
| FOR NO KEY UPDATE | no | yes | Postgres-only lock strength |
| FOR KEY SHARE | no | yes | Postgres-only lock strength |
| FOR UPDATE OF table list | no | yes | Postgres-only target list |
| NULLS FIRST/LAST | yes (3.30+) | yes | SQLite syntax arrived in 3.30.0 |
| DISTINCT ON | no | yes | Postgres extension, never emulated |
| TABLESAMPLE | no | yes | Postgres-only FROM extension |
| LATERAL derived-table join | no | yes | No SQLite version has LATERAL |
| Aggregate FILTER clause | yes (3.30+) | yes | |
| ROLLUP | no | yes | |
| CUBE | no | yes | |
| GROUPING SETS | no | yes | |
| array_agg | no | yes | Postgres-only function family |
| string_agg | no | yes | Postgres-only function family |
| group_concat | yes | no | SQLite-only function family |
| In-aggregate ORDER BY | yes (3.44+) | yes | SQLite syntax arrived in 3.44.0 |
| ANY/ALL array predicates | no | yes | Needs the ARRAY constructor |
| Window ROWS frame | yes (3.28+) | yes | SQLite frames arrived in 3.28.0 |
| Window RANGE frame | yes (3.28+) | yes | SQLite frames arrived in 3.28.0 |
| Window GROUPS frame | yes (3.28+) | yes | SQLite frames arrived in 3.28.0 |
| CTE MATERIALIZED modifier | yes (3.35+) | yes (12+) | |
| CTE SEARCH/CYCLE clauses | no | yes (14+) | No SQLite version has them |
| Vector ops (pgvector) | no | yes | SQLite vector legs brute-force in Go |
| tsvector full-text | no | yes | Postgres GIN-ranked flavor |
| FTS5 full-text | yes | no | SQLite virtual-table flavor |
| Lease-claim CAS | yes | yes | Atomic single-row compare-and-set |
| Placeholders | ? | $n | Syntax, not capability-gated |
| Identifier quoting | "..." | "..." | Embedded quotes doubled |
| Max bind params | 32766 (999 pre-3.32) | 65535 | Oversized value-list IN fans out per chunk |

## Migration workflow

End to end, `orm/migrate` diffs a compiled `.zen` schema against a live
database and applies the result. Live diffing covers `postgres`,
`sqlite`, and `mysql`; any other dialect fails closed up front with
`ErrUnsupportedDialect`.

1. Compile the schema to `*ir.Schema` (the same IR the atlas backend
   renders bootstrap DDL from).
2. `Plan(ctx, exec, schema, opts)` computes a `MigrationPlan`: ordered
   DDL plus non-fatal `Warnings`, without executing anything.
3. `LintPlan(plan)` (or `LintStatements`) flags destructive or
   long-blocking DDL. Gate deploys on zero `LintError` findings.
4. Preview with `plan.Statements()` (what `zever db migrate --dry-run`
   prints), then `Apply(ctx, exec, plan)` executes it: each statement
   runs in its own transaction with the plan's lock/statement timeouts
   on postgres, and every applied statement's checksum is recorded in
   `schema_migrations`, so a re-run is a no-op (non-idempotent
   statements like `DROP COLUMN` are never re-executed).
5. Roll back with `ComputeRollback(ctx, exec, n)` (inverse DDL for the
   n newest bookkeeping rows, newest first by id) followed by
   `ApplyRollback`. Lossy inverses carry data-loss warnings; rows with
   no synthesizable inverse are skipped with a notice, never guessed.

CLI equivalents: `zever db migrate [--dry-run] [--lint]
[--drop-columns]`, `zever db rollback --count n`. The CLI is a thin
wrapper; all computation lives in `orm/migrate`.

## Destructive-change policy

Drops are opt-in and loud, never silent:

- `PlanOptions.DropColumns` defaults to false: live columns missing from
  the schema produce no DDL unless the caller opts in (`--drop-columns`
  on the CLI). There is no drop-table detection at all -- removing a
  table is always a manual, reviewed migration.
- `LintDropTable` and `LintDropColumn` are `LintError` findings: any
  plan containing `DROP TABLE` or `DROP COLUMN` fails the lint gate.
  The recommended pattern is expand/contract (create the replacement,
  backfill, then drop).
- On sqlite, changes with no expressible statement (column type or
  nullability changes, adding a foreign key to an existing table) are
  `MigrationPlan.Warnings`, never silent skips and never lossy
  copy-table rebuilds (out of scope by design).
- Rollback inverts only what it can prove: `add_column`,
  `drop_column`, `alter_type`, and NOT-NULL-restoring nullability
  changes warn about data loss before their inverse; anything else is
  skipped with a notice.

No ADR was needed: every rule above already exists in `diff.go`,
`lint.go`, and `rollback.go`; this section documents the code rather
than inventing policy.
