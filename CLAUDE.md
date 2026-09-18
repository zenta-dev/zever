# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

zever is a schema-driven scaffolding compiler for Go. You describe services in a
proto-inspired custom DSL (`.zen` files), and zever compiles that schema into
database access, caching, queues, routing, and other backend plumbing. It never
generates or dictates business logic — only the infrastructure that logic runs on.

Every backend concern (db, cache, queue, auth, storage, ...) is a small interface
with multiple swappable adapters, self-registered and lazily resolved through a
shared `container.Container`. Swapping an adapter is a config change, not a code
change.

## Commands

```sh
make setup        # install pinned golangci-lint, govulncheck, cyclonedx-gomod (once)
make build         # go build ./...
make test          # go test ./...
make test-race     # go test -race ./...
make bench         # go test -bench=. -benchmem ./...
make cover         # coverage.out + total
make fmt / fmt-fix # gofmt check / apply
make lint / lint-fix
make vet
make vulncheck
make tidy-check
make check         # full local CI mirror: download, fmt, vet, tidy-check, lint, test-race, vulncheck, build
```

Single package / single test:

```sh
go test ./orm/...
go test ./container/... -run TestName -v
go test -race ./config/...
```

`orm` tests run on sqlite `:memory:` by default; set `POSTGRES_DSN` to also
run the postgres integration pass:

```sh
POSTGRES_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable' go test ./orm/...
```

Regenerate editor grammar files (nvim/vscode) from the DSL lexer/resolver after
touching `token.Keywords` or `resolver.ScalarTypeNames`:

```sh
make generate      # go run ./tools/gengrammar
```

`golangci-lint` config is `.golangci.yml`; `goimports` local-prefix is
`github.com/zenta-dev/zever`. `depguard` denies `github.com/pkg/errors` and
`io/ioutil`.

## Architecture

### Layer 1: config (`config/`)

Layered, strictly-typed configuration: `defaults < file < env`.

- `config.Default()` — zero-infrastructure adapters (memory, local, sqlite, log,
  noop, ...) so tests run with no external services. Ships one deterministic
  dev-only crypto key (never for production).
- `config.Load(path)` — decodes YAML/JSON **strictly**: unknown services or
  fields are errors. `Load("")` probes `zever.yaml`/`.yml`/`.json` in the cwd.
