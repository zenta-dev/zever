# fix(middleware): cover timeout/agent panic-recovery paths

## What

Adds `core/middleware/floor_cover_test.go` with focused tests for the nine
previously uncovered statements in the `core/middleware` root package, plus
an end-to-end pin of the #605/#622 panic-recovery path
(`Recover(Timeout(panicking handler))` → fixed 500 JSON, redacted log).
Test-only change; no production code touched, floor in
`tools/coverage-core.sh` unchanged.

## Why

`tools/coverage-core.sh` (`core coverage floor` in CI) failed on main:
`core/middleware` measured 97.8% against the 98% floor. PRs #605 (recover
handler panics in Timeout goroutines and agent fan-out) and #622 (redact
and cap panic values in Recover logs) added recovery code whose edge
branches had no tests. The single failure blocks every other open PR
(640, 641, 642, 644).

Uncovered lines found (`go test -coverprofile` on `core/middleware .`):

- `timeout.go:83` — silent handler (no write at all) through Timeout.
- `timeout.go:144` — `Write` without prior `WriteHeader` (implicit 200).
- `redact.go:46` — truncation cut landing mid-rune (backtrack loop).
- `cors.go:50,54,59,70` — `Validate`: empty methods, blank origin, blank
  method, negative MaxAge.
- `ratelimit.go:121` — blank/whitespace `X-Forwarded-For`.
- `securityheaders.go:83` — HSTS `preload` directive.

Tests added (`floor_cover_test.go`, deterministic, no sleeps, no network,
`t.Context()`, `t.Parallel`):

- `TestTimeout_ImplicitOKDefaultsTo200`
- `TestTimeout_SilentHandlerDefaultsTo200`
- `TestRecover_TimeoutGoroutinePanicResponds500` (secret-bearing panic →
  500 + `[REDACTED]`, no leak)
- `TestRedactPanicValueTruncatesOnRuneBoundary`
- `TestCORS_validateRejectsShapes` (4 subtests)
- `TestRightmostUntrusted_BlankHeader`
- `TestSecurityHeadersHSTSPreload`

The agent fan-out half of #605 (`core/agent` `safeDispatch`) already has
its test (`core/agent/fanout_test.go` from #605); scope here stays in
`core/middleware` per the task.

## Evidence

- Before: `coverage: 97.8% of statements` (`go -C core/middleware test
  -cover -count=1 .`), 9 zero-count blocks.
- After: `coverage: 100.0% of statements`, 0 zero-count blocks.
- `go test -race -count=1 ./...` in `core/middleware`: ok (both packages).
- New tests re-run with `-count=2`: pass.
- `gofmt -l` clean, `go vet ./...` clean.
- Floor in `tools/coverage-core.sh` NOT lowered (still 98).

## Risk + rollback

Risk: negligible — test-only, no production code, no config, no API
change. Rollback: revert this commit; coverage returns to 97.8% (gate
fails again, nothing else changes).

## Breaking: no. Security: no.

No public API, config, defaults, or serialization changes. One test
asserts a DSN-shaped panic value is redacted in logs (defense in depth,
already-shipped behavior from #622).
