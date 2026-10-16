# PR WS1: positioning rewrite and comparison

- Branch: `agent/ws1-positioning`
- PR: https://github.com/zenta-dev/zever/pull/637 (base: main)
- Title: `docs(readme): positioning rewrite and comparison`

## What/why

Rewrite top of README so first screen answers what zever is, who it is
for, and why not sqlc/ent/Goa/Encore/plain chi+sqlc. Add real `zenorm`
output, Core vs Extended tiers linked to STABILITY.md, a "What zever will
not do" section, and an honest `docs/comparison.md` stating where each
alternative is better.

## Commits

- `docs(readme): positioning rewrite with why-not table and real zenorm output`
- `docs(readme): add honest comparison page for sqlc ent goa encore hand-wired`
- `docs(readme): changelog entry for positioning rewrite`

## Evidence

- `go run ./cmd/zever check /tmp/opencode/ws1demo/task.zen` -> `OK 1 file(s) valid`
- `go run ./cmd/zever compile --backend=zenorm --out ./generated task.zen`
  (zever v0.7.1) -> output pasted verbatim into README quickstart
- `compile --backend=openapi` and `db migrate --dry-run --adapter=sqlite`
  run to verify quickstart claims (OpenAPI JSON, Atlas HCL for `tasks`)
- `bash tools/check-doc-snippets.sh` -> `all Go blocks clean`
- Link-inspection script over README.md + docs/comparison.md -> every
  target resolves (MISS hits were Go-code regex false positives)

## Risk + rollback

Docs-only, no code, no public API change, no new deps. Roll back by
reverting the three commits above.

- breaking: no
- security-sensitive: no

## Unverified

- Published site URLs (`zenta-dev.github.io/...`) carried over unchanged;
  not re-checked live.