- Env vars — **no prefix**, `<SERVICE>_<FIELD>=value` (e.g. `DB_MAXCONNS=20`,
  `AUTH_JWT_SECRET=...`). Env matching is **best-effort**: unknown variables are
  silently ignored (so a hostile/noisy process environment can't break startup);
  only values that fail to parse for a *resolved* field error out.
- Never log raw option maps (may hold secrets in plain strings) — always go
  through `RedactedServices`/`Redact`.
- Full adapter-per-service table: `config/README.md`.

### Layer 2: container (`container/`)

`container.New(cfg)` builds one process-wide `Container`; each service resolves
lazily on first accessor call (`c.DB()`, `c.Cache()`, ...) and caches the result
(clearing on error so the next call retries). No globals, no `init()` wiring —
a server and a worker each build their own `Container` from the same config.

- `Job()` has no adapter of its own — it builds a `*job.Dispatcher` over the
  already-resolved `Queue`. `Scheduler()` injects the resolved `Cache`/`Queue`
  so it shares connections instead of opening new ones; resolving it resolves
  `Cache`/`Queue` as a side effect.
- `Close(ctx)` only closes services actually resolved, in dependency order
  (dependents holding cache/queue refs → other snapshots → cache/queue leaves →
  grpc server). Shutdown shape is probed per service: `Close(ctx) error` →
  `Close() error` → `Stop() error` (scheduler) → no-op. Shared instances close
  once via pointer-identity dedup. Failures join via `errors.Join`; panics become
  `*ClosePanicError`, timeouts `*CloseTimeoutError`.
- `container` tests assert `goleak.VerifyTestMain` — no goroutine may outlive
  package tests. Keep timeout-test deadlines short.

Full accessor table and close-order diagram: `container/README.md`.

### Layer 3: business-logic packages

Each top-level package (`auth/`, `db/`, `cache/`, `queue/`, `session/`, `payment/`,
`storage/`, `permission/`, `workflow/`, ...) defines one small interface plus
self-registering adapters, matching the service table in `config/README.md`.
The container is the only place that resolves an interface to a concrete adapter.

### Schema compiler (`internal/dsl/`)

Compiler frontend for the `.zen` schema language, **internal**: runtime packages
must not import it (mirrors Go's `internal/` visibility rule).

```
source text → lexer → parser → ast → resolver → ir.Schema
                                    ↘ compile → backend outputs
```

| Package | Role |
|---|---|
| `diag` | Positions, diagnostics, multi-error lists (zero-dep foundation). |
| `token` | Token vocabulary + keywords. |
| `lexer` | Bytes → tokens; never panics; fuzz-tested. |
| `ast` | Pure-data syntax tree with positions on every node. |
| `parser` | Best-effort parse with per-decl error recovery + depth guard. |
| `naming` | Case converters for backends. |
| `resolver` | Multi-pass `[]*ast.File` → `*ir.Schema`, keep-going diagnostics. |
| `ir` | Resolved schema + gRPC-canonical `ErrorCode` table. |
| `format` | Token-gap formatter; refuses lex-dirty input; idempotent. |
| `breaking` | Old-vs-new schema compatibility diffing (`Change` taxonomy). |
| `compile` | Filename-sorted parse → resolve → `backend.Backend.Generate`. |
| `backend` | `Backend` interface; concrete backends (e.g. gogen, protogogen) live under it. |
| `gengrammar` | Renders `editors/nvim`/`editors/vscode` grammar from `token.Keywords`/`resolver.ScalarTypeNames`; invoked by `make generate`. |

Entry points: `parser.New(file, src).ParseFile()` → `resolver.Resolve(files)` →
`*ir.Schema`; or `compile.Compile(files, backends...)` for the full pipeline.
Canonical fixture: `internal/dsl/compile/testdata/app.zen` (a two-entity,
two-service schema with a `has_many`/`belongs_to` relation pair, used by
committed-output integration tests — regenerate with `go test ./internal/dsl/compile/ -run <TestName> -update`).

The parser never panics on malformed input; it always produces best-effort
output plus diagnostics.

### Typed ORM (`orm/`)

Generics-typed SQL query builder over `db.DB` — not raw string SQL. Ported from
an earlier `zen` package; MySQL was dropped (no MySQL adapter exists in `db/`).

- `orm.NewTable[T]`/`orm.NewColumn[T, V]` define typed table/column handles;
  `orm.From(Table)` builds queries. Model types implement `Scan(orm.Row) error`.
- Dialect (`sqlite`/`postgres`) is resolved per-call from `conn.Dialect()` — no
  silent fallback. Placeholder/quoting rules and capability gates live in
  `orm/dialect`; rendered SQL text lives in `orm/render` (args always bound,
  identifiers always quoted).
- Features a dialect lacks fail closed with `dialect.ErrUnsupportedByDialect`
  (e.g. `DISTINCT ON` and strong lock modes are postgres-only) — they never
  silently emit wrong SQL.
- All errors carry an `orm:` prefix (`orm/render:`, `orm/dialect:`) and stay
  `errors.Is`-compatible.

### CLI (`cmd/zever/`)

TUI-first shell (Bubble Tea) over the whole toolchain. Bare `zever` on a TTY
opens an interactive dashboard; every dashboard row has an exact CLI equivalent
shown on-screen, and off-TTY (pipes/CI) it prints usage to stderr and exits `1`
rather than hanging on a prompt.

Command groups: Scaffolding (`new`, `generate`, `extract`), Inspection
(`compile`, `check`, `breaking`, `fmt`, `doctor`, `routes`, `explain`,
`check-boundaries`, `graph`), Runtime (`serve`, `dev`, `queue:work`,
`schedule:run`, `tinker`), Database (`db migrate`, `db rollback`, `db seed`).

- `zever dev` is local watch-mode only (restarts the server entrypoint on
  schema/source change).
- `zever tinker` is **development-only**: evaluates arbitrary Go against a live
  container via a per-project shim (`cmd/tinker-shim`, scaffolded with
  `zever generate tinker`). It always prints a stderr warning before running —
  never point it at production data.
- All errors go to stderr; stdout stays clean for command output. Full
  screen-to-CLI map: `cmd/zever/README.md`.

## Coding standards (from CONTRIBUTING.md)

- Every exported identifier needs a doc comment starting with its name;
  unexported identifiers don't.
- Favor small, single-behavior interfaces; accept the smallest interface that
  satisfies the need. Adding a method to a public interface can break every
  downstream implementer — prefer a new interface instead.
- No unnecessary abstractions or dependencies: don't add an interface, package,
  or dependency until there's a concrete second use case. Prefer the stdlib.
- No global mutable state. Take `context.Context` as the first arg of any
  IO/cancellable function, pass it through, never store it in a struct.
- Any type usable from multiple goroutines must be made safe and documented as
  such; every goroutine needs a defined exit path (context cancellation or stop
  channel).
- Tests must be deterministic: no `time.Sleep` for synchronization, no network
  access, no unseeded randomness, no timing-dependent assertions.
- Public API changes (exported funcs/types/methods/interfaces, config structs,
  default behavior, serialization formats) need deliberate, explicit review —
  see "API Compatibility" in `.github/CONTRIBUTING.md` before changing any of
  these while the project is pre-1.0.
- Conventional Commits (`feat:`, `fix:`, `perf:`, `refactor:`, `test:`, `docs:`,
  `ci:`, `build:`); one logical change per commit. User-facing changes get a
  `CHANGELOG.md` entry under `## [Unreleased]` before opening a PR.
