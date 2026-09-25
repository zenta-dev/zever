# AGENTS.md

Single-module Go framework: `github.com/zenta-dev/zever`, `go 1.27.0`. Schema-driven scaffolding compiler (`.zen` DSL) + swappable backend adapters. No nested `go.mod`.

## Commands

```sh
make setup      # once: pinned golangci-lint v2.13.2, govulncheck v1.8.0, cyclonedx-gomod v1.12.0
make build      # go build ./...
make test       # go test ./...
make test-race  # go test -race ./... (local `make check` runs this; CI PR fast-gate does NOT)
make fmt / make fmt-fix   # gofmt check / write
make lint / make lint-fix # golangci-lint run (config `.golangci.yml`)
make vet
make vulncheck
make tidy-check # go mod tidy -diff
make generate   # go run ./tools/gengrammar — required after touching `token.Keywords` or `resolver.ScalarTypeNames`
make check      # full local gate: download, fmt, vet, tidy-check, lint, test-race, vulncheck, build
```

Focused tests:

```sh
go test ./orm/...
go test ./container/... -run TestName -v
go test -race ./config/...
go test ./internal/dsl/compile/ -run TestName -update  # regen committed backend output only
```

## Architecture (ownership)

- `config/` — layered typed config: `defaults < file < env`. `Default()` = zero-infra adapters, deterministic dev crypto key (never prod). `Load(path)` strict YAML/JSON; `Load("")` probes `zever.yaml/.yml/.json`. Env = no prefix `<SERVICE>_<FIELD>` (e.g. `DB_MAXCONNS`), best-effort: ignore unknown vars, error only on unparsable resolved field. Never log raw option maps; use `RedactedServices`/`Redact`. Table: `config/README.md`.
- `container/` — `container.New(cfg)` (nil cfg valid), lazy per-service accessors (`c.DB()`, `c.Cache()`…), retry-on-error (cache cleared). `Job()` = dispatcher over resolved `Queue` (no adapter); `Scheduler()` shares `Cache`/`Queue` (resolves them as side effect). `Close(ctx)` closes only resolved services, dependency order; probes `Close(ctx)` → `Close()` → `Stop()` → `Shutdown(ctx)`; `errors.Join`; panics → `*ClosePanicError`, timeouts → `*CloseTimeoutError`. Tests enforce `goleak.VerifyTestMain`; keep timeout tests short (~100ms parent / ~500ms block). Table + close diagram: `container/README.md`.
- Top-level packages (`auth/ db/ cache/ queue/ session/ payment/ storage/ …`) — one small interface + self-registering adapters each. Container is sole resolver. Don't add methods to public interfaces (breaks implementers); prefer new interface.
- `internal/dsl/` — `.zen` compiler frontend, internal-only (runtime must not import): `lexer → parser → ast → resolver → ir.Schema`, `compile → backend.Generate`. Parser never panics (best-effort + diagnostics + depth guard). Canonical fixture `internal/dsl/compile/testdata/app.zen`.
- `orm/` — generics typed builder over `db.DB` (`NewTable`/`NewColumn`, `From`), no raw SQL. Dialect (`sqlite`/`postgres`) per-call from `conn.Dialect()`; unsupported features fail closed with `dialect.ErrUnsupportedByDialect`; errors prefixed `orm:` and `errors.Is`-compatible. Default tests sqlite `:memory:`; postgres pass needs `POSTGRES_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable'`.
- `cmd/zever/` — Bubble Tea TUI-first CLI. Bare TTY → dashboard; bare off-TTY → usage to stderr, exit 1 (never hang). Errors → stderr, stdout clean. `dev` = local watch only; `tinker` = dev-only arbitrary-Go eval with stderr warning via `cmd/tinker-shim` — never prod data. Map: `cmd/zever/README.md`.

## Conventions (repo-specific, differ from defaults)

