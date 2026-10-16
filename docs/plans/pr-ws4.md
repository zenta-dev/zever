# PR WS4: ORM matrix + explain + bench harness

## What

- `orm/DIALECTS.md`: dialect capability matrix generated from the
  `orm/dialect` capability methods (54 rows: sqlite 3.46 default vs
  postgres 16 default, version floors in cells), plus the end-to-end
  migration workflow and the destructive-change policy. No ADR: every
  policy rule already exists in `diff.go` / `lint.go` / `rollback.go`;
  the doc documents code rather than inventing policy.
- `orm/dialects_test.go`: `TestDialectMatrixMatchesDoc` fails on drift
  (exact cell match + live probe of each capability method on the
  defaults + `JSONDialect` non-implementation pin + placeholder and
  `MaxParams` pins). Pattern follows `config/readme_test.go`.
- `orm/debug_sql.go`: `Query.DebugSQL(dialectName) []DebugStmt` renders
  exactly what `Stream` would execute (same gates, same IN-chunk
  fan-out) without a connection. Args stay bound; dev-only printing
  documented. `streamWheres` gained a caller tag so `Stream` and
  `DebugSQL` keep their own `orm:` error attribution (unexported
  signature change only).
- `orm/bench/`: runnable harness (zever vs hand-written SQL, 4 shapes,
  `results.txt` committed) + `ormcompare`-tagged GORM/sqlc stubs that
  skip with `TODO(verify)`.
- `orm/README.md` Debugging section, `CHANGELOG.md` Unreleased entry.

## Evidence

- `go test ./orm/...` clean (sqlite `:memory:`; postgres pass needs
  `POSTGRES_DSN`, unavailable here).
- `go test -race ./orm/...` clean.
- `golangci-lint run orm/...`: 0 issues. `gofmt`, `go vet` clean.
- Bench: `go test ./bench/ -bench=. -benchmem -run='^$' -count=1`
  (machine: i5-11400H laptop, 16 GB, go1.27.2). ORM tracks raw SQL
  within noise; constant extra allocs (+6 to +27/op by shape). No
  cross-tool wins claimed.
- Code: 395 lines (`debug_sql.go` + 2 test files); bench harness
  separate; docs excluded.

## Risk + rollback

No public interface changes; one unexported signature change
(`streamWheres` + tag, 2 call sites). Rollback: revert the single
commit. No migrations, no config changes, no new dependencies
(`orm/go.mod` untouched).

## Breaking / security

Breaking: no. Security: no (DebugSQL never interpolates args;
secrets-printing warning in doc comment and README).
`TODO(verify)`: GORM numbers, sqlc generated-code comparison,
postgres bench pass.
