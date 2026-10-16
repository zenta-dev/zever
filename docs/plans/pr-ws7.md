# PR WS7: testing and quality — PR notes

- Branch: `agent/ws7-testing` (worktree
  `/home/vee/Zenta/zever/.worktrees/agent-ws7-testing`, based on origin/main).
- PR: https://github.com/zenta-dev/zever/pull/645
  (`ci(test): live conformance, mutation pilot, coverage gate, nightly fuzz`,
  base `main`).
- Commits (3, conventional, no attribution):
  1. `test(parser,render): kill 5 surviving mutants from mutation pilot`
  2. `test(cache,queue,storage): skip-gated live conformance tests`
  3. `ci(test): live conformance, core coverage gate, nightly fuzz`
- Diff vs origin/main: only additions (2 workflows, 3 live tests, 2 mutant
  tests, coverage script, mutation report, 2 corpus seeds) + CHANGELOG
  `## [Unreleased]` entry. `ci.yml` and all existing jobs untouched.

## Local evidence (all green where runnable without docker/network)

- `go test -count=1`: dsl/parser, orm/render, adapters/cache/redis,
  adapters/queue/redis, adapters/storage/s3.
- New live tests SKIP without env; non-TTL kit paths verified over TCP
  miniredis. TTL/delayed paths cannot be validated on miniredis (virtual
  clock) — need real services.
- `tools/coverage-core.sh`: PASS, 43 core packages at/above floor.
- `gofmt -l` clean on all touched dirs; both new workflows parse (yaml.v3).
- Each mutant-killer test verified FAIL-under-mutant then PASS-after-revert.

## CI watch

- `gh pr checks --watch` on #645; fix up to 4 rounds, rebase on origin/main
  on conflict (never touch main checkout, never force-push main).
- Known risk: minio `docker run` step and real-redis TTL/delayed paths are
  first-run code paths — TODO(verify) items in the PR body.

## Human-review needs

- Mutation method was manual (offline); consider a gremlins follow-up run.
- `core/eval` floor is 64 — low but current; ratchet deal is documented.
- Whether `coverage-core` should eventually merge into `ci.yml` ci-gate
  (kept separate per WS7 "new workflows only" rule).