- Exported identifiers need doc comment starting with name; unexported don't.
- `goimports` local prefix `github.com/zenta-dev/zever`; `depguard` denies `github.com/pkg/errors` (use stdlib + `%w`) and `io/ioutil`.
- No globals / no `init()` wiring; `ctx` first arg for IO, never stored in struct; goroutine-safe types documented + explicit exit path.
- Tests deterministic: no `time.Sleep` sync, no network, no unseeded randomness, no timing assertions.
- Test helper roles: `stub*` = test doubles/fakes, `must*` = do-or-`t.Fatal` assertion helpers, `freshAdapter` = registry fixture adapter. Test-file suffix encodes purpose (`cover`/`live`/`bench`/`internal`/`race`/`closure`); plain `*_test.go` otherwise.
- Test style: `t.Fatal` aborts setup, `t.Error` continues table assertions (no testify); table vars `tests`/`cases` both accepted; `t.Helper` required on helpers; `t.Context()` in tests (`b.Context()` in benchmarks); error assertions via `errors.Is`/`As`, `strings.Contains` for CLI output; `t.Parallel` except example/live/determinism/close tests.
- Serialization tags: `json`/`toml`/`yaml` (snake_case) required on file-decoded Options and JSON-serialized wire types; Go-API-only structs stay untagged; backends map storage keys explicitly.
- Time: durations are `time.Duration` (cookie `MaxAge` int-seconds excepted for `net/http`); no magic durations — package-level `Default*Timeout/Interval/Delay` consts; pool options use `MaxConn*` prefix order.
- Conventional Commits (`feat/fix/perf/refactor/test/docs/ci/build:`), one logical change/commit; user-facing change → `CHANGELOG.md` `## [Unreleased]` before PR.
- Releases bump every version pin together (root, LSP, editors, docs — file list: `.github/CONTRIBUTING.md` release checklist); tag both `vX.Y.Z` and `tools/zever-lsp/vX.Y.Z`. `internal/dsl` has no release version (schema `v1`/`v2` are API versions).
- Public API changes (exports, config structs, defaults, serialization) need explicit maintainer review; pre-1.0 `0.x` may break with release-note docs.
- CI note: `ZEVER_CHROMEDP_NO_SANDBOX=1` set in CI for document/local render tests; `go test` runs with `-vet=off -count=1` (vet is separate step); race+coverage gate runs only on `push` to main/tags, not PRs.
- Accepted exceptions (deliberate, not drift): external-mirror shapes keep upstream spelling (`go-redis` `ConnMax*` fields, stdlib `SetConnMax*`, posthog `DistinctId`, proto-mirror `UserId`, cookie `MaxAge` int-seconds for `net/http`); env-var rule covers service config only (test DSN vars and ambient `NO_COLOR`/`TERM`/`CI`/`FORCE_COLOR` exempt); `check:boundaries` is the deliberate alias pair of `check-boundaries`; backend-invariant panics live only in `gogen/render_validate.go` (resolver-invariant guards) — the parser itself stays panic-free; `authz` `GetId` mirrors protoc-gen-go output; `password` adapter string `argon2id` is PHC-canonical (dir `argon2` is package path); `cache` `Encode`/`Decode` name the `codec.Codec` operation, lowercase `typed get/set` names the wrapper; `ollama.New` takes `ollama.Options`, container bridges from `ai.Options` (`BaseURL→Addr`); generated server template consts use `DefaultGRPC*`; infallible leaf constructors stay error-free (`log`/`observability` noop `New()`, logger `New(opts)` with fallbacks validated at `Open`, `cookie.New` stdlib builder, `billing/stub` `New()`+validating `Open`, `container.New` lazy root); unexported adapter structs are conventionally named `driver` with receiver `d` (10 packages); `compile --backend` names DSL codegen backends (atlas/gogen/…), not service adapters; near-duplicate helpers stay duplicated when semantics differ (`s3opts.ValidateEndpoint` vs `endpoint.ValidateURL`, `redis.RedactAddr` vs `config.Redact` — cycle, `databaseDSNPreview` display contract); `secrets/env.New` takes local `env.Options` (sole secrets adapter).
