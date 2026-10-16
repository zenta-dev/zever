# WS5 Runtime and architecture — PR plan

Branch: `agent/ws5-runtime` (base `main`).
PR title: `feat(runtime): minimal profile, patch-break guard, shutdown tests`.

## What

1. Minimal runtime profile (`db`, `orm`, `router`, `config`, `log`):
   `docs/src/content/docs/reference/minimal-profile.mdx` commits binary-size
   bounds plus fresh transitive dep counts with the exact commands.
   Measured 2026-10-16 (Go 1.27.2, linux/amd64): minimal 419 packages
   (185 third-party) vs full CLI 1706 (1396 third-party); binaries cited
   from `reference/benchmarks` (309.7 MB full, 63.3 MB slim floor).
   Builds on the existing `zever build --adapters` slim mechanism and the
   `minimal|api|full` scaffold profiles; adds no new mechanism.
2. One-command release pins plus generated compatibility matrix:
   `make release-prep VERSION=vX.Y.Z` stays the single pin command
   (VERSION, release-pins.txt, intra-repo requires, `zever new` pins via
   check-version.sh); `tools/apidiff --compat-matrix/--check-compat-matrix`
   generates and verifies
   `docs/src/content/docs/reference/compatibility-matrix.mdx` (13 Core rows).
3. Patch-break guard: `tools/apidiff --check-patch` fails when a released
   patch section newer than `cmd/zever/VERSION` documents a Core-tier
   `**Breaking:**` entry (Extended breaks stay allowed pre-1.0; older
   sections grandfathered). Bad fixture plus
   `TestCheckPatchFailsOnBadFixture` prove it exits 1. CI `patch-break`
   job runs it and the matrix check; wired into `ci-gate`.
4. Container under SIGTERM: `container/shutdown_test.go` (ordered shutdown
   scheduler/cache/queue/db, readiness flips to `ErrDatabaseUnavailable`,
   idempotent second Close) and `container/shutdown_leak_test.go`
   (`//go:build leak`, goleak over the same path).

## Why

Quantify the leave-the-framework story (minimal cost), close the
patch-release hole for Core (apidiff gates minors, nothing gated patches),
and prove SIGTERM shutdown order instead of asserting it in prose.

## Evidence

- `go test -race -run TestContainer_Shutdown_UnderSIGTERM ./container/...`
  (from `container/`): PASS.
- `go test -count=1 -run 'TestPatch|TestCheckPatch|TestCompat' ./tools/apidiff`
  (from `tools/apidiff`): PASS, including the bad-fixture exit-1 proof.
- `go run ./tools/apidiff --check-patch`: exit 0 on the real CHANGELOG.
- `go run ./tools/modgraph --check`: clean (to be re-run pre-push).
- `tools/check-doc-snippets.sh`: all Go blocks clean.
- `gh pr checks --watch`: to be recorded at PR time.

## Risk and rollback

Low. No public interface method adds; no globals/`init` wiring; `ctx`
first arg on new IO paths. Worst case: revert the five commits in reverse.

## Breaking / security

No breaking changes. No security impact (test-only signal use targets the
test process itself via `signal.NotifyContext`; no new network, config, or
crypto surface).

## TODO(verify)

- Full `go test -race ./container/...` and apidiff suite pre-push.
- `gh pr checks --watch` green; rebase on `origin/main` on conflict.
- Human review: guard scope (Core-only in patches), floor-from-VERSION
  grandfathering, doc numbers.
