# PR plan: WS3 diagnostics with positions and did-you-mean (agent/ws3-dsl)

## Scope (one PR, diagnostics focus)

- `dsl/diag/suggest.go`: `Closest` + `HintSuffix` (Damerau-Levenshtein,
  thresholds mirror `config/suggest.go`; duplication documented, zero-dep kept).
- `dsl/diag/snippet.go`: `Excerpt` + `Render` (header `file:line:col` plus
  source line + caret; total render never fails on missing/stale sources).
- `dsl/resolver/suggest.go` + message-only hint wiring in entity, message,
  service, relation, validate, job, saga sites. No new sentinels, no new
  diagnostics kinds, no interface changes. Parser untouched (never panics).
- `cmd/zever/compile.go|check.go|breaking.go`: print excerpts via
  `printDiagnosticsWithSources`; old `printDiagnostics` kept as wrapper.
- Tests: `suggest_test.go`, `snippet_test.go`, `resolver_suggest_test.go`
  (28 hint cases + position/no-hint controls).
- Docs: breaking CI workflow (`resolver-ir.mdx`), extend-around-generated-code
  (`customizing-and-leaving.mdx`), `docs/adrs/2026-10-16-wasm-playground.md`
  (proposal only), CHANGELOG Unreleased.

## Deliberately out

- Parser changes, new error kinds/sentinels, gogen output changes (golden
  tests already exist: `TestGoldenFiles`, `TestGoldenSagaFile`).
- WASM implementation (ADR only). Generated-code extension ADR (a clean
  story already exists: generated interfaces + write-once stubs).

## Verify

- `go -C dsl test ./...`, `go -C dsl test -race ./dsl/...` scope,
  `gofmt -l`, `go vet`, `cmd/zever` build + tests, fuzz smoke on touched
  parser packages (or full `dsl/...` suite), `tools/apidiff` if supported.
- `gh pr checks --watch`, fix <= 4 rounds, rebase on `origin/main` on conflict.
