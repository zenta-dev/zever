# ADR: WASM playground (compile `.zen` in the browser)

Date: 2026-10-16
Status: proposed (not implemented)

## Context

The `.zen` compiler frontend (`dsl/lexer`, `parser`, `resolver`, `ir`) is pure
Go with no I/O: `parser.New(file, src).ParseFile()` plus `resolver.Resolve`
turns source text into a schema plus `diag.List`. That shape fits a
browser playground: paste a schema, see diagnostics and resolved output
without installing the toolchain. Backends that shell out (`protogogen`
drives `go tool protoc-gen-go-grpc` as a subprocess) or touch the filesystem
cannot run under WASM; pure renderers (`openapi`, `atlas`, `mcp`, `gogen`
minus gofmt) could.

## Decision (proposed)

Ship a `//go:build js,wasm` entry point behind a new `dsl/playground`
package that exposes `Compile(src) (diagnostics, outputs)` over `syscall/js`,
limited to the pure-Go backend subset. The docs site loads it for an
interactive `.zen` page. No CLI or backend changes: backends opt in by
declaring themselves I/O-free, and subprocess/filesystem backends stay
server-side. This ADR proposes the boundary only; no code is committed.

## Consequences

- New public surface (`dsl/playground`) needs maintainer review per the
  pre-1.0 API-compatibility rule, and a WASM build tag in CI.
- `diag.Position`/`Excerpt` rendering already produces the header-plus-caret
  text a browser UI can display verbatim.
- Editor grammars (`tools/gengrammar`) stay the source of highlighting; the
  playground reuses them rather than inventing a second grammar.

## Alternatives considered

- Server-side compile API: simpler backend coverage (all backends work) but
  adds hosting, auth, and sandboxing for arbitrary schema input.
- Full CLI in WASI: keeps every backend, but subprocess backends
  (`protogogen`) still fail and the runtime is heavier than `js,wasm`.
- Do nothing: the `zever check`/`compile` loop stays local-only; acceptable
  until onboarding friction justifies the maintenance cost.
