# Threat model: dsl

Audited against `dsl/` (lexer, parser, ast, resolver, ir, format, breaking,
compile, backend) at WS6 time. The compiler is build-time only: runtime
packages must not import it.

## Assets

- Build integrity: generated servers, migrations, and API surfaces derive
  from `.zen` files. A malicious or malformed schema poisons everything
  downstream.
- Developer-machine availability: the compiler runs on untrusted-shaped
  input during `zever fmt/compile/check` and in editor tooling (LSP).

## Trust boundaries

- `.zen` files are **code**, trusted to the same degree as the Go files they
  generate — but they arrive through the same untrusted channels as any
  source edit (PRs, editor buffers, `compile --check` in CI).
- Generated output is derived artifact: review the schema, not just the diff
  of generated files (stamps carry backend/CLI version/schema sha for drift
  detection via `zever compile --check`).

## What zever guarantees

- The lexer and parser **never panic** on malformed input: best-effort output
  plus diagnostics, with per-declaration error recovery and a depth guard
  (`dsl/parser`, `AGENTS.md`). Fuzz targets (lexer, parser, resolver,
  formatter idempotence) guard the no-panic invariant in CI.
- The resolver is multi-pass with keep-going diagnostics: one bad declaration
  does not suppress or corrupt the diagnosis of others.
- The formatter refuses lex-dirty input and is idempotent (pinned by
  `FuzzFormatIdempotent`); the breaking comparator detects structural
  regressions (enum/relation/index/job/schedule/saga coverage) for
  `zever breaking` gates.
- Backend rendering is explicit: the `gogen` validator-render panics are the
  sole deliberate panics in the repo (resolver-invariant guards in
  `dsl/backend/gogen/render_validate.go`), never reachable from parser input.

## What the caller must do

- Review `.zen` changes like code changes: new entities, exposed fields,
  routes, and jobs are attack-surface changes even when the diff looks like
  "just schema".
- Run `zever compile --check` (drift gate) and `zever breaking` in CI so
  generated output cannot silently diverge from the reviewed schema.
- Treat compiler diagnostics as errors in CI; do not ship with resolver
  errors suppressed.

## Explicit non-guarantees

- The compiler does not sanitize business semantics: an `expose: true` on a
  sensitive field or a permissive route is faithfully generated, not
  questioned. Authorization review of the schema is human work.
- Editor/LSP grammar artifacts derive from `token.Keywords`/
  `resolver.ScalarTypeNames` via `make generate`; stale grammars are a
  correctness (not security) issue.
