# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `docs`: a background jobs overview (how jobs, queue, worker, scheduler, outbox,
  event bus, workflows and sagas fit together), a battery and adapter decision
  guide, a Kubernetes deployment guide with Deployment, Service, PodDisruptionBudget,
  HorizontalPodAutoscaler, ConfigMap and Secret manifests, and an FAQ with a
  cross-cutting troubleshooting list. Docs only.

- `docs(readme)`: positioning rewrite at the top of `README.md` (what/whom,
  why-not table for sqlc/ent/Goa/Encore/plain chi+sqlc, real `zenorm`
  output, Core vs Extended tiers, "What zever will not do") plus a new
  honest `docs/comparison.md`.
- `feat(runtime)`: minimal runtime profile (`config`, `orm`, `core/db`,
  `core/router`, `core/log`) measured at 419 transitive packages (185
  third-party) versus 1706 (1396 third-party) for the full CLI; numbers and
  repro commands live in `docs/src/content/docs/reference/minimal-profile.mdx`.
  Binary sizes are cited from the benchmarks page (full 309.7 MB, slim
  log+router floor 63.3 MB).
- `feat(tools)`: `tools/apidiff` gains `--check-patch` (fails when a released
  patch section newer than `cmd/zever/VERSION` documents a Core-tier
  `**Breaking:**` change; Extended breaks stay allowed pre-1.0 and older
  sections are grandfathered) and `--compat-matrix`/`--check-compat-matrix`
  (generates and verifies
  `docs/src/content/docs/reference/compatibility-matrix.mdx`). CI runs both
  in a new `patch-break` job.
- `feat(container)`: SIGTERM shutdown tests prove ordered draining
  (scheduler, then snapshots, then cache/queue leaves, then db), readiness
  flipping to `ErrDatabaseUnavailable` once the database is down, and no
  goroutine leaks.
- `ci(test)`: live conformance workflow (`.github/workflows/conformance-live.yml`)
  running the shared kits against real services in containers — postgres:17,
  redis:7, S3-compatible minio — via new skip-gated live tests
  (`ZEVER_REDIS_ADDR`, `ZEVER_S3_ENDPOINT`; postgres reuses the existing
  `POSTGRES_DSN` suites), plus a per-package Core coverage floor
  (`tools/coverage-core.sh`, ratchet-up-only) and a nightly long-fuzz workflow
  with cross-night corpus reuse (`.github/workflows/fuzz-nightly.yml`).
  Mutation pilot report at `docs/plans/mutation-report.md` with five new
  mutant-killing tests in `dsl/parser` and `orm/render`.
- `docs(process)`: new contributor docs — `docs/architecture.md`
  (compiler pipeline, container, registries, conformance kits),
  `docs/ai-assisted-development.md` (how AI assistance is reviewed),
  and `docs/plans/good-first-issues.md` (draft pool, not filed on
  GitHub) — plus concrete deprecation timelines and Go support
  window in `STABILITY.md`.
<- `feat(dsl)`: diagnostics carry positions plus source excerpts and
  "did you mean" hints. `diag.Closest`/`HintSuffix` (same thresholds as
  `config`'s suggester) power hints on every unknown type, attribute, field,
  error code, and service/RPC reference in the resolver; `diag.Excerpt`/
  `Render` format `file:line:col` plus the offending line and caret, and
  `zever compile`, `check`, and `breaking` print excerpts from the compiled
  sources. Docs gain a ready-to-copy `breaking` CI workflow, an
  extend-around-generated-code guide, and a WASM-playground ADR (proposal
  only, not implemented).
- `docs(quickstart)`: re-executed every quickstart command in a temp dir and
  fixed the doc to match reality (fresh scaffolds serve fine — the stale
  `unknown adapter: jwt` caution is gone; `zever new` already runs
  `go mod tidy`; the API-call step shows `/healthz` 200 plus the `zever
  routes` listing with a note that RPCs 404 until handlers are written).
  New `quickstart.yml` CI job runs the same commands verbatim in a temp
  directory so future doc drift fails loudly.
- `feat(orm)`: new `orm/DIALECTS.md` dialect capability matrix generated
  from the `orm/dialect` capability methods (sqlite vs postgres, with
  version floors), pinned by `TestDialectMatrixMatchesDoc` which fails on
  drift; the same doc carries the end-to-end migration workflow and the
  destructive-change policy (opt-in drops, lint gate, lossy-rollback
  warnings). New `Query.DebugSQL(dialectName)` renders the exact SQL +
  bound args `All`/`Stream` would execute, without a connection (args
  stay bound; dev-only printing). New `orm/bench` harness comparing the
  ORM against hand-written SQL on a fixed four-shape query set
  (`results.txt` committed; GORM/sqlc comparisons scaffolded behind the
  `ormcompare` tag as `TODO(verify)`).
- `feat(zevertest)`: new `github.com/zenta-dev/zever/zevertest` module with test
  helpers: `NewTestContainer(t)` (zero-infrastructure adapters, per-test sqlite
  and storage under `t.TempDir()`, quiet logging, closed on cleanup), a fake
  `Clock` and deterministic `IDs`, and an in-process HTTP `Client`. The testing
  guide documents it, and CI gains a 20s-per-target fuzz smoke job over the DSL
  lexer, parser, resolver and the new formatter idempotence target.
- `feat(orm)`: typed keyset and MERGE helpers. `orm.AfterTuple2` and
  `orm.AfterTuple3` build the multi-column keyset predicate from typed columns
  and typed values (no length-mismatch error path), and `orm.MergeValueOf`
  is the typed form of `orm.MergeValue` (column and literal share one value
  type). All additive.
- `test(shared)`: benchmarks for `shared/msgspan` (untraced fast path, traced
  producer/consumer span) and `shared/outboxbridge` (queue and eventbus
  publishers, with and without headers). Test-only.
- `docs(system)`: documentation IA pilot — new generated-from-code reference
  pages for the `flag`, `ratelimit`, and `session` batteries (adapters, config
  keys, options, per-battery error indexes), each pinned by drift tests in
  `core/<battery>/reference_test.go`; three page moves with redirects
  (`digging-deeper` strays to `guides/`, `agent-skill` to `guides/extend/`);
  full quadrant map proposed in `docs/plans/pr-ws8.md`.

### Security

- `fix(orm/migrate)`: rollback now validates the stored `Table` and
  `ObjectName` of every `schema_migrations` row as plain (optionally quoted or
  schema-qualified) identifiers, and the `PriorSQL` allowlist for re-creating a
  dropped index is end-anchored to the exact `CREATE INDEX` shape this tool
  generates, so a hand-edited row can no longer splice or append extra
  statements into a rollback.

- `fix(dsl)`: the resolver now rejects `http:` paths that do not start with `/`
  or contain anything outside printable ASCII (no spaces, control characters or
  newlines), and `dsl/backend/gogen` quotes the path in the generated handler
  comment. A crafted `.zen` path could previously inject Go source through the
  comment via lexer-decoded `\n`.
- `refactor(auth,idempotency)`: the jwt adapter, the memory idempotency store and
  the memory revocation store read time through an unexported injectable clock
  (matching the memory session store), so expiry behavior is tested with a fake
  clock instead of real sleeps. No public API or default behavior change.


- `docs(security)`: new per-battery threat models under `docs/security/`
  (`threat-model-<battery>.md` for every Core battery: config, container,
  db, orm, router, cache, queue, session, auth, crypto, password, secrets,
  dsl) stating assets, trust boundaries, zever's guarantees, and caller
  duties — each grounded in the current code, including explicit
  non-guarantees. New ADR (`docs/adrs/2026-10-16-asymmetric-jwt.md`)
  proposes EdDSA + RS256 JWT support with algorithm pinning and key-rotation
  guarantees (proposal only, nothing implemented). Draft-only proposals under
  `docs/plans/` for an OpenSSF Scorecard workflow and for cosign release
  signing + SBOM binding (neither wired; both need human approval).
  `.github/SECURITY.md` gains a no-public-PR rule for unpatched
  vulnerabilities and a scope section. Docs only; no behavior change.

- Docs: `start/versioning` and `start/project-layout` no longer carry stale
  module counts (about 180 modules; 176 workspace `use` entries).


- `container`: resolving the postgres `db` service now logs a warning when the
  DSN leaves TLS optional (`sslmode` unset, `prefer` or `allow`) toward a
  non-loopback host outside a dev `ZEVER_ENV` (dev, test, development), since
  those modes fall back to plaintext. The message never includes DSN content.

- Generated-file provenance stamps no longer embed the zever CLI version
  (`// Code generated by zever compile --backend gogen - schema sha256:...`;
  the `x-zever-generated` JSON block drops `version`), so a release never
  churns committed generated code. `compile --check` still treats stamps from
  earlier releases as version-only drift. All `examples/*/generated` trees were
  regenerated (unstamped and v0.6.1/v0.7.0 stamps are gone) and
  `make compile-check` now gates every example, not just `examples/todo`.


- `core/middleware`: `Recover` and `RecoverUnaryServerInterceptor` now log the
  panic value with credentials scrubbed (URL userinfo, bearer/basic tokens,
  `password`/`token`/`api_key`-style pairs, long opaque IDs) and capped at
  1 KiB, so secrets embedded in panic messages stay out of log sinks.


- **Breaking:** the Redis and DB session stores no longer put the bearer session
  ID in the storage key: keys are now `<prefix>:<hex sha256(id)>`, so key
  listings, MONITOR/slow logs, backups and database dumps cannot be replayed as
  logins. Sessions created by earlier versions are not found under the new
  keys, so every active Redis/DB session ends once at upgrade (users sign in
  again). The memory and cookie stores are unaffected.


- Supply-chain hardening: `release.yml` now defaults to `permissions: {}` and
  grants write scopes only to the publish job (dropping `actions: write`);
  `bench.yml` pins benchstat and no longer interpolates `github.base_ref` into
  shell. `install.sh` fails (and removes the binaries) when `gh attestation
  verify` rejects a download and `gh` is authenticated, unless
  `--insecure-skip-verify` is set, and uses `wget --https-only`. `zever
  upgrade` refuses release downloads larger than 512 MiB instead of buffering
  them (checksum verification was already enforced).

- Repo housekeeping: add `.github/CODEOWNERS` (`@zenta-dev/maintainers`),
  ignore `.claude/` (agent harness state), group all github-actions Dependabot
  updates into one PR (`tools/gen-dependabot.sh`), and remove the empty stub
  `docs/production.md` (nothing linked to it).


- `adapters/session/cookie`: `SameSite=None` now forces `Secure`, even when
  `Secure` is explicitly set to false (browsers reject insecure `SameSite=None`
  cookies, which silently dropped the session).

### Fixed

- `fix(middleware)`: cover panic-redact and timeout-recover paths.
- `fix(outbox)`: the postgres claim statement put `FOR UPDATE SKIP LOCKED`
  inside an `IN` subquery, which postgres rejects (SQLSTATE 42601) — every
  live claim failed. Selection now runs in a `WITH due AS (...)` CTE feeding
  the `UPDATE`. Found by the new live conformance workflow.
- `fix(postgres)`: the `?`→`$n` rewriter mistook `LIMIT ? FOR ...` for a
  JSONB existence check (`FOR` was missing from the keyword list), sending a
  raw `?` to the server. The row-lock keyword cluster is now recognized
  (`KEY` stays unlisted so `payload ? key` keeps working).
- `fix(dsl/format)`: the formatter wrote gap edits at rune columns instead of the
  byte offsets `Apply` expects, so a multi-byte character earlier on a line
  (a non-ASCII string literal) made it insert whitespace inside the character
  and corrupt the file; `OffsetOf` also now counts UTF-16 code units, matching
  the lexer's columns, so astral characters such as emoji no longer shift
  later edits. Found by the new `FuzzFormatIdempotent`.
- `fix(eventbus,revocation)`: the memory eventbus no longer swallows handler
  panics silently when `Options.OnPanic` is unset; it logs the panic value and
  stack through `slog`. The memory revocation store's background pruner now
  recovers per sweep, logs the panic with its stack, and keeps running instead
  of dying silently.


- `go install github.com/zenta-dev/zever/cmd/zever@vX.Y.Z` now works (from the
  next release): `cmd/zever/go.mod` no longer commits `replace` directives and
  pins every sibling module. `tools/modgraph --check` and
  `tools/check-version.sh` fail if a replace reappears in a `go install`
  target, `tools/gotidy.sh` keeps `make tidy-all` / `check` working for such
  modules (temporary local replaces), and the release workflow runs
  `tools/verify-go-install.sh` against the pushed tag. v0.7.1 and earlier
  tags stay uninstallable this way; docs updated accordingly.
- `zever new` scaffolded CI workflow installs the CLI from the release binary
  (`install.sh --version <pin>`) instead of `go install .../cmd/zever@<pin>`,
  which fails for every tag that commits `replace` directives in
  `cmd/zever/go.mod` (v0.7.1 and earlier).
- `perf(router)`: `router.WithOwnedParams` stores a route-params map in the
  request context without the defensive copy `WithParams` makes (the caller
  hands over a fresh per-request map). The stdhttp and fiber drivers now use
  it, cutting the per-request params cost from ~164 ns, 384 B and 3 allocs to
  ~19 ns, 48 B and 1 alloc. `WithParams` is unchanged. Additive.
- `fix(cli)`: shell completion actually works. Cobra's hidden `__complete`
  command, which the generated bash/zsh/fish/powershell scripts call, is now
  routed to Cobra instead of failing as an unknown subcommand, and completes
  `.zen` files, `generate` kinds, `--backend`, `--profile` and `--adapters`
  values, and `--config` files.

### Changed

- `perf(queue)`: the memory queue adapter keeps delayed messages in a min-heap
  on `readyAt`, so each promoter wake-up pops only due entries instead of
  rescanning and reallocating the whole delayed set (100k delayed messages:
  one wake-up now ~370 ns, 1 alloc).
- `fix(middleware)`: `core/middleware` `Timeout` and `TimeoutUnaryServerInterceptor`
  now recover a handler panic on their child goroutine (previously unrecovered,
  which crashed the process despite an outer `Recover`). The HTTP variant
  re-panics on the request goroutine; the gRPC variant returns `codes.Internal`.
  `core/agent` parallel tool fan-out likewise turns a panicking tool into a
  `Run` error instead of crashing the process.

- `feat(ai)`: `ai.Options` (and `ollama.Options`) gain an optional `Logger *slog.Logger`;
  the anthropic, gemini, ollama and openai adapters send their open-time
  `allow_private` / `insecure_skip_verify` notices to it instead of the global
  `slog.Default()` (still the fallback when nil). Additive, never read from
  configuration.
- `core/middleware.RequestLogger` now logs `duration_ms` as a float
  (fractional milliseconds) instead of a truncated integer, so sub-millisecond
  requests no longer log as `0`. Log pipelines that typed the field as an
  integer should accept a number.
- **Breaking:** `fix(storage)`: hardened `adapters/storage/local` and `core/storage`.
  `core/storage` `ValidKey` now rejects ASCII control characters (including DEL).
  The local adapter rejects keys ending in `.meta` or with a segment starting
  `.tmp-`/`.mtmp-` (its own sidecar and staging files), caps a PUT at 64 MiB by
  default (was 2 GiB; new `storage.LocalOptions.MaxUploadBytes` /
  `max_upload_bytes` overrides it, enforced with `http.MaxBytesReader`), only
  echoes stored content types from an allowlist (anything else is served as
  `application/octet-stream`), always sends `Content-Disposition: attachment`,
  and, when no `root` is set, refuses the shared default `/tmp/storage` if it
  is a symlink, not a directory, or owned by another user. `config` schema
  gains `storage.options.max_upload_bytes`.
- `fix(oidc)`: `adapters/auth/oidc` now caps discovery and JWKS response bodies
  at 1 MiB (via `httpclient.ReadLimited`; go-oidc reads them unbounded) and
  throttles repeat fetches of the same URL to one per 10 seconds, so tokens
  with unknown `kid` values can no longer be amplified into a stream of JWKS
  requests to the IdP. A freshly rotated signing key can take up to 10 seconds
  to become visible. The package docs state that the adapter is verify-only.

- `tools/tag-release.sh`: new `--push` publishes every module tag with a single
  `git push --atomic` (a rejected ref leaves the remote with no lockstep tags)
  and the closing hint no longer suggests `git push --tags`; new
  `--verify-proxy` asks the module proxy to resolve each module at the tag,
  retrying while the proxy catches up.
- `feat(cli)`: every usage screen now lists flags in `--name` style and ends
  with the global flags (including `--json`); `zever help doctor` shows the
  real `--config` / `--strict` flags instead of an empty list, and
  `doctor`, `fmt`, `config show` and `new` document their flags in
  `--help` and completion.

## [v0.7.1] - 2026-10-15

### Breaking

- **Breaking:** `fix(payment)!: all adapters use minor units (cents)`: every `Amount`
  field across `core/payment` (`Request`, `Result`) and `core/billing`
  (`Invoice`) is now uniformly minor units, extending the Paddle payment
  normalization above to every other money path. The Paddle billing adapter
  parses totals via the shared `billing.ParseMinorUnits` contract (whole
  units scale by the currency exponent, so bare `"4200"` reads as 420000
  minor units; excess precision rounds half away from zero; unrepresentable
  magnitudes fail with `billing.ErrAmountOverflow`). New
  `billing.FormatMinorUnits` encodes minor units back to major-unit
  decimals with a tested round-trip. `billing.ParseMinorUnits` overflow is
  now exact rather than conservative (representable extrema convert).
  Stripe and stub payment/billing adapters already passed native int64
  minor units through and are unchanged apart from pinning
  normalize/round-trip/refund-symmetry tests.
  Paddle billing totals without decimals change value
  (`"4200"` was 4200 minor units, now 420000); truncation becomes
  half-away-from-zero rounding there too.
- **Breaking:** `fix(gogen,proto)!: wire pagination through gRPC and drop collision-prone helpers`:
  a `paginated: true` RPC's synthesized request message now
  carries `string cursor` and `int32 limit` (numbered after the declared
  params), so gRPC clients can paginate; an RPC whose sole param is a
  message/entity is now wrapped in `<Op>Request` like any other paginated
  RPC, and declaring a param named `cursor` or `limit` on a paginated RPC
  is a resolver error. In generated Go, the service method now takes
  `cursor string, limit int32` (was `Cursor`), the gRPC adapter forwards
  the request's cursor and limit instead of always passing `Cursor("")`
  and `0`, and one generated `normalizePageLimit` applies identical
  bounds on both transports (zero becomes 20, above 100 is clamped
  rather than rejected over HTTP, negative is `InvalidArgument`). The
  exported `Cursor` type, the unused generic `Page[T]` and the exported
  `DefaultPageLimit`/`MaxPageLimit` were removed (now unexported) so they
  cannot collide with user-declared messages or entities.
- **Breaking:** `fix(payment)!: normalize Paddle whole-unit amounts to minor units`:
  the `paddle` adapter now scales whole-unit decimal totals to
  minor units (`"10"` parses to `1000`, matching `"10.00"`), where it
  previously passed them through verbatim (`"10"` parsed to `10`). Sub-cent
  precision now rounds half away from zero (`"10.005"` becomes `1001`,
  `"-10.005"` becomes `-1001`) instead of truncating, and partial-refund
  adjustment amounts are formatted back to major-unit decimals (`1000` is
  sent as `"10.00"`) so both directions agree. Totals that overflow `int64`
  once scaled still fail with `ErrAmountOverflow`. The `stripe` adapter is
  unchanged (native minor-unit integers).
- **Breaking:** `fix(httpclient)`: clients built with `WithSafeDial` /
  `NewSafeClient` no longer honor `HTTP_PROXY`/`HTTPS_PROXY` (the dial guard
  only saw the proxy address, so private targets were reachable through a
  proxy); pass the new `WithAllowProxy()` option to opt back in with a
  trusted proxy. `IsPrivateIP` now also classifies NAT64
  (`64:ff9b::/96`, `64:ff9b:1::/48`), 6to4 (`2002::/16`) and IPv4-compatible
  (`::/96`) addresses by their embedded IPv4 address, and treats Teredo
  (`2001::/32`) and site-local (`fec0::/10`) as private.
- **Breaking:** `fix(document)`: the `latex` adapter now defends against
  local file reads on TeX Live >= 2026 (where `openin_any` is a no-op). New
  `latex_sandbox` option (`auto` default, `required`, `off`) runs TeX inside
  bubblewrap (no network, read-only system trees, only the render dir
  writable); `required` fails at open when bwrap is unavailable, `auto` logs
  one warning and falls back. A source pre-scan now rejects `\input`,
  `\include`, `\openin`, `\openout` etc. with absolute, `..`, `~` or piped
  paths, `\write18`, and shell-escape packages with `ErrUnsafeSource`;
  `latex_allow_unsafe_source: true` restores the old behavior. Output and
  stderr are size-capped before buffering, and process-group kill is split

### Added

- `fix(payment): at-least-once webhook handling`: `WebhookEvent` on the
  `stripe` and `paddle` adapters records the event id as seen before the
  caller processes it (at-most-once), so a failing handler loses the event
  on the provider retry, and events with an empty id are never deduped. New
  optional interface `payment.WebhookHandling` (`HandleWebhook(ctx, raw,
  signature, handler)`) runs the handler under the reservation, commits only
  on success, and releases the key on handler error or panic so retries are
  accepted; an empty event id is rejected with `payment.ErrMissingEventID`.
  `payment.ProcessWebhookOnce` is the shared helper. `WebhookEvent` behavior
  is unchanged and now documented as at-most-once.
- `feat(cli): slim adapter-scoped builds`: the new `zever build`
  command compiles a slim CLI registering only `--adapters=a,b`
  (bare batteries select every known adapter, `battery/adapter` pins
  one; empty builds the full CLI). Unselected adapter packages leave
  the binary via a `go build -overlay` of the central registration
  list, so no build-tagged source variants are committed. Unknown
  names fail listing the choices, `--dry-run` prints the plan, and
  `--json` emits `{out, sizeBytes, adapters}`. A `log,router` floor
  build measures 63 MB against the 310 MB full CLI with warm `--help`
  at ~27 ms vs ~40 ms. The `doctor` adapters meta-check is now scoped
  to the project's enabled batteries (nil scope still checks all),
  so a slim binary passes its own `doctor` for its project.
- `perf(queue/memory): cut round-trip allocs 12 to 1 per op`: the P5-8
  follow-up pass over the documented leftovers. Drained topics are retained
  instead of sweep-deleted and recreated per message (channels and maps
  reused; the sweep still deletes promoter topics so their quit channel
  closes, and now resets the retained heap/ready window since every leftover
  heap entry references a gone inflight id); the ready list pops via a head
  index instead of reslicing past the front (which shrank capacity and forced
  the next Push to reallocate); the visibility heap is hand-rolled instead of
  `container/heap` (whose `any` Push/Pop boxed one entry per Pop). Measured
  `BenchmarkMemoryRoundTrip` 12→1 allocs/op, 1984→16 B/op (parallel
  10/11→1). Deliberately deferred: `Message.Clone`'s payload copy (the last
  alloc, pinned by `TestMemory_CloneIsolation`), `msgspan` wired-provider
  spans (18 allocs/op when telemetry is wired, 0 on the default nil-provider
  path), `traceprop` inject/extract (nil-path 0), and `shared/lrucache`
  (0 allocs/op steady-state on every bench).
- `perf(alloc): P5-8 leftover dispositions, bench-first (i5-11400H,
  `-benchmem -count=3`)`: `traceprop BenchmarkInject` holds at 400 B/op,
  3 allocs/op — memprofile attributes the third alloc to OTel-internal
  `TraceContext.Inject` (string builder plus a `[][]byte` literal), and a
  direct-propagator variant measured byte-identical, so the composite call
  stays and the floor is pinned by upstream OTel. `msgspan` wired Producer
  holds at ~1822 B/op, 18 allocs/op with a stub tracer (roughly two thirds
  in the stub recorder/`SetAttributes` appends, the rest option closures,
  attribute slice, span name, and `traceprop.TraceID`); the default
  nil-provider path stays 0 B/op, 0 allocs/op and is now guarded by
  `TestProducerNilPathZeroAllocs`. `BenchmarkMemoryRoundTrip` remainder is
  the single `Message.Clone` payload copy, now documented on `Clone` as
  isolation-pinned (the Pop clone keeps consumer mutations off the inflight
  copy). `shared/lrucache` re-verified at 0 allocs/op steady-state on
  Get/Put/GetOrCompute. No benchmark deltas by design: only regression
  guards, no behavior change.

### Changed

- `build(release): v0.7.1 replaces v0.7.0`: the v0.7.0 module tags carry
  `go.mod` files that still require sibling zever modules at v0.6.1 and are
  immutable once cached by the Go module proxy. v0.7.1 contains the same work
  plus everything merged afterwards, with every intra-repo `require` pinned at
  v0.7.1; use v0.7.1 and do not depend on v0.7.0 (see `STABILITY.md`). New
  `make release-prep VERSION=vX.Y.Z` (`tools/release-prep.sh`) sets
  `cmd/zever/VERSION`, every pin in `tools/release-pins.txt` (installers,
  `CITATION.cff`, docs, LSP/MCP server versions, VS Code extension) and every
  intra-repo `go.mod` require (`tools/modgraph --set-pins`) in one pass.
  `tools/check-version.sh`, `make modgraph-check` (`--check-pins`) and
  `tools/tag-release.sh` (all modes) now fail when any of them drifts from
  `cmd/zever/VERSION`.
- `docs(stability): document the kid-less JWT token timeline`: `STABILITY.md`
  states the current behavior (kid-less tokens verify against the legacy
  `jwt.secret`), that it is not deprecated and has no removal date, and how
  any future deprecation would proceed.
- `chore: repo hygiene and docs checks`: `tools/check-doc-snippets.sh` now
  compiles and vets Go doc snippets marked ` ```go ... check ` (complete
  `package main` programs) in a temporary workspace module, and one stale
  outbox snippet it caught was fixed. The tracked `tools/modgraph/modgraph`
  and `tools/affected/affected` binaries were removed and built tool binaries
  are git-ignored. `docs/.../zen-syntax.mdx` gained enum, message, and grammar
  sections, and a test in `dsl/gengrammar` fails when the page omits a keyword,
  scalar type, attribute, or block label. The `.golangci.yml` wrapcheck note
  now records measured numbers.

### Fixed

- `fix(cli): zever new --profile full scaffolds a config that passes doctor`:
  the full profile wired `ai` (anthropic, needs an API key), `geo` (static,
  needs a dataset) and `i18n` (embed, needs an embedded FS), so a fresh
  `--profile full` project failed `zever doctor` with 3 FAILs. Those three
  services are now left out; add them with `zever add`.
- `docs(lint): refresh the wrapcheck measurement`: the `.golangci.yml` note now
  records 162 hits for `container` + `core/*` (17 + 145), the reason wrapcheck
  stays disabled.
- `docs(auth): drop duplicated alg-none sentence from the JWT guide`: the
  guide states HS256-only once and no longer repeats the `alg: none` rejection
  note. No code or behavior change; asymmetric algorithms stay unsupported by
  design.
- `fix(cli): zever doctor uses the built server report when batteries fail`:
  a generated server prints its `--doctor` JSON and exits 1 when a battery
  fails; the CLI treated that exit as "server unavailable", fell back to the
  CLI registry, and showed an unrelated log WARN as the reason. It now reads
  the report regardless of exit status and, on a real crash, reports the last
  stderr line.
- `fix(build): exclude agent harness checkouts from the module list`:
  `ALL_MODULES` in `Makefile` now skips `./.claude/*`. Harness checkouts
  under `.claude/worktrees/` each contain full module trees; without the
  exclusion the list ballooned past `ARG_MAX`, `printf` failed, and every
  `*-all` loop silently tested nothing (false green `make check`).
- `fix(cli): resolve the CLI version from ldflags, build info, or VERSION`:
  `zever -V`, `zever new` pins, and generated-file stamps no longer rely on a
  hardcoded constant that lagged the release tags. The version now comes from
  `-ldflags "-X main.cliVersion=..."` (release workflow and `make
  build-release`), then the `go install module@vX.Y.Z` build info, then
  `cmd/zever/VERSION` (now `0.7.0`), which `tools/tag-release.sh` requires to
  match before tagging. `make version-check` (part of `make check`) fails when
  that file is lower than the newest `cmd/zever/v*` tag.
- `fix(cli): zever compile --check ignores the stamped CLI version`: a zever
  upgrade alone no longer reports every generated file as drifted. Files that
  differ only in the stamp version pass with a note (`stampVersionOnly` in
  `--json`); the new `--strict` flag fails on them. Schema hash and content
  changes still fail.
- `fix(container,cmd)`: SIGHUP reload is now atomic and race-free.
  `ReloadableLimiter` retires a replaced limiter and closes it only after its
  in-flight `Allow`/`Reset` calls drain; the boot-time limiter stays owned by
  the container, so no limiter is closed twice at shutdown (the generated
  server also closes reload-opened limiters on exit). `ReloadableLimiter.Swap`
  now owns the swapped-in limiter and callers must not close it.
  `ReloadableLogger` child loggers (`With`/`WithContext`) follow later level
  swaps. New `container.ApplyReload` opens the new logger and limiter first
  and swaps both only on success, so a failed rate-limit reopen no longer
  leaves the log level changed.
- `fix(orm): dedupe oversized IN lists before chunking`: a value repeated in
  a value-list `IN` that is auto-chunked across several statements could land
  in two chunks, so `All`/`Stream` returned the same row twice and `Count`
  double counted it. Repeated values are now removed (order-preserving, NULLs
  collapsed since they never match) before splitting, and `Preload` fetches
  each distinct parent key once. Chunked reads are documented as multiple
  statements that are not snapshot-consistent unless run in a transaction.
- `fix(config,container,cmd): scope adapter validation to enabled services and
  run it at boot`: new `config.(*Config).EnabledServices`,
  `ValidateEnabledAdapters` and `container.(*Container).ValidateEnabledAdapters`
  check only services explicitly configured or at a non-default adapter, so a
  generated server's `-doctor` no longer exits 1 on `unknown adapter
  "anthropic" for service "ai"` for unconfigured services. The generated
  server now validates at startup (one aggregated error with did-you-mean,
  exit non-zero) instead of failing lazily on first use, and `zever doctor`
  surfaces the server's `adapters` row. `ValidateAdapters` keeps checking
  every service.
- `fix(container): share a failed build with parked waiters`: concurrent
  callers waiting on a lazy service build that errors or panics now receive
  that build's error instead of each re-running the builder; a later call
  still retries. The unused builder `context.Context` plumbing is removed and
  the root-context design is documented in `container/README.md`.
- `refactor(registry): drop dead nil-hook branches`: `registry.New` already
  defaults every hook, so the unreachable nil checks in `Register`/`Lookup`
  are gone (no behavior change).
- `fix(cli,container): CLI polish and scaffold orm requirement`: `zever new`
  now requires the `orm` module in the scaffolded `go.mod` (the zenorm
  output imports it); `zever compile` runs `go mod tidy` (or prints a hint
  with `--no-tidy`) when generated code imports framework modules `go.mod`
  lacks; an unknown subcommand prints the error and did-you-mean hint first
  instead of dumping usage; `db migrate --lint` prints error findings as
  `error:` on stderr (warnings stay warnings), `-h/--help` exits 0 without
  `flag: help requested`, and `--dry-run` help now states that it connects
  read-only when `--dsn` is given; the informational `container: pool shared`
  line is shown only when `ZEVER_DEBUG` is set.
- `fix(auth,crypto): warn on in-memory JWT revocation; shared dev-env check`:
  the `jwt` adapter now logs a one-time `slog` WARN (and exposes it via
  `jwt.WarningReporter.Warnings()`) when the default per-process revocation
  store is in use, pointing at `revocation/redis`. Docs and package comment
  state "HS256 only; asymmetric via custom adapter". New
  `crypto.IsDevEnv(value)` trims and case-folds `ZEVER_ENV` against
  dev/test/development; `crypto.DevKeyAllowed` and the CLI's `devEnv` both
  use it, so a padded value like `" Dev "` is no longer treated as
  non-dev by the dev-key gate.
- `fix(cmd/zever): parallel-safe agent catalog tests`: `agentCommands`,
  `agentGlobalFlags`, and `printAgentHelp` gain `*On` variants taking the
  Cobra tree explicitly (mirroring `topLevelNames`), and
  `TestPrintAgentHelp`/`TestUninstallUpgradeWiring` run `t.Parallel`
  against fresh per-test trees. `cobra.Command.Commands` sorts the tree's
  children in place, so parallel reads of the shared `rootCmd` global race
  (reproduced with a throwaway parallel test under `-race`); production
  output is unchanged.
- `fix(payment/paddle): reject decimal totals that overflow int64 minor
  units`: `parsePaddleAmount` now returns the new `ErrAmountOverflow`
  sentinel when the whole part no longer fits after the x100 scale shift or
  the final fraction add/subtract carries past the limit (e.g.
  `"9223372036854775807.99"` previously wrapped silently to a negative
  amount, which could invert refund `amount > total` checks); the exact
  `math.MaxInt64`/`math.MinInt64` totals still convert, and out-of-range
  whole parts that `strconv.ParseInt` already rejects are unchanged. Stripe
  needs no equivalent: it carries amounts as native int64, never decimals.
- `fix(cli): zever new pins every framework module at the framework version`:
  the v0.7.0 release tags carry go.mod files that still require v0.6.1
  siblings, so a published-mode scaffold resolved `shared/registry` at v0.6.1
  and `go build` failed with `undefined: registry.SortedNames`. The scaffold
  go.mod now also requires every shared, core and adapter module at the
  framework version (as `// indirect`; `go mod tidy` drops unused ones).

### Security

- `middleware.ClientIPKey` now reads every `X-Forwarded-For` header line
  (not just the first) before picking the rightmost untrusted entry, so a
  trusted proxy that appends a separate header line can no longer be
  shadowed by a client-supplied first line. `ParseTrustedProxies` also
  unmaps IPv4-mapped IPv6 prefixes (`::ffff:10.0.0.0/104` becomes
  `10.0.0.0/8`) so trusted ranges match in either form.
  into build-tagged files so the module compiles on Windows.
- `config` redaction now covers every secret shape: `*Secret`, `[]Secret`,
  `map[string]Secret`, `zever:"secret"`-tagged slices and maps (every element
  redacted), structs, pointers, and slices of structs containing secrets, and
  `any`-typed values holding a `Secret`; `Redact` also handles Go-typed
  secret values. Nil pointers, maps, and cyclic values do not panic. DSN-like
  values under URL/DSN-suggesting keys (`*_url`, `*_dsn`, `*_endpoint`, ...)
  are scrubbed of scheme-less `user:pass@` userinfo and `password=` pairs, and
  URL userinfo inside string slices is scrubbed. A guard test fails when a
  secret-looking field in `core/*/options.go` is neither tagged nor
  allowlisted.
- `fix(security): P2-11 audit follow-ups`: the vault secrets adapter now
  requires an `https` Addr (the token travels in `X-Vault-Token`, so plain
  http sends it in cleartext); test setups opt into plain http explicitly
  with `allow_insecure: true`, which logs a warning at open. Round-trips
  use a client with a TLS 1.2 floor. The kms crypto adapter propagates
  Key/SignKey base64 decode errors as `ErrInvalidKey` instead of deriving
  keys from empty material, and `Sign`/`Verify` fail with `ErrKeyNotFound`
  on a missing or malformed private key instead of panicking. The ffmpeg
  media toolkit emits `--` before output paths (and at the exec boundary),
  rejects flag-looking inputs/outputs before exec, allowlists probe inputs
  via `ValidatePath` (`media.KindForExt`), bounds runs with
  `DefaultProbeTimeout`/`DefaultTranscodeTimeout` when the caller sets no
  deadline, and checks successful outputs against
  `DefaultMaxOutputBytes`. `core/session` gains a `Rotate` helper for
  post-login ID rotation (fixation defense) with no `Store` interface
  change.
- `fix(security): P2-11b vault token-file permissions`: a `token_file`
  readable beyond its owner (any group/other permission bit, i.e. broader
  than `0600`) now fails closed with `ErrInsecureTokenFilePerms` at both
  validation and token resolution. Restrict the file to `0600`, or opt out
  explicitly with `allow_insecure_token_file_perms: true` (logs a warning
  at open when the file is actually permissive).
- `fix(security): P2-11c vault mount escaping`: the KV mount path is now
  escaped per slash-separated segment before interpolation into request
  URLs, so reserved characters (`?`, `#`, `%`, …) cannot shift path
  segments; empty and dot segments are rejected at validation. Nested
  mounts (`team/secret`) keep working.

### CI

- `ci(apidiff)`: the `apidiff` gate is now module-scoped. A `**Breaking:**`
  entry in `## [Unreleased]` only covers a module it names in backticks
  (module path, directory, package path or short name), a Core-tier module
  with no baseline tag now fails unless listed in
  `.github/apidiff-baseline-allowlist.json` with an expiry, and
  `tools/tag-release.sh --verify vX.Y.Z` checks that every module tag exists
  locally or on origin. See `API Compatibility` in `.github/CONTRIBUTING.md`.

## [v0.7.0] - 2026-10-14

### Breaking

- **Breaking:** `feat(gogen)!: typed saga steps`: the `gogen` backend's `<module>/saga.go`
  no longer emits the untyped `SagaCaller.Call(ctx, service,
  method, in any, out any)`, the `sagaStep` helper, or `map[string]any`
  payloads. It now generates per-step typed input/output structs (one pair
  per step role, e.g. `CheckoutSagaReserveInput/Output`, with plain
  standard-library field types), a `SagaCaller` interface with one typed
  method per step, and `RegisterSagas(reg workflow.TypedSagaRegistrar,
  caller SagaCaller)` registering `workflow.TypedStep` values built by
  `workflow.NewTypedStep` / `NewTypedStepWithoutCompensate` over
  `workflow.StepFuncOf[In, Out]`. The new `core/workflow` surface is purely
  additive (`StepFuncOf`, `TypedStep`, `TypedSagaRegistrar`,
  `TypedStepsToSagaSteps`, `ErrTypedStepConvert`); the existing
  `SagaRegistrar`/`SagaStep`/`StepFunc` interfaces are unchanged and both
  `memory` and `db` workflow adapters now also satisfy
  `TypedSagaRegistrar`. Hosts must regenerate saga output (`-update`
  goldens) and reimplement the caller seam with the new per-step methods;
  values still cross step boundaries via encoding/json, so every generated
  struct stays JSON-marshalable.
- **Breaking:** `fix(httpclient)!: block private addresses for remote adapters by default`:
  outbound HTTP clients in the `ai` (`anthropic`, `openai`, `gemini`,
  `ollama`), `document/remote`, `geo` (`google`, `osm`), `i18n/remote`,
  and `notification/twilio` adapters now dial only globally routable
  addresses and never follow redirects. Loopback, private, link-local
  (including the `169.254.169.254` cloud metadata address), carrier-grade
  NAT, multicast, reserved, and other non-routable ranges are refused with
  `httpclient.ErrPrivateAddress` (documentation ranges stay dialable: they
  are unroutable and serve as public examples); every resolved address is checked from a
  single lookup and the validated IP is dialed directly, closing the
  DNS-rebinding check-to-dial window. Operators pointing an adapter at a
  local Ollama/sidecar must now set `allow_private: true` (new option on
  `ai`, `document`, `geo`, and `i18n.remote` options), which logs a line
  at startup. `allow_insecure` no longer disables TLS verification: the
  new `insecure_skip_verify: true` option (on `ai` and `ollama` options)
  is required for self-signed test servers and logs a warning at open.
  The `webhook` and `oidc` adapters already enforced this posture and are
  unchanged. New `shared/httpclient` seams: `LookupFunc`,
  `SafeDialContextWithLookup`, and the `WithLookup` option for pinning DNS
  in tests and split-horizon environments.
- **Breaking:** `fix(server)!: gRPC reflection off by default`: the generated server
  entrypoint (`zever generate server`) no longer registers the gRPC
  reflection service unconditionally. Reflection exposes every service
  schema to any client that can reach the port, so it now wires only
  when the operator sets `GRPC_REFLECTION=true` or the process runs in a
  dev environment (`ZEVER_ENV=dev|test|development`); an explicit
  `GRPC_REFLECTION=false` always wins. Existing projects that relied on
  grpcurl/reflection keep working by setting `GRPC_REFLECTION=true` in
  their environment or re-scaffolding with `--force`.
- **Breaking:** `fix(installer)!: fail when checksum cannot be verified`: `install.sh` and
  `install.ps1` now abort when no SHA-256 tool is available (or hashing fails)
  instead of skipping checksum verification with a warning. Opt out
  explicitly with `--insecure-skip-verify` / `-InsecureSkipVerify` or
  `ZEVER_INSTALL_INSECURE=1` to install unverified binaries. When `gh` is not
  installed, both installers now print the manual
  `gh attestation verify <binary> --repo zenta-dev/zever` command. New
  offline test harness: `sh tools/installer-test.sh` (also wired into
  `make check` and CI).
- **Breaking:** `fix(crypto)!: reject the public dev key unless explicitly allowed`: the
  local crypto adapter now fails `Open` with a typed `DevKeyError`
  (sentinel `ErrDevKey`, with a fix hint) when the deterministic public
  dev key is supplied without an explicit allow. The dev key is accepted
  only when `crypto.options.allow_dev_key: true` is set in the config file
  or `ZEVER_ENV` is `dev`, `test`, or `development`; every other
  environment (including an unset `ZEVER_ENV`) must supply a real key.
  This removes the old opt-out `ZEVER_CRYPTO_REQUIRE_REAL_KEY` escape
  hatch: the dev key is now rejected by default instead of accepted by
  default, so a production deploy that forgets to configure crypto fails
  closed instead of silently running with a publicly known key. The
  scaffolded `zever new` dev config writes `allow_dev_key: true` plus the
  dev key with a comment, and `zever doctor` flags the dev key in
  non-dev environments. Test fixtures must set `allow_dev_key` or
  `ZEVER_ENV` explicitly when resolving crypto from `config.Default()`.
- **Breaking:** `auth.JWTOptions` and `auth.Options` are no longer
  comparable: the new `Keys []JWTKey` field (kid-based rotation) makes `==`
  on these structs — and on `config.Service[auth.Options]` — a compile
  error. Compare field-by-field instead.

### Added

- `feat(cli): eject and drift tooling`: `zever compile` stamps every
  generated file with a provenance header (backend, CLI version, schema
  sha256; `.json` specs embed an `x-zever-generated` block instead of a
  comment so parsers keep working), and the new `zever compile --check`
  flag exits 1 naming `missing`/`differs`/`extra` files when committed
  output no longer matches the schema — the CI drift gate. `zever new`
  scaffolds `.github/workflows/ci.yml` (regenerate, `--check`, build,
  test), and the new "Customizing and leaving zever" docs page covers
  standalone `orm`/`core/*` use, custom backends, generated-vs-owned, and
  the eject path. `make compile-check` (wired into `make check`) plus a
  CI `drift` job enforce it over `examples/todo`.
- `feat(cli): start-small profiles`: `zever new --profile
  minimal|api|full` scaffolds a preset adapter set — `minimal` is the
  bootable floor only, `api` adds cache/queue/session/password/scheduler/
  outbox/idempotency/lock, `full` wires every known battery. Explicit
  `--batteries` add to the preset and `--adapters` override its picks;
  `--list-batteries` prints the presets too. Measured footprints
  (`go list -deps ./...` packages, stdlib included): minimal 450, api
  492, full 649; direct requires 22/38/82.
- `feat(gogen): typed pagination`: `paginated: true` operations now generate
  an opaque `Cursor` string type, a generic `Page[T]` (`Items` plus
  `NextCursor`), and `DefaultPageLimit` (20) / `MaxPageLimit` (100) bounds in
  `service.go`. Service signatures take `cursor Cursor` instead of `cursor
  string`; the generated router handler passes the cursor query parameter
  through untouched and rejects a negative limit or one above `MaxPageLimit`
  as `InvalidArgument` before the service runs (zero still means "service
  default"). The HTTP wire (cursor/limit query parameters) and the gRPC
  proto shape are unchanged, so this is not wire-breaking; Go implementers
  must update `cursor string` parameters (and `string` cursor variables) to
  `Cursor`, converting from protobuf `next_cursor` strings where they cross
  the boundary.
- `feat(payment): dedupe webhook events by id`: the stripe and paddle
  adapters accept the existing optional `payment.Options.Idempotency`
  store for webhook replay protection (previously refund-only). After
  signature verification, the provider event id (Stripe event `id`,
  Paddle `event_id`, now surfaced as `payment.Event.ID`) is reserved
  with a payload-fingerprint guard and a 24h TTL
  (`DefaultWebhookIdempotencyTTL`); a repeat delivery fails with the new
  typed `DuplicateEventError` (sentinel `ErrDuplicateEvent`) instead of
  re-executing downstream work, and a same-id/different-payload delivery
  trips the store fingerprint mismatch fail-closed. No store (or an
  event without an id) keeps the old process-every-delivery behavior.
  `container.Payment()` reuses the resolved `c.Idempotency()` service
  best-effort when no explicit store is configured; an unresolvable
  idempotency service never blocks payment resolution.
- `feat(middleware): trusted-proxy aware rate limit key`: new
  `middleware.ClientIPKey(trusted []netip.Prefix)` honors
  `X-Forwarded-For` only when the peer is trusted, keying on the
  rightmost untrusted entry (spoofed headers from untrusted peers are
  ignored; all-trusted chains fall back to the leftmost client).
  `ratelimit.Options` gains `trusted_proxies` (CIDRs or bare IPs,
  validated by `ratelimit.ParseTrustedProxies`, mirrored as
  `middleware.ParseTrustedProxies` and in the config JSON schema); the
  generated server builds the key from it and keeps `RemoteAddrKey`
    when none are configured. `RemoteAddrKey` itself is unchanged.
- `feat(orm): auto-chunk large IN lists`: a value-list `IN` larger than
  the resolved dialect's per-statement parameter budget now fans out into
  one statement per chunk for plain unordered unbounded `SELECT`s (rows
  concatenated) and for `Count`/`Exists` (summed/ORed), instead of failing
  in the driver with "too many SQL variables". Budgets come from the new
  `dialect.ParamLimitDialect` interface (`MaxParams`, with a conservative
  999 fallback): Postgres 65535 (wire-protocol uint16 ceiling), SQLite 999
  before 3.32.0 and 32766 after (version-gated via the existing
  `NewWithVersion` detection). Fan-out fails closed with the new
  `ErrInListTooLarge` sentinel when the query carries `ORDER BY`/`LIMIT`/
  `OFFSET`, a row lock or `TABLESAMPLE` (not reconstructible across
  statements), or when the oversized `IN` sits under `OR`/`NOT` or is a
  `NOT IN`. `Preload` sizes its child-`IN` chunks against the same budget
  (minus the child query's own arguments), so old-SQLite backends get
  fitting statements instead of driver errors; the 1000 default stands on
  modern backends.
- `docs: recommend stdhttp as the default router`: the routing guide now
  recommends `stdhttp` (already the `zever new` default) with measured
  numbers, and the benchmarks page gains a 2026-10-14 re-measurement:
  fiber `ServeHTTP` costs ~46 allocs/op (isolated adaptor wrapping ~35,
  concurrent ~55) against ~17 for a `stdhttp` param route (~10 static).
  The older "45–55 allocs/op adaptor cost" note is corrected: the
  `newHTTPHandler` rework cut the isolated adaptor to 35; 45–55 still
  brackets the full serve path.
- `feat(db): migration lint and lock timeouts`: `zever db migrate --lint`
  lints the planned DDL before anything runs and fails on error-severity
  findings: `DROP TABLE`/`DROP COLUMN` (`drop_table`, `drop_column`),
  `ADD COLUMN ... NOT NULL` without a `DEFAULT`
  (`not_null_without_default`), and column type rewrites (`type_rewrite`)
  fail the run, while a postgres `CREATE INDEX` without `CONCURRENTLY`
  (`non_concurrent_index`) only warns. One rule is waived per statement with a
  `-- zever:allow:<code>` comment (bare `-- zever:allow` waives all); the
  pure `orm/migrate.LintStatements`/`LintPlan` helpers back it, with golden
  DDL fixtures under `orm/migrate/testdata`. New
  `--lock-timeout`/`--statement-timeout` flags (falling back to
  `db.options` `lock_timeout`/`statement_timeout` from the config file or
  `DB_*` env) bound every applied postgres statement via `SET LOCAL`
  inside its own transaction; `PlanOptions` carries them (`DefaultLockTimeout`
  10s, `DefaultStatementTimeout` 30s; negative fails `Plan` with
  `ErrInvalidTimeout`). `--concurrent-indexes` builds postgres indexes as
  `CREATE INDEX CONCURRENTLY` (each build outside a transaction, since
  postgres forbids it inside one), via `PlanOptions.ConcurrentIndexes`
  and `atlas.DDLOptions`/`RenderSchemaDDLWithOptions`. Docs gain
  expand/contract, batched-backfill, concurrent-index, and timeout sections
  in the migrations guide.
- `feat: SIGHUP reload for log level, flags, rate limits`: new
  `container.ReloadableLogger`, `container.ReloadableFlag`, and
  `container.ReloadableLimiter` front their battery behind an atomic swap,
  and `container.WatchSIGHUP` runs a reload closure on every SIGHUP until
  its context ends. The generated server (`zever generate server`) wires
  it: SIGHUP re-reads the config file and swaps the log level and the rate
  limiter (old instances released after the swap); a failed re-open keeps
  the old instance and only warns. Flag clients held directly swap the same
  way; the `static` flag adapter also re-reads its file per evaluation when
  `reload: true`. Everything else stays restart-only — adapters, DSNs,
  credentials, TLS material, and listen addresses need a process restart
  (see the new "Reload" section in `config/README.md`).
- `feat(cli): zever upgrade --project`: the binary upgrader gains a project
  mode that bumps every `github.com/zenta-dev/zever/*` require without a
  matching `replace` directive in the current project's `go.mod` to the
  target version (`--version`, default latest stable), runs `go mod tidy`,
  and prints the `**Breaking:**` CHANGELOG entries between the installed
  and target versions (fetched once, cached under the user cache dir).
  `--dry-run` prints the plan without writing, fetching, or tidying;
  `--check` prints the available version only.
- `feat(breaking): index, relation, enum, job, schedule, saga, default
  detection`: the schema comparator now covers the extended structural
  set. Enum removal and value removal are breaking (pure additions and
  reorders are not); removing an index is informational while adding a
  unique one is breaking; relation removal or shape change is breaking;
  adding a field default is informational but removing or changing one is
  breaking; job removal or param change is breaking (queue/retry retunes
  are not); schedule removal is breaking (cron/dispatch retunes are not);
  saga removal or any step change is breaking. `zever breaking` picks the
  new kinds up with no command changes (it renders kinds generically).
- `test(mcp): backend coverage 74% to 93%`: message returns, inline
  enums, `TJSON` fields, ref-typed fields, unknown-enum and empty-ref
  fail-closed paths, the full scalar table, and server-embed round-trip
  are now covered, clearing the P5-7 90% bar.
- `test(fuzz): gogen router input, webhook verifiers, config env parse`:
  new fuzz targets prove no-panic (plus invariant) behavior for import
  classification, route-param extraction, and the resource-ID expression
  picker (`dsl/backend/gogen`); Stripe/Paddle `WebhookEvent` signature
  paths and Paddle amount parsing; and the config env overlay's
  reflection path including its no-secret-in-errors scrubbing. Each ran a
  10–20s burst clean (0.5–1.4M execs); failure seeds stay as regression
  corpus under `testdata/fuzz`. Follow-up noted, not changed: whole-unit
  Paddle totals (`"10"`) pass through verbatim as minor units while
  `"10.00"` scales — pinned by existing tests, needs maintainer call.
- `perf(queue/memory): cut round-trip allocs 16 to 11 per op`: three hot-path
  fixes from a `-benchmem`/`memprofile` pass over the cache/queue adapters.
  Push no longer builds its 50ms backpressure timer before checking capacity
  (fast path returns first); Pop builds its poll timer lazily on the first
  miss instead of per call (timeout now measured from first miss — same or
  more generous, never a spurious Empty); the visibility heap stores entries
  by value instead of pointers (one fewer heap alloc per Pop). Measured
  `BenchmarkMemoryRoundTripParallel` 16→11 allocs/op, 2059→1826 B/op.
  Deliberately deferred (documented follow-ups): per-iteration topic
  sweep-then-recreate churn, `msgspan`/`traceprop` per-Push span costs,
  `Message.Clone` payload/header copies, and cache-memory allocs which sit
  inside `shared/lrucache`, not the adapter.
- `ci: non-gating benchmark trend for orm and container`: new
  `.github/workflows/bench.yml` runs `bench-orm` and `bench-container`
  jobs plus a best-effort `benchstat` compare on PRs touching either
  module, pushes to main, and weekly. Every job sets
  `continue-on-error: true` and the workflow stays out of required
  checks, so benchmark noise never gates a merge; results upload as
  artifacts (90-day retention). A syntax test pins the job names and the
  non-gating flag.
- `feat(container): pool mismatch diagnostics in doctor`: shared-pool
  `MaxConns` mismatches (borrower cap ignored in favor of the first
  opener's) are now recorded, not just warned on stderr: new
  `Container.PoolMismatches()` (borrow order) and `Container.PoolStatus()`
  (backend, borrowers, effective cap), both DSN-free. `zever doctor`
  prints them as `pool:` warnings with the `dedicated_pool: true` fix
  hint (plus `pool_warnings` in `--json`); warnings never fail the
  command. Container README gains `dedicated_pool` guidance for
  queue/workflow/scheduler (share while idle-to-light, split on backlog;
  a pool warning naming one of the three is the signal).
- `perf(cli): cold-start measurement and startup budget`: in-process
  `--help` is now benchmarked (`BenchmarkHelpStartup`: ~99µs/op, 69KB/op,
  625 allocs/op on 2026-10-14 hardware) and capped by a deterministic
  allocation threshold (`TestHelpStartupAllocs`, 1500 allocs) next to the
  existing 200ms wall-clock budget. Release-style binary measured at
  309 MB / ~60–90 ms warm start, dominated by binary load and transitive
  deps (sqlite, TUI, FIPS tables) rather than Cobra dispatch; numbers and
  repro commands live under "Cold start & binary size" in the benchmarks
  reference. A `zever build --adapters=a,b` slim-CLI flag is deferred as
  a follow-up (registration/doctor/scaffold would all need build-tagged
  variants for little saving while sqlite+TUI stay linked).
- `feat(auth/jwt): kid-based key rotation and nbf validation`: `JWTOptions`
  gains `keys: [{kid, secret}]` plus `active_kid` (empty defaults to the
  first entry; validated for unique kids, secret length, and a matching
  active ID). Tokens are signed with the active key and carry its `kid`
  header; verification selects the key by `kid`, falls back to the legacy
  `secret` for kid-less tokens, and rejects unknown kids fail-closed. The
  legacy single-secret path is unchanged and kid-less. `nbf` (when present)
  and `exp` are both enforced with the existing `leeway` tolerance (default
  `0`, strict; ~30s is typical for skewed fleets). HS256-only stands:
  asymmetric algorithms stay unsupported by design and `none`/foreign-alg
  tokens are rejected. Docs now state the default memory revocation store
  is per-process and point multi-instance deployments at the shared
  `revocation/redis` store.
- `ci: apidiff gate for Core-tier modules`: new `tools/apidiff` module plus
  `make apidiff` and a CI `apidiff` job compare the exported API of
  `config`, `container`, `core/{auth,cache,crypto,db,password,queue,router,
  secrets,session}`, `dsl`, and `orm` against each module's latest
  `<dir>/vX.Y.Z` tag via `golang.org/x/exp/apidiff`. Incompatible changes
  fail the gate unless `## [Unreleased]` carries a `**Breaking:**` line.
  The gate already caught two real breaks on adoption (`auth.JWTOptions` /
  `auth.Options` comparability, documented above).
- `feat(config): fail fast on unknown adapters with did-you-mean`: new
  `config.ValidateAdapters(registered)` checks every service's adapter
  against the process's registrations and joins `UnknownAdapterError`s
  carrying a "did you mean %q?" hint (sourced from `config.closest`). Every
  `core/<battery>` gains `RegisteredAdapters()` over its factory registry
  (new `registry.Names`/`SortedNames` seams), the container gains
  `RegisteredAdapters(service)` plus `(c).ValidateAdapters()`, and
  `zever doctor` — CLI and generated-server `--doctor` alike — reports an
  `adapters` row before resolving, turning a forgotten import or typo into
  one actionable error. `ParseAdapter` stays permissive for custom adapters;
  typed `Adapter` constants are unchanged.
- `ci: blocking govulncheck with expiring allowlist`: the `vulnerability` CI
  job no longer runs `continue-on-error` on PRs — reachable vulnerabilities
  fail the gate via the new `tools/vulngate` module (`make vulncheck-all`
  scans every module and filters `govulncheck -json` findings).
  `.github/govulncheck-allowlist.json` records accepted risk with a reason
  and an expiry date; expired entries fail closed. `automerge.yml`
  (`pull_request_target`) was reviewed: it never checks out PR code and only
  reads PR metadata, so no fix was needed. `.github/SECURITY.md` gains a
  supply-chain section (SBOM + attestation verify commands, installer
  verification); the latest-only backport policy already stated there and in
  `GOVERNANCE.md` stands unchanged.
- `docs: test your zever app guide`: new `guides/operate/testing-your-app`
  page — per-test container from `config.Default()` with `t.Cleanup` close,
  httptest routes, per-test sqlite files under `t.TempDir()`, the `dbtest`
  and `outboxtest` conformance kits for custom adapters, deterministic-clock
  and goleak tips. Guide patterns compile and run as
  `docs/examples/testing_test.go`; the todo example gains a small
  `boundary_test.go`. Contributor-only `docs/testing.md` moves to
  `.github/CONTRIBUTING-testing.md` (linked from CONTRIBUTING).
- `docs: CSRF guidance for cookie sessions`: the sessions guide gains a
  "Defend cookie sessions against CSRF" section — why `SameSite=Lax`
  alone is not enough, the `__Host-` prefix, no-state-on-`GET`,
  `Origin`/`Referer` verification with a same-origin code sketch, when
  to add a synchronizer token, and when to prefer `Authorization`-header
  tokens instead. Deliberately docs-only: no speculative CSRF middleware
  ships with this change.
- `feat(middleware): SecurityHeaders for generated server`: new
  `middleware.SecurityHeaders(opts)` sets baseline response security
  headers on every HTTP response (error responses included): always
  `X-Content-Type-Options: nosniff`, a locked-down `Permissions-Policy`,
  and `Referrer-Policy` (all defaulted, all overridable);
  `X-Frame-Options` defaults to `SAMEORIGIN` and `Content-Security-Policy`
  is opt-in (setting a CSP omits the frame header so `frame-ancestors`
  owns framing). `Strict-Transport-Security` is emitted only for a
  positive `HSTSMaxAge` (`DefaultHSTSMaxAge` is one year with
  `includeSubDomains`); invalid options fail closed with the fixed 500
  envelope. The generated server wires it innermost and passes
  `DefaultHSTSMaxAge` only when TLS is configured, so plaintext dev
  servers never emit HSTS.
- `feat(server): TLS/mTLS options for generated server`: the router
  battery gains `tls_cert`, `tls_key`, and `tls_client_ca` options
  (file paths; `ROUTER_TLS_CERT` and friends in env; schema files
  updated). The generated server (`zever generate server`) builds one
  shared `tls.Config` from them (TLS 1.2 floor) and serves both
  listeners from it: gRPC via `credentials.NewTLS`, HTTP via
  `ListenAndServeTLS`, with `tls_client_ca` turning on mutual TLS
  (`RequireAndVerifyClientCert`) on both. Unpaired paths fail
  `config.Validate` fail-closed; the server fails fast with a fix hint
  when key material cannot be loaded. Serving plaintext on a
  non-loopback address logs one warning per listener at startup;
  plaintext loopback stays quiet as the local-dev default. See
  "Terminate TLS" in the deployment guide.
- `feat(container): report abandoned closers on timeout`: `CloseTimeoutError`
  gains an `Abandoned` flag (value type kept) set when a timed-out
  `closeAsync` goroutine is deliberately left running because the service
  ignored its context; the timeout is also reported via the container
  logger (when resolved) with the service name and cause.
- `feat(config): redact by type tag, not only key name`: the redactor now
  consults the new `config.Secret` string type and the `zever:"secret"`
  struct tag via a reflection pass over typed options, so oddly named
  secret fields (e.g. `access_key_id`, which the key-name denylist misses)
  are redacted too. The denylist stays as defense-in-depth. String values
  holding URL userinfo under any other key (e.g. a DSN under
  `database_url`) are scrubbed to `scheme://[REDACTED]@host...` instead of
  leaking credentials. YAML/JSON/env decode of `Secret` is unchanged and
  strict decoding stays strict. Known secret option fields across
  `core/*/options.go` (API keys, tokens, passwords, webhook secrets, HMAC/
  signing keys, AWS key pairs, crypto keys) are tagged.
- `install.sh` / `install.ps1`: richer output (plan summary, numbered steps,
  download progress, checksum details, next steps), uniform `--dry-run`
  rendering, actionable hints on failure, and new `--quiet` / `--no-color`
  (`-Quiet` / `-NoColor`) flags; honours `NO_COLOR`.
- `install.sh` / `install.ps1`: one-line installers that download the release
  binaries (`zever`, `zever-lsp`), verify `SHA256SUMS.txt`, and add them to
  `PATH` (`--version`, `--prefix`, `--yes`, `--force`, `--dry-run`).
- `zever uninstall`: remove the `zever` and `zever-lsp` binaries from
  `PATH` (binaries only; `--dry-run`, `--keep-lsp`, `--yes`/`--force`,
  `--json`, TTY `[y/N]` confirmation with non-TTY re-run hint).
- `zever upgrade`: upgrade the `zever` binary to the latest stable
  GitHub release or an explicit `--version` (SemVer compare via
  `golang.org/x/mod/semver`, `--check`/`--dry-run`, SHA256-verified
  `GOOS`/`GOARCH` asset, atomic POSIX install, staged Windows
  install, same confirmation rules as `uninstall`).

### Changed

- `perf(orm): LRU shape cache`: the render-shape cache evicts only the
  least-recently-used entry on overflow instead of dropping the whole map,
  so hot shapes survive churn. It is striped 16 ways over
  `shared/lrucache` (one bounded LRU per stripe, 256 entries total):
  LRU `Get` rewrites recency and needs exclusive access, so a single
  instance would serialize every parallel hit where the old `RWMutex` map
  allowed concurrent reads. Hit-path cost is unchanged (128 B/op,
  1 alloc/op sequential; 155 B/op, 3 allocs/op parallel on the bench
  machine, within noise of the map). New tests pin single-entry eviction,
  concurrent access under `-race`, and the capacity bound.
- `docs: hygiene pass`: recount confirms 37 configured services
  (`knownServiceNames()` in `config/config.go`) against 42 container
  accessor rows — the five extra rows (`agent`, `grpc`, `grpcclient`,
  `job`, `rag`) resolve over other services and take no `config`
  section; `STABILITY.md` and `container/README.md` now say so instead
  of implying drift. The stale `dsl/compile/testdata/app.zen` header
  (non-existent `internal/dsl/compile` paths, test names, and committed
  packages) now describes the fixture's real consumers, and stale
  `internal/dsl/` path comments across `dsl/`, `cmd/zever`, `orm/migrate`,
  `tools/gengrammar`, and the generated `editors/` grammar are fixed.
  New `config/readme_test.go` pins the `config/README.md` adapter table
  to the `Config` struct (row per known service, default adapter listed).
  The `docs/{production,errors,writing-a-plugin}.md` redirect stubs, the
  `session/cookie` + `media/ffmpeg` matrix rows, `argon2id` vs `argon2`,
  `resilience` memory vs inproc, and the `.zen` reference
  (`reference/zen-syntax.mdx`) were verified already correct.
- `docs: governance files`: new `GOVERNANCE.md` (roles, decisions,
  releases, policy index), `MAINTAINERS`, and `ROADMAP.md` with the 1.0
  criteria checklist (mirrors `STABILITY.md`, all three unchecked).
  `STABILITY.md` gains the Go version policy (latest toolchain only) and
  the deprecation window policy (≥1 minor with `// Deprecated:`); README
  notes generic methods need Go 1.27+. `.github/SECURITY.md` already
  records the backport decision (latest release only) — no change.
- `refactor(container): identity-based close dedup without unsafe`: `Close`
  now dedupes services by the value's own reference identity (pointer, chan,
  map, func, slice via `reflect.Value.Pointer`) instead of searching struct
  fields for the first non-nil pointer. Two distinct services whose structs
  embed the same pointer (e.g. a shared logger) both close now; previously
  the second was skipped. The `unsafe` import is gone, and the 33-entry
  `snapshots()` copy-paste collapsed into a generic `snap` helper over a
  method-value table.
- `chore(lint): wrapcheck evaluation, errscan missing-%w, nolintlint, any`:
  `wrapcheck` was trial-enabled over `container` and `core/cache` (9 and 7
  hits, all registry `Register`/`Lookup` passthroughs, `Close`-probe returns,
  and test helpers): enabling it would relitigate the registry-passthrough
  convention in every battery, so it stays off and the existing
  `.golangci.yml` comment records why. `tools/errscan` gains a `missing-%w`
  rule flagging `fmt.Errorf` without `%w` that still takes a bare `err`
  argument (conservative: `err.Error()` and other names are out of scope);
  the repo scans clean.   `nolintlint` is now enabled so stale suppressions
  fail the build, and the 123 unused directives it reported (mostly
  `nolint:gosec` on `os.ReadFile`/`os.WriteFile` already covered by the
  `std-error-handling` exclusion preset, plus stale `unparam`/`dupl`/
  `nilerr`/`unused`/`revive`/`tparallel`/`exhaustive`/`forcetypeassert`/
  `errcheck`/`contextcheck`/`staticcheck`/`nilnil`/`spancheck`) are removed.
  One survivor needed care: `adapters/media/local` `MkdirAll` tripped gosec
  G703 taint analysis once its directive was lifted (the sink is safe —
  `filepath.Ext` yields no separators), so it now carries a gosec-native
  `#nosec G703` pin like the one in `tools/errscan`, which both linters
  accept stably. Remaining `interface{}` spellings outside
  generated protobuf code become `any` (the `twilio` test fake keeps
  `interface{}` to match the upstream `twilioclient.BaseClient` signature).
- `chore(modules): tag-path gate, shared/* verdict, versioning doc, mysql
  status`: `modgraph --check` (hence `make modgraph-check`) now also
  verifies every `go.mod` module path follows the `tools/tag-release.sh`
  rule (tag `<suffix>/vX.Y.Z`), allowlisting the two deliberate
  consumer-proof exceptions (`docs/examples` dashed path,
  `examples/external-sms` under another org); clean over all 177 modules.
  Import scan of all 22 `shared/*` packages shows every one has importers
  in at least two areas or sits on a public API seam (`mcpclient` in
  `core/agent` signatures, `outboxbridge` as the documented hand-wiring
  seam, `prompt` unused in-repo), so nothing moves under `internal/`.
  New `start/versioning` docs page pins the lockstep release model and
  replace-less consumer rules; `orm/README` no longer claims mysql support
  was dropped and states the real split (query builder sqlite/postgres
  only; atlas DDL + `orm/migrate` live-diff cover mysql).

### Fixed

- `fix(cli): project-aware doctor`: `zever doctor` now checks only the
  services enabled in the project config (top-level batteries in
  `zever.yaml`/`.yml`/`.json`, same discovery as `serve`; falls back to
  checking all batteries with a note when no project config is found).
  The generated server gains a `--doctor` mode that resolves every enabled
  service via its real container and registrations and prints JSON; the CLI
  execs `go run <entry> --doctor` when the project has a server entrypoint
  and falls back to its CLI registry with a note otherwise. Human `doctor`
  now exits 1 on failures (previously 0); flag misuse exits 2; `--json`
  keeps the shared envelope.
- `fix(cli): consistent --json envelope across inspection commands`:
  `routes`, `config show`, and `graph` now honor the global `--json` flag
  (previously `routes --json` printed no JSON at all and the other two
  ignored the flag), and `breaking` and `check-boundaries` gained `--json`
  too. All of them share one envelope helper (`{ok, command, exitCode,
  data, diagnostics, error}` on stdout only; human-readable diagnostics
  and hints stay on stderr): failure envelopes now carry the compiler
  diagnostics that used to print to stderr only, and unknown flags are
  rejected with a did-you-mean suggestion instead of being mistaken for
  file paths. `doctor --json` reports `ok:false` with a nonzero exit code
  when any battery fails (previously `ok:true`/exit 0 despite FAIL rows);
  the human `doctor` output and its battery checks are unchanged.
  `check`, `compile`, and `explain` already emitted the envelope and keep
  byte-compatible success output.
- `fix(cli): zever new yields a bootable app`: `zever new` now scaffolds
  every battery the generated server unconditionally resolves (auth/jwt
  with a dev-only secret, permission/noop, observability/stdout, db/sqlite,
  ratelimit/memory plus the log/router floor) so `/healthz` and `/readyz`
  pass out of the box; runs `go mod tidy` automatically (`--no-tidy` to
  skip) with an actionable error on failure; pins the `protoc-gen-go-grpc`
  tool version and adds local replaces for `orm` so tidy works offline
  against a framework checkout; bumps the scaffold `go` directive to
  `1.27.2`; and `zever serve` pre-flights missing `go.sum` and unregistered
  adapters with a one-line fix instead of raw `go` output.
- `fix(orm): escape single quotes in sqlite JSON path key literals`:
  `sqlitePathLiteral` now doubles embedded `'` in the rendered `$`-rooted
  path body before wrapping it in the SQL string literal, so a json key
  containing a quote (e.g. `a'b`) can no longer terminate the literal
  early; previously such a key produced malformed SQL, and a `''`
  sequence inside the key was silently truncated to `'`. The postgres
  `pgArrayEscape` path was already correct (quotes doubled).
- `fix(document/latex): restrict file access and scrub environment`: LaTeX
  renders run in a per-render temp dir as `cwd` with `-no-shell-escape` and
  a minimal env allowlist (`PATH`, `HOME`, `TEXMF*` only — no inherited
  parent secrets), pin `openout_any=p` (blocks `\openout` outside the render
  tree) and `openin_any=p` (restricts `\input` on TeX Live releases that
  honor it; a documented no-op upstream since TeX Live 2026), kill the
  whole process group on timeout so grandchildren cannot hold pipes open,
  and enforce the output size cap on converter output as well.
- `gogen`: generated `writeGogenJSON` now checks the `json.Marshal` error
  (previously discarded with `_ =`) and answers a bare 500 via the request
  logger instead of writing a response body after the header; generated
  import blocks are grouped goimports-style (std / third-party / zever-local);
  the per-file 10MB request cap is now the document battery's
  `DefaultMaxSourceBytes` constant; golden output is covered by a
  compile test (`go vet` in a temp module) and goldens/examples regenerated.
- `shared/registry`: `New` no longer returns nil errors for unknown,
  duplicate, or nil factories when hooks are nil: a nil `errNil` falls back to
  the exported `ErrNilFactory` sentinel, and nil `duplicate`/`unknown` hooks
  fall back to errors wrapping the exported `ErrDuplicate`/`ErrUnknown`
  sentinels. All three sentinels are now exported for `errors.Is` checks.
- `fix(cli): derive command list from Cobra tree`: the did-you-mean
  suggestion list is no longer hand-maintained — it derives from the Cobra
  command tree (names plus aliases, sorted), so `docs` and `completion` are
  suggested and every future command is covered without a second list to
  update. The deliberate `check-boundaries`/`check:boundaries` alias pair
  keeps working. The unknown-command usage text now lists `docs` and
  `completion`, and `zever --help` shares the same `zever — zever toolkit`
  header as the unknown-command usage.
- fix(container): a panicking service builder no longer wedges concurrent
  callers; returns BuildPanicError.
- refactor(container): service builders now receive a builder-facing root
  context (detached `context.WithoutCancel` with container-lifetime
  cancellation) instead of `context.Background()`, so cancellation and
  tracing propagate into service construction; `Close` cancels it.
  `zever launch` uses a signal context for `exec.CommandContext`.
- Point all one-line installer URLs at `main` instead of the `v0.6.1`
  tag: `install.sh`/`install.ps1` were added after `v0.6.1` was cut, so
  the tag-pinned raw URLs returned 404. The scripts still install
  pinned-version binaries by default.
- `adapters/ratelimit/memory`: hold the admission lock across slot
  reservation and bucket insert so concurrent new-key bursts can no
  longer overshoot the `MaxEntries` bound.

### Security

- Bump the Go toolchain pin from `go 1.27.0` to `go 1.27.2` in `go.work`
  and every module's `go.mod` (fixes stdlib `net/http` / HTTP/2
  advisories `GO-2026-6603`…`GO-2026-6617`).
- Bump `golang.org/x/net` from `v0.58.0`/`v0.59.0` to `v0.60.0` across all
  modules that require it (same advisory family, `x/net` HTTP/2 surface).
- Bump the pinned `golangci-lint` from `v2.13.2` to `v2.14.0` (the old
  linter cannot read `go1.27.2` export data); the CI binary cache is now
  keyed on `go.work` so future toolchain bumps rebuild it.

### Tests

- `test(orm): fuzz JSON path key escaping`: native Go fuzz targets
  `FuzzSQLiteJSONPathKey` and `FuzzPostgresJSONPathKey` prove
  `sqlitePathLiteral` and `pgPathArray` never emit an unescaped
  string-literal terminator for arbitrary json path keys (quotes,
  backslashes, NUL, unicode, `}`/`,`/`{`), with seed corpus under
  `orm/render/testdata/fuzz`.

## [v0.6.1] - 2026-10-08

### Changed

- Release binaries now ship for linux/darwin/windows × amd64/arm64
  (`.exe` on Windows; existing linux asset names unchanged).
- New `dist/SHA256SUMS.txt` published with every release.
- SBOM generation sharded across parallel jobs.
- Apache-2.0 license stamped on own modules in SBOMs.

## [v0.6.0] - 2026-10-08

### Breaking

- OTel semantic-convention attribute/metric rename: span attributes
  (`http.method`→`http.request.method`, `http.path`→`url.path`,
  `http.status_code`→`http.response.status_code`, plus new `rpc.*` and
  `messaging.*` keys) and counter labels migrated under unchanged metric
  names; update dashboards/queries accordingly.
- `orm.Min`/`orm.Max`/`orm.MinNullable`/`orm.MaxNullable` (plus the four
  window variants) now constrain their column value type to the new
  `orm.Orderable` constraint — MIN/MAX over non-orderable columns is now a
  compile error.
- `orm.StringAgg` now constrains its column value type to `~string`.
- `ir.Validation.Kind` and `ir.DefaultValue.Kind` are now typed
  `ir.ValidationKind` / `ir.DefaultKind` instead of bare `string`.
- `orm.Option[T]` now constrains `T` to the new `orm.ScanValue` constraint;
  migrate struct/entity maybe-wrappers to the new `orm.Nullable[T]`.
- Removed `adapters/webhook/sqlite` (`http` and `queue` remain; default
  stays `http`).
- Removed `adapters/search/sqlite` (folded into `adapters/search/db`).
- Removed `adapters/vectorstore/sqlite` (folded into
  `adapters/vectorstore/db`).
- DB-backed adapter string unified to canonical `db` for `search`,
  `vectorstore`, and `workflow`; legacy names (`postgres`, `sqlite`,
  `pgvector`) stay registered as aliases.
- DB-backed adapter directories renamed to `db` (`adapters/search/postgres`
  → `adapters/search/db`, `adapters/vectorstore/pgvector` →
  `adapters/vectorstore/db`, `adapters/workflow/postgres` →
  `adapters/workflow/db`); import paths change, `Register`/`New`/`Open`
  signatures unchanged.

### Release notes

- `tools/zever-mcp` backend handlers are stubs.
- The default `payment`/`billing` `stub` adapter must be overridden in
  production.

### Added

- Messaging spans with correct span kinds: new `shared/msgspan` module
  (`Producer`/`Consumer` helpers setting `messaging.*` attributes with
  `SpanKindProducer`/`SpanKindConsumer`), applied to eventbus publish/consume,
  queue push, and the job dispatcher drain path.
- Outbox relay wiring: new `shared/outboxbridge` module
  (`QueuePublisher`/`EventBusPublisher` adapting a resolved transport to
  `outbox.Publisher`), `core/outbox.PublisherSetter` for post-`Open`
  attachment, and `container.OutboxRelay(ctx)` which attaches the
  transport-selected publisher and starts the relay.
- Trace propagation in built-in outbound HTTP clients:
  `httpclient.WithTracing()` (opt-in W3C trace-context injection) enabled in
  document/remote, i18n/remote, auth/oidc, the ai adapters
  (anthropic/gemini/ollama/openai), and `shared/providersclient`.
- Generated entrypoints wire the new batteries: the worker starts the outbox
  relay via `container.OutboxRelay(ctx)`; the server resolves the resilience
  manager and ships a `guardedGRPCClient` helper (per-dependency breaker +
  `container.GRPCClient`).
- Prompt pack 3: `DoctorTriage`, `DiagRepair`, `OutboxTriage` and
  `ExtractSplit` builders for doctor, diagnostic, outbox, and extraction
  workflows.
- RAG reranking: `Reranker` interface with deterministic `TermOverlapReranker`
  default and `JudgeReranker` LLM opt-in, applied post-fusion via
  `HybridOptions.Reranker`.
- Container agent options: `Agent(WithMaxParallel(...), WithAgentObserver(...))`
  and `RAG(WithTopK(...), WithHybridSearch(), ...)`; existing no-arg calls
  unchanged.
- Richer `tools/zever-mcp`: `zever_doctor` and `zever_generate` tools
  (via the installed CLI), static `zever://` resources, prompt templates,
  and server-initiated elicitation gating destructive applies.
- `shared/mcpclient` growth: sampler hooks, elicitation policies, and
  reconnecting dial with backoff.
- Nightly eval workflow (schedule + manual dispatch, never gating):
  deterministic golden dataset with report artifact.
- Prompt pack 2: `SchemaDesign`, `MigrationReview`, `CitationCheck` and
  `ToolPlan` builders.
- Agent-app tutorial (`docs/tutorials/build-an-agent-app`): end-to-end
  walkthrough from container resolution through RAG grounding, agent loops,
  MCP serving, and eval scoring.
- Agent runtime upgrades: `Loop.RunStream` event channel over `ai.Stream`,
  ordered parallel tool fan-out via `Options.MaxParallel` (with the
  `ParallelToolCalls` model hint), `AsTool` sub-agent composition, and
  `Options.Observe` execution hooks.
- Hybrid RAG: `rag.NewHybrid` indexes into a keyword backend alongside the
  vector store; `RetrieveHybrid` fuses both with RRF plus metadata
  filtering, with `AnswerHybrid` grounding. `Options.Observe` reports
  ingest/retrieve/generate events.
- New `shared/mcpclient` module: MCP stdio client (initialize handshake,
  tools/list, tools/call) over any `io.Reader`/`io.Writer`, plus
  `agent.ToolsFromClient` exposing server tools as agent tools.
- New `core/eval` module: dataset/scorer harness (`ExactScorer`,
  `ContainsScorer`, LLM-judge scorer) with suite reports.
- New `shared/prompt` module: pure-string builders for system prompts,
  grounded context, JSON repair, and tool-error feedback.
- Rewrote the docs landing page as a standalone Astro page
  (`docs/src/pages/index.astro`): dark-first/light themed, live schema-to-outputs
  compile stage, hand-wired vs zever comparison, scroll-driven schema
  walkthrough, adapter-swap playground, filterable batteries grid, container
  lifecycle graph, terminal quickstart, agent section, reproducible
  benchmarks, examples carousel, and FAQ. The former splash content moved to
  `getting-started/introduction`.
- Docs redesign to match the landing page: custom Starlight theme replacing
  `starlight-theme-black` (Space Grotesk and JetBrains Mono, shared color
  tokens, dark and light), blurred header with Guides/Reference tabs and a
  search pill, icon sidebar with active pill, scroll-spy table of contents,
  breadcrumbs with reading time, previous/next cards, and styled callouts,
  steps, tabs, cards and code windows. Added "Copy page" and "Open in
  Claude/ChatGPT" page actions, a per-page `.md` route, `llms.txt` and
  `llms-full.txt`, and a "Was this page helpful?" widget. Key pages gained
  steps, tabs and callouts, and `zen` code blocks are now highlighted.
- Search, AI and sharing optimization for the docs site: generated 1200x630
  Open Graph cards (landing plus one per docs page) and app icons, canonical
  and social meta, JSON-LD (Organization, WebSite, SoftwareApplication,
  TechArticle, BreadcrumbList), `robots.txt` that welcomes search and AI
  crawlers, a web manifest, font preloading, a richer `llms.txt` with key facts,
  and `noindex` on the 404 page. The sidebar scroll position, open groups and
  per-page content scroll now persist across navigation and reloads.
- Docs restructured around the reader's goal (Diataxis): Start, Tutorials,
  Guides (HTTP APIs, Data and storage, Background work, Security, Operate,
  Integrations, Extend), Concepts, and Reference, with header tabs that filter
  the sidebar. Old URLs redirect. Prose is rewritten to lead with the answer,
  with task-based sections and a complete per-service config reference, plus a
  docs style guide (`contribute/writing-docs`), `lint-docs` and `check-links`
  scripts, a quickstart, and a "How Zever works" page. `docs/errors.md`,
  `benchmarks.md`, `production.md`, and `writing-a-plugin.md` now live on the
  site.
- `--dry-run` across all nine `generate` subcommands: validation and
  rendering still run, but no filesystem writes occur; append-mode
  subcommands preview the rendered declaration.
- The `mcp` codegen backend now also emits a runnable `<module>/mcpserver`
  stdio server per service-bearing module: self-contained stdlib-only Go
  with per-tool handler stubs, proven by a test that builds the generated
  server and exercises it over stdio.
- Claude Code agent skill: `zever docs --skill --dir` renders `SKILL.md`
  from a prose template plus the live command catalog; snapshot at
  `skills/zever/SKILL.md` pinned by tests, documented in
  `reference/agent-skill`.
- Agent-friendly CLI: global `--json` (`ZEVER_JSON`) emitting JSON envelopes
  with typed exit codes; structured data for `compile`, `check`, `explain`
  and `doctor`; `--help --agent` machine-readable command catalog; `--dry-run`
  for `new`, `add` and `extract`.
- New `dsl/backend/mcp` codegen backend: renders each service RPC as an MCP
  tool with JSON Schema input/output (opt-in via `compile --backend mcp`),
  plus `openapi.JSONSchemaForEntity` for self-contained entity schemas.
- `container.Agent()` and `container.RAG()` accessors building the agent loop
  and RAG engine over the shared `AI` and `VectorStore` instances (no `config`
  entry, closed first); root `llms.txt` for coding agents.
- Generated HTTP+gRPC server entrypoint now registers the standard gRPC
  Health Checking Protocol (`grpc_health_v1`) and server reflection, marks
  the overall serving status SERVING on boot, ties `/readyz`'s DB ping to
  the health status, and flips NOT_SERVING before graceful stop.
- New `core/agent` package (bounded tool-calling loop over `core/ai` with an
  optional human-in-the-loop confirmation hook and schema-constrained
  `GenerateStructured` with decode retries). No `config` entry: build it over a
  resolved `ai.AI` with `agent.New`.
- New `core/rag` package (chunk, batch-embed and upsert documents into a
  `vectorstore`, then retrieve top-K context and generate grounded answers).
  No `config` entry: build it over a resolved `ai.AI` and
  `vectorstore.VectorStore` with `rag.New`.
- New `tools/zever-mcp` module: a stdio Model Context Protocol server exposing
  `zever_compile`, `zever_schema` and `zever_explain` so AI agents can
  introspect and validate `.zen` schemas. Hand-rolled over newline-delimited
  JSON-RPC 2.0 (no SDK dependency; stdio-only).
- Saga orchestration for the `workflow` battery: `SagaStep` plus
  `SagaRegistrar` / `SagaRunner` / `SagaInspector` interfaces in
  `core/workflow`, with in-process (`adapters/workflow/memory`) and durable
  (`adapters/workflow/db`) implementations featuring forward execution,
  pivot roll-forward, reverse compensation with retry, and
  `RecoverStuckSagas` crash recovery.
- `.zen` top-level `saga { ... }` declaration: composes existing services'
  RPCs into a workflow saga (`step` with required `execute`, optional
  `compensate`, and at most one `pivot: true`), resolved into IR and rendered
  by the `gogen` backend as `workflow.SagaStep` registration behind a
  `SagaCaller` seam.
- New `core/resilience` battery (process-local circuit breaker, bulkhead, and
  timeout/retry policy composition; default adapter `memory`) and
  `core/outbox` battery (transactional outbox/inbox; default adapter `db`,
  plus `memory`), wired into `config` (`resilience: memory`,
  `outbox: db`), the `container` accessors `Resilience()` / `Outbox()`,
  `zever new`/`zever add` battery selection, and `zever generate adapter`.
  Adapter modules `adapters/resilience/inproc`, `adapters/outbox/memory`, and
  `adapters/outbox/db` ship as separate Go modules.
- New `shared/grpcclient` helper module (`New(ctx, target, opts...)`:
  round-robin load balancing, per-method retry policy, TLS/insecure
  credentials, static resolver, resilience guard, metadata propagation) and
  the `container.GRPCClient(target, opts...)` accessor: one cached
  `*grpc.ClientConn` per target string, closed with the container.
- New `adapters/resilience/redis` adapter module: a distributed circuit breaker whose state is shared across replicas via Redis (redsync-backed `sony/gobreaker/v2` `DistributedCircuitBreaker`); timeout/retry/bulkhead stay process-local. `core/resilience` gains `RedisOptions` (`redisopt.Options` + `Prefix`); live tests gated by `REDIS_ADDR`.
- New `adapters/outbox/cdc` adapter module (postgres logical replication via
  `pg_logical_emit_message` + `START_REPLICATION`), registering the
  `outbox.CDC` adapter; live tests gated by `POSTGRES_DSN`.
- CDN battery: new `core/cdn` module (`CDN` interface with `Purge`/`Close`/`Name`
  and `PurgeRequest` by URL, cache tag, or full zone), plus `adapters/cdn/noop`
  (zero-infra default) and `adapters/cdn/cloudflare` (purge via the official
  `cloudflare-go/v7` client). Wired into the container (`c.CDN()`), layered
  config (`CDN_*` env vars, strict schema), and the `zever` CLI (`zever add cdn`,
  the `zever new` battery picker, and `zever generate adapter cdn`).
- Outbox relay metrics: gauges and a histogram on the relay loop
  (published/consumed counts, DLQ depth, poll/claim latency), exported
  under `outbox.*` metric names.
- `zever outbox` ops CLI group: `status` (relay health), `dlq list`,
  `dlq requeue`, `dlq purge`, and `purge` (acked messages).
- `outbox.stall_readiness` opt-in config plus `container.Ready` — an
  aggregate readiness check (DB ping + relay stall) wired into `/readyz`
  and the gRPC health service.
- `shared/grpcclient`: client spans now start with `SpanKindClient` via
  `observability.StartSpan`.

### Documentation

- New digging-deeper guides for the microservice-readiness capabilities:
  `resilience` (guard composition, error classification, `memory`/`redis`
  adapters), `outbox` (transactional outbox/inbox, polling relay, DLQ,
  `db`/`memory`/`cdc` adapters), `grpc-client` (round-robin, retry,
  interceptors, TLS, container caching), and `saga` (pivot roll-forward,
  reverse compensation, `RecoverStuckSagas`, `memory`/`db` adapters);
  all four wired into the docs sidebar, with `production.md` and the
  deploy checklists updated.
- `saga` guide documents the top-level `.zen` `saga { ... }` declaration:
  `step` syntax, validation rules (required `execute`, unique step names,
  at most one `pivot`), and the generated `SagaCaller`/`RegisterSagas`
  output with an end-to-end wiring note.
- New digging-deeper guides for the outbox ops and cross-service tracing
  surfaces: `outbox-ops` (exact `outbox.*` relay metric names and
  attributes, recommended Prometheus alert rules, the DLQ runbook around
  `zever outbox status`/`dlq list`/`dlq requeue`/`dlq purge`/`purge`, and
  the opt-in `outbox.stall_readiness` gate) and
  `cross-service-tracing` (inbound extraction vs outbound injection,
  the `tenant.id`/`user.id`/`correlation.id` baggage keys, the five span
  kinds on `observability.StartSpan`, provider setup and collector
  verification, plus the OTel semconv attribute migration table). Both
  wired into the docs sidebar; `production.md`'s observability section
  no longer calls propagation evolving, and its pre/post-deploy
  checklists now cover relay alerts and trace continuity.

### Dependencies

- Routine dependency refresh: `anthropics/anthropic-sdk-go` 1.75.0 → 1.76.0,
  `modernc.org/sqlite` 1.59.0 → 1.60.1, `twilio/twilio-go` 1.31.1 → 1.31.2,
  `openai/openai-go/v3` 3.66.0 → 3.68.0, `posthog/posthog-go` 1.27.0 → 1.27.1.
  `google.golang.org/grpc` is held at 1.83.2 (1.84.0 is affected by
  CVE-2026-84445 / GO-2026-6443; no patched stable release yet).

### Changed

- **Breaking:** span and metric attribute names migrated to OpenTelemetry
  semantic conventions: `http.method`→`http.request.method`,
  `http.path`→`url.path`, `http.status_code`→`http.response.status_code`;
  gRPC spans now carry `rpc.system`/`rpc.service`/`rpc.method`/`rpc.grpc.status_code`;
  messaging spans use `messaging.*`. Update dashboards/queries accordingly.
  Counter labels migrated too, under unchanged metric names: the
  `http.request` counter is tagged by `http.request.method`/
  `http.response.status_code` (was `method`/`status`, where `status` held
  status *text*) and the `rpc.request` counter by
  `rpc.service`/`rpc.method`/`rpc.grpc.status_code` (was `method`/`outcome`).
  Keys are exported as constants from `core/observability`
  (`HTTPRequestMethod`, `URLPath`, `HTTPResponseStatusCode`, `ServerAddress`,
  `RPCSystem`, `RPCService`, `RPCMethod`, `RPCGRPCStatusCode`,
  `MessagingSystem`, `MessagingDestinationName`, `MessagingOperation`,
  `MessagingMessageID`, `MessagingMessageConversationID`).
- `vectorstore/db` sqlite query now decodes row metadata lazily: only rows
  that survive the topK heap get their metadata JSON parsed (was: every
  scanned row). Corrupt metadata in a row that ranks below topK no longer
  fails the query; a surviving row with corrupt metadata still fails.
  `BenchmarkQuery` (256 rows, topK=10) drops ~49% ns/op, ~53% B/op, ~30%
  allocs/op.
- Standardized error message format across all modules: messages now use
  `<package>: <message>` prefixes (colon form, no brackets, no trailing
  period). Sentinels gained module prefixes, all sentinels carry doc
  comments, custom error types use value receivers, and duplicate-
  registration sentinels were unified. Double-`%w` wrapping in a single
  `fmt.Errorf` is removed (use `errors.Join` to combine a sentinel with a
  cause). Added `docs/errors.md` as the canonical error conventions guide
  and `make err-lint` (`tools/errscan`) as a forward-looking guard for the
  rules above.
- `zever.schema.json` now covers the `outbox` and `resilience` services and
  previously-missing option fields, with a schema drift test guarding
  future divergence from `config.Config`.
- Corrected stale agent docs (CLAUDE.md, AGENTS.md) describing a removed TUI
  and pointer-typed close errors.

### Fixed

- scheduler/postgres: a tick skipped because the slot load or claim failed
  (DB error, or the fire timeout expiring before dispatch) now logs a
  `postgres: tick skipped` warning instead of returning silently; Stop
  cancellation is still not logged.
- `zever tinker` no longer deadlocks when the shim hangs: each `call` is bounded
  by a timeout and kills the shim on expiry, so a stuck read cannot hold the
  client mutex forever and block later calls or `Close`.
- Example apps (bookings, showcase, todo, demoapp) failed at startup with
  `unknown adapter: slog`; their app.go and self-wired entrypoints now register
  every adapter configured in zever.yaml, and the CLI composition root
  registers the 10 previously-missing adapters (crypto/kms, idempotency/db,
  outbox/{cdc,db,memory}, resilience/{inproc,redis}, scheduler/postgres,
  secrets/vault, session/db).

### Security

- `document/local` render now blocks external network egress: Chrome runs
  with `--host-resolver-rules="MAP * ~NOTFOUND, EXCLUDE localhost"` and
  `--disable-background-networking`, so untrusted HTML cannot trigger SSRF to
  internal services or cloud metadata endpoints via embedded resources.
- `crypto/local` now fails closed when the deterministic dev key is used and
  `ZEVER_CRYPTO_REQUIRE_REAL_KEY` is set, so production deployments can reject
  the publicly known dev key instead of silently running with it. Added
  `crypto.IsDevCryptoKey` for the check.
- `vault` secrets adapter now closes HTTP response bodies on every call
  (previously leaked a connection per operation). `argon2id` `Verify` and
  `NeedsRehash` reject stored-hash parameters outside the supported ranges
  instead of panicking (`t`/`p` below 1) or allocating unbounded memory.
- `shared/httpclient.SafeDialContext` dials the resolved, validated IP
  instead of re-resolving the hostname, closing a DNS-rebinding TOCTOU in
  the SSRF guard. `media/s3` validates asset ids as 32-hex before listing,
  preventing an empty-prefix IDOR (first-object match) and enumeration.
- Outbound endpoints for `search` (Meilisearch `Host`), `vectorstore`
  (Qdrant `URL`), and `auth` OIDC (`Issuer`) are now https-only by default
  via `shared/endpoint.ValidateURL`, matching `geo`/`i18n`; each gains an
  explicit `AllowInsecure` opt-in for tests. Qdrant `UseTLS` follows the
  validated scheme instead of silently downgrading `http://`/`grpc://`/bare
  hosts to plaintext, and OIDC discovery runs through
  `httpclient.NewSafeClient` (TLS 1.2 floor, private-address dial guard, no
  redirects) instead of `http.DefaultClient`.
- `ai/ollama` no longer hardcodes insecure endpoints: the adapter requires
  https for non-loopback `Addr` and verifies TLS unless the new
  `ai.Options.AllowInsecure` (also `ollama.Options.AllowInsecure`) is set,
  consistent with the `ai/gemini` fix. Loopback http stays permitted for
  local development.
- `session/cookie` adds opt-in `__Host-`/`__Secure-` cookie-name prefixes
  via `Options.Prefix` (constants `PrefixHost`/`PrefixSecure`). `New`
  enforces each prefix's browser rules: `__Host-` forces `Secure`,
  `Path=/`, and omits any `Domain` (which would widen the cookie to
  subdomains, defeating the host-only guarantee); `__Secure-` forces
  `Secure`. The default (no prefix) and `SameSite` behavior are unchanged.
- `ai/gemini` no longer auto-disables TLS verification for loopback
  `BaseURL`: the new `ai.Options.AllowInsecure` (https-only default,
  matching `geo`/`i18n`) is required to skip verification. Loopback
  endpoints now verify certificates unless explicitly opted out.
- `document/remote` no longer unconditionally allows insecure endpoints:
  the new `document.Options.AllowInsecure` gates http endpoints
  (https-only default, matching `geo`/`i18n`).
- `crypto/kms` `New` fails closed unless the new `Options.DevStub` opt-in
  is set: the deterministic stub client provides no real KMS boundary, so
  production configs must inject a real client via `NewWithClient`.
- `secrets/vault` `Get` decodes base64 strictly and returns the new
  `ErrInvalidBase64` on malformed values instead of silently falling back
  to the raw value (which corrupted secrets written by foreign clients).
- `webhook/http` `Register` rejects an empty target secret with the new
  `ErrMissingSecret`: every delivery now carries `X-Hub-Signature-256` and
  is verifiable by the receiver.
- `tenant/header` rejects case-variant duplicate tenant headers with the
  new `ErrDuplicateHeader` (previously resolved in nondeterministic
  map-iteration order) and validates resolved tenant IDs (1-128 chars of
  `[A-Za-z0-9._-]`) with the new `ErrInvalidTenantID`.

### Changed

- `ratelimit/memory` bounds its bucket table via the new
  `ratelimit.Options.MaxEntries` (default 1000, mirroring `lock/memory`);
  past the bound a new key evicts the least-recently-used live bucket.
- `idempotency/memory` bounds its record table via the new
  `idempotency.Options.MaxEntries` (default 1000) with soonest-expiry
  eviction, and replaces the per-`Begin` O(n) full-map purge with a
  background janitor goroutine joined on `Close`.
- Redis-backed batteries (`cache`, `queue`, `lock`, `ratelimit`,
  `idempotency`, `session`, `eventbus`) now share one connection pool per
  resolved addr+DB via the new refcounted `redisclient.Shared` registry
  (mirroring `container`'s DB pool sharing), instead of opening an
  independent client per battery. The first caller to open a key supplies
  the pool-sizing options; later borrowers reuse that pool and their
  pool-size fields are ignored (first-creator-wins). `redisclient.New`
  remains for callers that need a private client.

### Added

- Fuzz targets (seed-corpus regression tests, run with `-fuzz`):
  `config.FuzzDecodeFile` (YAML/JSON decode path), `orm/render.FuzzRenderRaw`
  (raw/identifier/JSON render path), `core/session.FuzzValidateID`.
- `flag.GetJSON[T]` — type-safe generic wrapper over `Flag.JSON` that
  decodes a flag value into a concrete type with a typed fallback.
- Security hardening (additive, fail-closed defaults): `authz`
  `UnaryServerInterceptorStrict` denying unlisted gRPC methods; opt-in
  `Policy.OwnerField` wiring into resource attributes; `permission.Options`
  rejects `Rules` combined with `ModelPath`; JWT `Leeway` option for clock
  skew (default 0); secret zeroization on `Close` for the JWT and local
  crypto adapters; session/OIDC revocation and timestamp-precision doc
  notes.
- Conformance kits for 11 batteries (`lock`, `ratelimit`, `eventbus`,
  `idempotency`, `flag`, `log`, `observability`, `crypto`, `password`,
  `billing`, `payment`) plus adapter wiring; kit list in
  `docs/writing-a-plugin.md` grows 3 → 15.
- Concurrent-load benchmarks for router, ORM, and queue with published
  numbers in `docs/benchmarks.md`.
- SLSA build provenance attestation over SBOMs in CI
  (`gh attestation verify sbom/<slug>.json --repo zenta-dev/zever`).
- `STABILITY.md` 1.0 promotion criteria with current-standing report;
  DSL plugin-battery design note
  (`dsl/plugin-batteries.mdx`, boundary recommendation).
- `examples/showcase` positioned as newcomer-clone-first reference app.
- Conformance kits for 19 more batteries (`db`, `search`, `vectorstore`,
  `document`, `media`, `session`, `notification`, `webhook`, `workflow`,
  `scheduler`, `ai`, `analytics`, `geo`, `i18n`, `mailer`, `tenant`,
  `router`, `middleware`, `job`, `auth`, `authz`, `permission`) plus
  adapter wiring; kit list in `docs/writing-a-plugin.md` grows 15 → 24+;
  smoke suites for the `password`/`billing` kits.
- `adapters/queue/db`: DB-backed queue for multi-worker deploys without
  Redis (postgres URL opens postgres, anything else opens sqlite, empty
  selects a private in-memory-style database). Single messages table with
  lease-based claiming (compare-and-set on `claimed_by`/`claimed_until`,
  attempt bump on reclaim, `available_at` for delayed delivery), mirroring
  `adapters/queue/redis`'s claim/reclaim/ack/nack scripts and
  `adapters/workflow/postgres`'s lease CAS; passes the shared
  `queuetest.Conformance` kit. Polling transport with moderate throughput,
  not a Kafka/SQS replacement (separate queue database with 3-5
  connections per worker recommended).
- `adapters/scheduler/postgres`: durable leased scheduler for
  multi-instance deploys (mirrors `adapters/workflow/postgres`).
- `adapters/cache/db`: DB-backed `cache.Cache` over `shared/kvstore`
  (mirrors `adapters/session/db`; sqlite `:memory:` by default, postgres
  via DSN, `NewFromDB` shared-connection path), proven by the
  `cachetest.Conformance` kit on sqlite and postgres. Opt-in only;
  default stays `memory`.
- Redis conformance suites (`cache`, `queue`, `lock`) run green on
  miniredis instead of skipped (FastForward clock seam, header-decoding
  and BLPop-floor fixes, `Increment` error mapping).
- Release workflow (`.github/workflows/release.yml`) with SLSA-attested
  binaries and SBOMs; verify with
  `gh attestation verify <artifact> --repo zenta-dev/zever`.
- `lock.Options.MaxEntries`: configurable bound for the memory adapter
  lease table (non-positive means 1000, matching `cache.Options.MaxEntries`).
  Past the bound the least-recently-used live lease is evicted and its
  holder observes `lock.ErrNotHeld`; set a large value for effectively
  unbounded storage. This documents the fixed 1000-entry LRU bound the
  TTLCache migration imposed on previously-unbounded lease storage.

### Changed

- **Breaking:** `orm.Min`/`orm.Max`/`orm.MinNullable`/`orm.MaxNullable` and
  the four window variants (`orm.MinOver`/`orm.MaxOver`/
  `orm.MinNullableOver`/`orm.MaxNullableOver`) now constrain their column
  value type to the new exported `orm.Orderable` constraint (the
  `~int* | ~uint* | ~float* | ~string` union plus `time.Time` — stdlib
  `cmp.Ordered` omits `time.Time`, which SQL MIN/MAX do order). Previously
  `V any` let MIN/MAX compile over non-orderable columns (e.g. `bool`,
  `[]byte`) and fail only at runtime; now such misuse is a compile error.
  Migration: none for orderable columns (all existing call sites are
  `int*`/`float*`/`string`/`time.Time`); a MIN/MAX over a non-orderable
  column was never valid SQL anyway.
- **Breaking:** `orm.StringAgg` now constrains its column value type to
  `~string` (`V ~string` in place of `V any`): the SQL `string_agg(c, ...)`
  concatenates its argument as text, so a non-string column type was a
  programmer error that compiled silently. Migration: none for string
  columns; non-string use was never meaningful.
- **Breaking:** `ir.Validation.Kind` and `ir.DefaultValue.Kind` are now typed
  `ir.ValidationKind` / `ir.DefaultKind` (exported string types with
  constants `ir.ValidationFormat`/`ValidationMinLen`/`ValidationMaxLen`/
  `ValidationGT`/`ValidationGTE`/`ValidationLT`/`ValidationLTE` and
  `ir.DefaultLiteral`/`DefaultNow`) instead of bare `string`. Every consumer
  switch (gogen `writeValidationCheck`, openapi `applyValidation`) now
  lists all kinds exhaustively and fails closed with a panic on an unknown
  kind, mirroring the existing invariant-guard style — a new kind added to
  the resolver now forces every switch site to handle it at compile time.
  Migration: code comparing `Kind` against string literals still compiles
  (untyped constants convert); switch sites should move to the typed
  constants. The openapi behavior change is observable: a validation rule
  with an out-of-set kind now panics at render time instead of being
  silently skipped (resolver-produced IR never triggers it).
- **Breaking:** `orm.Option[T]` now constrains `T` to the new exported
  `orm.ScanValue` constraint (`~string | ~[]byte | ~int64 | ~int32 |
  ~float64 | ~float32 | ~bool | time.Time`; `orm.JSONText` is covered by
  `~string`). Previously `T any` let a struct/entity or any other
  non-scannable type compile and fail only when `Scan` ran; now such a type
  is a compile error. Migration: use the new `orm.Nullable[T]` (same
  present/absent API — `NullableSome`/`NullableNone`/`IsSome`/`Get`/`GetOr`,
  but no `Scan`/`Value`) for struct/entity maybe-wrappers, e.g. the
  outer-join result types (`orm.Row2[A, orm.Nullable[B]]`, and the
  `LeftJoin2`/`LeftJoin3`/`RightJoin*`/`FullJoin*`/TVF variants).
  `orm.Option[T]` stays for NULL-capable scan columns (codegen'd nullable
  fields). Generated relation doc comments and goldens were updated to
  reference `orm.Nullable`.
- Bounded connection-pool defaults for the `sqlite` and `postgres` `db`
  adapters when `MaxConns` is left at zero: file-backed `sqlite` now caps
  `database/sql`'s previously unlimited pool at `max(1, GOMAXPROCS)`, and
  `postgres` now defaults to `max(4, GOMAXPROCS)` instead of pgxpool's
  `max(4, NumCPU)`, so a container's cgroup CPU limit is honored (GOMAXPROCS
  is container-aware since Go 1.25). Explicit `MaxConns` is unchanged; set
  it to restore the old unbounded/native behavior.
- feat!: unify DB-backed adapter strings to canonical `db` for `search`,
  `vectorstore`, and `workflow` (`core/search.DB`, `core/vectorstore.DB`,
  `core/workflow.DB`; adapter packages' `Adapter` consts now equal the
  `DB` consts). The old names stay registered as aliases — `postgres` and
  `sqlite` for search, `pgvector` and `sqlite` for vectorstore, `postgres`
  for workflow — so existing `zever.yaml` files, `*_ADAPTER` env values,
  and `search.Open`/`vectorstore.Open`/`workflow.Open` calls keep
  resolving to the same driver; `ParseAdapter` was already open (any
  non-empty name) and the config schema enums keep the old spellings
  alongside `db`. What changes in generated files: `config.Default()`
  (and therefore `zever new` scaffolds and the `search`/`vectorstore`
  `allServiceAdapters` defaults) now renders `adapter: db` instead of
  `adapter: sqlite` — behavior-identical (empty DSN still selects the
  embedded sqlite backend) but the string differs; `zever new`'s picker
  offers `db` first with legacy names after, and `compose.yaml` is now
  emitted for `db`-named picks (a `db` DSN may point at postgres) just
  like the legacy `postgres`/`pgvector` picks. Migration: prefer `db` in
  new files; old files need no change.
- feat!: rename DB-backed adapter directories to `db` (`adapters/search/postgres`
  → `adapters/search/db`, `adapters/vectorstore/pgvector` →
  `adapters/vectorstore/db`, `adapters/workflow/postgres` →
  `adapters/workflow/db`; packages `postgres`/`pgvector` → `db`).
  Migration for direct importers: Go import path change only — replace the
  old import paths with the new `.../db` paths and rename aliases
  (`searchpostgres` → `searchdb`, `vectorstorepgvector` → `vectorstoredb`,
  `workflowpostgres` → `workflowdb`); `Register()`/`New`/`Open` signatures
  unchanged. `zever.yaml`/`config` adapter strings (`postgres`, `pgvector`,
  `sqlite`, `memory`) keep working unchanged.
- Shared postgres/sqlite pool (container-owned DSN→pool registry): every
  db-backed battery pointing at the same DSN (`db`, `cache`, `queue`,
  `search`, `session`, `idempotency`, `workflow`, `scheduler`,
  `vectorstore`) now borrows one pool per exact-DSN match instead of
  opening one pool per battery; borrowers keep `owns=false` and the
  registry closes each pool once, after every borrower (`db` moved out of
  the close snapshots into the registry band). The shared pool takes the
  first opener's pool knobs and a differing nonzero `MaxConns` warns
  (service named, DSN never logged). Opt out per battery with
  `dedicated_pool: true` (new option on `core/db.Options` and every
  db-backed options struct; `db` itself ignores it); `cache`, `session`,
  `idempotency`, and `scheduler` also accept `dsn` for their `db`/`postgres`
  adapters. Existing files keep working unchanged: sharing activates only
  on exact-DSN (or same-file) match, and downgrading requires removing the
  new fields (strict decoding). Queue, workflow, and scheduler under
  sustained load should use `dedicated_pool: true` with a separate
  database (Solid Queue guidance). Wiring lives in the battery cores now:
  each db-backed core exposes `RegisterShared`/`OpenShared` (adapters
  register their `OpenFromDB` constructors there), so the container
  resolves pools via `db.Open` plus the battery cores only — the scaffold
  floor no longer vendors pgx/modernc and consumers need no lockstep
  adapter replaces.
- `adapters/cache/memory`: reimplemented on
  `shared/lrucache.TTLCache[string, []byte]` (deleted the hand-rolled
  map+list; `SetIfAbsent`/`CompareAndDelete`/`CompareAndExtend` and the
  `Increment`/`Decrement` CAS loop delegate to the library's atomic
  primitives; the janitor calls `PurgeExpired`). One deliberate
  boundary change: entries now expire exactly at their TTL deadline
  (`!expiresAt.After(now)`) instead of staying live until strictly
  after it — a read landing on the exact deadline nanosecond now
  misses. `cachetest` conformance passes identically before/after.

### Fixed

- `cmd/zever`: `doctor` and `config show` now resolve their config through
  `config.Load`, so `zever.yaml`/`.yml`/`.json` discovery and `SERVICE_FIELD`
  env overrides apply when `--config` is absent (previously both silently
  ignored the file and env layers and used `config.Default()`), matching how
  the container resolves config.
- `cmd/zever`: `check`, `fmt`, `routes`, `graph`, `check-boundaries`,
  `explain`, and `db migrate` now prefix "no input .zen files" and
  file-read errors with their own command name instead of the shared
  `zever compile:` text.
- `config`: env parse errors now name the offending variable (e.g.
  `DB_MAXCONNS`) and the expected kind, not just the field.
- Docs: README quickstart adds the missing `cd hello` step; the tutorials
  overview no longer claims `examples/` has no separate Go modules.
- `zever new`: scaffolded README points at the real
  `generated/zenorm/orm/gen/app` query-builder path instead of
  `internal/orm`.
- `adapters/auth/jwt`: propagate `SignedString` errors instead of
  minting an empty token; reject empty `sub` on verify.
- Default lease owners in `adapters/workflow/postgres` and
  `adapters/scheduler/postgres` are now random per driver (via the new
  `shared/dbconn` helper, matching `adapters/queue/db`) instead of the
  static `"workflow-owner"`/`"scheduler-owner"`: two replicas sharing
  one database no longer share one lease-holder identity. Table-name
  validation errors from these adapters (and `queue/db`) now wrap the
  shared `dbconn.ErrInvalidTableName` sentinel, so the text gains a
  `dbconn:` segment (e.g. `postgres: dbconn: invalid table name:
  "no-dashes!"`); match with `errors.Is` going forward.
- `core/authz`: nil `Auth`/`Checker` now fail closed
  (`Unauthenticated`/`PermissionDenied`); clone claims/roles/subjects
  across context boundaries.
- `adapters/permission/casbin`: cancelled context now fails closed.
- `core/auth/revocation/memory`: ignore already-lapsed entries like the
  redis store; synchronous ticker construction.
- `adapters/webhook/queue`: the one-permanent-goroutine-per-event consumer
  is replaced by a fixed shared worker pool (`DefaultConsumerWorkers`), so a
  long-lived process with churning event names no longer grows consumer
  goroutines without bound. Each event is owned by exactly one worker and
  served round-robin, preserving per-event delivery order; `Close` stops the
  pool under the same deadline as before.
- `core/job`: `Scheduler` no longer stores a `context.Context` (previously
  in an `atomic.Value`, hidden from the `containedctx` linter, overwritten
  by concurrent `Run` calls, and retaining request-scoped values). Fires
  now abort off a lazily created lifecycle channel closed when the owning
  `Run` returns; `Run` remains the only ctx entry point.
- `adapters/scheduler/postgres`: cron ticks are rooted in a
  scheduler-owned background ctx instead of the registration request ctx
  (which retained caller values for the entry's lifetime), and each fire's
  load/claim/dispatch now runs under a bounded timeout (new
  `Options.FireTimeout`, default `DefaultFireTimeout` 30s), so a hung DB
  can no longer block the tick goroutine until `Stop`.

### Removed

- `adapters/webhook/sqlite`: removed mistaken adapter module and its
  `core/webhook` surface (`AdapterSQLite`, `Options.DSN`). `http` and
  `queue` remain; default stays `http`. Config schema, scaffold list,
  adapter matrix, and Dependabot entries updated (regen also picks up
  previously missing `kms`/`vault`/`workflow/postgres`/`traceprop`
  entries).
- `adapters/search/sqlite`: folded into `adapters/search/postgres`, which
  is now the single DB-backed search adapter (postgres tsvector/GIN on a
  postgres URL DSN, embedded sqlite FTS5 otherwise; empty DSN selects
  `:memory:`). The hand-rolled `pool.go` files are gone: construction
  flows through `adapters/db/postgres` (TLS 1.2 floor) and
  `adapters/db/sqlite`, writes go through the `orm` typed builder, and
  `NewFromDB`/`OpenFromDB` share a caller-owned `coredb.DB`.
  Migration for direct importers: replace
  `adapters/search/sqlite` with `adapters/search/postgres`
  (`sqlite.New(search.Options{...})` becomes
  `postgres.New(postgres.Options{...})` or `search.Open`); the `sqlite`
  adapter name keeps resolving via `postgres.Register`, so `zever.yaml`
  files and the `sqlite` default need no change.
- `adapters/vectorstore/sqlite`: folded into
  `adapters/vectorstore/pgvector`, which is now the single DB-backed
  vectorstore adapter (pgvector cosine on a postgres URL DSN, embedded
  sqlite brute-force otherwise; empty DSN selects `:memory:`). The
  hand-rolled `pool.go` files are gone: construction flows through
  `adapters/db/postgres` (TLS 1.2 floor) and `adapters/db/sqlite`,
  sqlite-leg writes go through the `orm` typed builder, and
  `NewFromDB`/`OpenFromDB` share a caller-owned `coredb.DB`.
  Migration for direct importers: replace
  `adapters/vectorstore/sqlite` with `adapters/vectorstore/pgvector`
  (`sqlite.New(vectorstore.Options{...})` becomes
  `pgvector.New(vectorstore.Options{...})` or `vectorstore.Open`); the
  `sqlite` adapter name keeps resolving via `pgvector.Register`, so
  `zever.yaml` files and the `sqlite` default need no change.

### Added

- `core/workflow`: `PostgresOptions` (`DSN`/`Table`) embedded in `Options`
  plus `Postgres` adapter const, so `adapters/workflow/postgres`
  registers a working `workflow.Open` factory (sqlite on empty DSN,
  postgres when set) instead of failing closed; flows through
  `config` file/env and `container.Workflow()` unchanged.
  `docs/production.md` gains a Workflows section and correct
  Vault/KMS coverage.
- `docs/production.md`: single operator-facing production deployment guide
  tying together the generated server contract (`/healthz`/`/readyz`,
  `signal.NotifyContext` + `Shutdown`), Postgres pool/TLS/DSN rules,
  read-only env secrets baseline, rate-limit wiring, `orm/migrate`
  plan/apply/rollback, OTLP observability, CI gates, and
  pre-deploy/deploy/post-deploy checklists.
- `core/middleware`: new `CORS` HTTP middleware over an app-level
  `Options` value (explicit origin allowlist echoed, never reflected;
  preflight `OPTIONS` answers 204 or 403; `Vary: Origin` always; invalid
  options fail closed with 500). Ships `Options.Validate` and
  `DefaultCORSMaxAge`, wired outermost in the `bookings`, `showcase`, and
  `todo` example servers. See `basics/cors`.
- `examples/showcase/Dockerfile`: worked multi-stage container image
  (`golang:1.27.1-bookworm` static build,
  `distroless/static-debian13:nonroot` runtime, non-root
  `ENTRYPOINT ["/server"]`); `docs/production.md` and `deployment.mdx`
  link it with `/healthz` vs `/readyz` probe mapping.
- `zever new`: emit `compose.yaml` when picked batteries need external
  infra (`db`/`search`/`vectorstore` on postgres/pgvector, or any
  `cache`/`queue`/`session`/`ratelimit`/`lock`/`eventbus`/`idempotency`
  on redis) — `app` plus health-gated `postgres:16-bookworm`/`redis:7-alpine`
  with persistent volumes and env-carried secrets; local-only picks emit
  no file.
- `core/secrets/secretstest`: new conformance kit mirroring
  `core/cache/cachetest` (missing-key, round-trip, list, delete, name
  validation, close), run against `env` and `vault` adapters.


### Fixed

- `adapters/workflow/postgres`: `Signal`/`Cancel` are state-guarded
  (`state=running` predicate + row-count check) so a race with completion
  returns `RunCompletedError`/`ErrUnknownRun` instead of silent success.
- `core/job`: worker consume path uses `traceprop.StartConsumeSpan`
  (`queue.consume`) instead of bare `Extract`, restoring trace continuity.
- `adapters/crypto/kms`: reject overlong `KeyID`/`KeyIDs` in
  `Options.Validate` and before the KMS call (no wasted `GenerateDataKey`);
  `Decrypt`/`Encrypt` now preserve `context.Canceled`/`DeadlineExceeded`
  via `errors.Join` so callers can `errors.Is` cancellation.


## [v0.5.3] - 2026-09-29

### Added

- `tools/modgraph`: Go port of the `fix-module-graph.py` build tool
  (exact `go/parser` import analysis instead of regex over template
  text). The CI module-graph job runs it; new requires pin to the
  CHANGELOG lockstep version.

### Fixed

- Module-graph gate reconciled with `go mod tidy` (direct-import
  closure; 250 unused requires dropped across 55 drifted modules).
- Installation guide: the external-consumer snippet now registers the
  `sqlite` adapter (`UnknownAdapterError` before), verified verbatim
  from a scratch module outside `go.work`.

## [v0.5.2] - 2026-09-29

### Added

- `core/cache/cachetest`: new `Conformance(t, factory)` kit covering
  `Get`/`Set`/TTL expiry (short TTL plus deadline-polled expiry, no
  `time.Sleep` sync), `SetIfAbsent`, `Delete`, `Increment`/`Decrement`,
  `Exists`, and `Close`. External adapters run it against their factory;
  in-tree proof is `core/cache/cachetest/conformance_test.go` over the `memory`
  adapter. See `docs/writing-a-plugin.md`.
- `docs/writing-a-plugin.md`: plugin authoring guide — new adapter for an
  existing battery via `Register`/`Open` (worked `cache` example with a
  string adapter name), entirely new battery via `RegisterPlugin`/`Resolve`
  plus `Plugins` config and `RegisterPluginValidator` (worked `sms`
  example), `PluginAPIVersion` contract, and repo naming
  `zever-adapter-<name>` / `zever-battery-<name>`.
- **Breaking:** every battery `Adapter` changes from an `int` enum to a
  string type (`Memory Adapter = "memory"`, etc.). `String()` is now the
  identity (`"unknown"` only for empty) and `ParseAdapter` accepts any
   non-empty name, so out-of-tree adapters (`core/cache.Adapter("foo")`) resolve
   via `Register`/`Open` and round-trip through config-file/env selection
   like core adapters. Numeric `Adapter(...)` conversions and exhaustiveness
   over the old constants no longer compile.
- `zever new` battery picker (opt-in only): `--interactive` (`-i`,
  `ZEVER_INTERACTIVE`) runs the `huh` wizard to pick extra batteries;
  without it the command never prompts and scaffolds the floor set
  (`log`+`router`) plus whatever `--batteries a,b` and
  `--adapters b=a` select. `--list-batteries` prints the
  battery/default-adapter table and exits.
- `examples/external-sms`: out-of-tree proof battery template
  (`github.com/example/zever-sms`, deliberately outside the workspace)
  proving a third-party author can ship a battery on public modules only
  (`config`, `container`, `shared/registry`). Ships `SMS` interface plus
  `Options{From}` with `Validate`, string `Adapter` with
  `Register`/`Open`, a deterministic network-free stub, and plugin
  wiring (`config.RegisterPluginValidator` plus
  `container.RegisterPlugin("sms", container.PluginAPIVersion,
  BuildFromConfig)`); `app_test.go` asserts `Resolve`/`Send` end to end
  plus the version-mismatch failure. See `docs/writing-a-plugin.md`.
- `core/queue/queuetest` and `core/storage/storagetest`: new
  `Conformance(t, factory)` kits mirroring `core/cache/cachetest`, so
  queue and storage adapters prove parity the same way cache adapters do.
  See `docs/writing-a-plugin.md`.
- `zever new` picker veto/skip semantics: `-y`/`--yes` skips the whole
  wizard (floor plus flags only), while `--batteries` skips the battery
  prompt and `--adapters b=a` skips that battery's adapter prompt, so
  every wizard answer has a flag; `ZEVER_INTERACTIVE` also opts in. See
  `cmd/zever/new.go` (`wantNewPicker`, `runBatteryPicker`).
- `zever add` flags and plugin batteries: `--adapter <name>` (must agree
  with the positional slash form when both are given), `--force` to
  overwrite an already-pinned `zever.yaml` stanza (re-adding without it
  now errors naming the current adapter), and plugin-battery support for
  names outside `config.Default()`'s set — a `plugins:` stanza in
  `zever.yaml`, a `--module <path[@version]>` require in `go.mod`
  (version defaults to `v0.1.0`), and a
  `container.RegisterPlugin`/`Resolve` wiring comment in
  `internal/app/app.go`. See `cmd/zever/add.go`.

### Changed

- **Breaking:** scaffold floor slimmed to `log`, `router`. `observability` (`stdout`), `permission` (`noop`), `queue`
  (`memory`) and `ratelimit` (`memory`) are no longer pinned into every
  generated `app.go`/`zever.yaml`: all four defaults are light
  (dependency-free, still explicitly registered by the generated `app.go`),
  entrypoints keep resolving them at runtime with no change; new scaffolds
  just stop pinning them. Re-add by picking the battery in `zever new`.
- **Breaking:** every third-party adapter is now a nested Go module with
  its own `Register()` (81 adapter modules: all `*/redis` adapters, `db/sqlite`,
  `db/postgres`, `auth/jwt`, `auth/oidc`, `ai/anthropic|openai|gemini`,
  `billing/stripe|paddle|stub`, `payment/stripe|paddle`,
  `search/sqlite|postgres|meilisearch`,
  `vectorstore/sqlite|pgvector|qdrant`, `storage/s3|r2`,
  `media/local|s3`, `document/local`, `notification/fcm|twilio`,
  `permission/casbin`, `router/fiber`, `analytics/posthog`,
  `flag/firebase`, `geo/google`, `log/zerolog`, `observability/otlp`,
  `password/argon2`, `scheduler/embedded`, `webhook/sqlite`, plus the
  `storage/s3core` shared library). The `container` wires nothing itself;
  every adapter resolves only after the host calls its
  `Register()`. Unregistered adapters fail
  resolution with `UnknownAdapterError` hinting at the forgotten call.
  `cmd/zever` registers every family at startup, so CLI behavior is
  unchanged.
- **Breaking:** `container/adapters` bundles deleted entirely (flat bundle
  and per-family subpackages gone); every adapter resolves only after the
  host calls its module `Register()`. `RegisterAll` is deleted; call the
  `Register()` of each adapter module you resolve.
- **Breaking:** `zever new` emits one `require` (+ local `replace` in
  `ZEVER_FRAMEWORK_PATH`/checkout mode) per chosen nested adapter module, so a blank
  app's module graph contains only its selection. In checkout mode it also
  emits version-less replaces for the remaining nested modules so
  `go list -m` resolves offline without widening requirements.
- Shared SDK-independent option fields moved to light shared packages
  so facades compile without SDKs: `shared/redisopt` (ex-`internal/redis`
  address/prefix/validation helpers; `shared/redisclient` keeps only client
  constructors) and `shared/providersopt` (ex-`internal/providers`
  `Common`/`ValidateEndpoint`/`PaddleEndpoint`; `shared/providersclient` keeps
  only the Stripe client constructors).

### Security

- **Breaking:** `payment/stub` `WebhookEvent` now fails closed with
  `payment.ErrInvalidSignature` instead of returning a static
  `stub.event`. `stub.New` documented test-only, never production.

### Changed

- **Breaking:** repo is now a Go multi-module monorepo: battery
  interfaces plus `Register`/`Open` registries in `core/<b>`, one Go
  module per adapter in `adapters/<b>/<a>` with `Register()` wiring it
  into its `core` registry, SDK-free helpers in `shared/*`, plus
   `config|container|orm|dsl|cmd/zever` modules. Local dev resolves them
   through the committed `go.work` workspace at the repo root (144 `use`
   entries covering every workspace module (`examples/external-sms` and `docs/examples` stay outside `go.work` by design)).
  Releases are lockstep: one version, per-module tags `<path>/vX.Y.Z`
  via `tools/tag-release.sh`. See `docs/src/content/docs/getting-started/migration.mdx`.
- **Breaking:** `container/adapters` flat bundles deleted
  (`RegisterAll` deleted); every adapter resolves only after the
  host calls its module `Register()`. Unregistered adapters fail with
  `UnknownAdapterError` hinting at the forgotten call.
- Shipped: `zever add <battery>[/<adapter>]` battery-adding command
  (`cmd/zever/add.go`; adds a require plus the `Register()` wiring to the
  calling project) and `tools/migrate-imports.sh` import rewriter
  (old flat paths to `core/`/`adapters/`/`shared/`/`dsl/` modules).
- **Breaking:** `payment/paddle` `New` now rejects empty `WebhookSecret`
  with `payment.ErrMissingWebhookSecret`, mirroring `payment/stripe`.
- **Breaking:** `authz.BearerTokenFromMD` now rejects multiple
  `authorization` metadata values (returns empty), mirroring
  `authz.BearerToken` fail-closed handling.

### Fixed

- Scaffolded `internal/app/app.go` imports and registers exactly the
  selected batteries: one named import plus `Register()` call per adapter
  module (e.g. `dbsqlite.Register()` for `db/sqlite`,
  `logslog.Register()` for `log/slog`), and nothing else.

- `zever new --help` and `zever compile --help` now expose the real flags
  (`--module`, `--dir`, `--framework-version`, `--force`,
  `--interactive`/`-i`, `--batteries`, `--adapters`, `-y`/`--yes`,
  `--list-batteries` for `new`;
  `--backend`, `--out` default `./generated` for `compile`) via
  cobra-registered flags and shared stdlib flag-set helpers, so
  `-h`/`--help`/`help` stay uniform with exit 0; execution still uses the
  legacy parsers (exit 0 ok / 1 runtime error / 2 flag misuse unchanged).

## [v0.4.0] - 2026-09-25

### Changed

- **Breaking:** `zever` CLI migrated to Cobra: every subcommand is a
  `*cobra.Command` under one root with persistent `--quiet`/`--no-color`
  flags; exit codes are now `0` ok / `1` runtime error / `2` flag misuse
  (Cobra's `SilenceUsage` convention — bad flags no longer print the
  full usage-plus-error mix); `-h`/`--help` works uniformly, fixing the
  hand-rolled `-h` gaps on `routes`/`graph`/`check-boundaries`/`explain`;
  new `zever completion <bash|zsh|fish|powershell>` and
  `zever docs --dir <dir>` (man pages + markdown) commands; release
  builds strip via `make build-release`
  (`-trimpath -ldflags="-s -w"`). (`bubbletea`/`teatest` remain as
  test-only dependencies driving the kept `huh` prompt tests — unrelated
  to this migration; see the TUI-removal entry below, left intact.)
- **Breaking:** `zever` dashboard/TUI removed. `cmd/zever` is now
  flags-only: bare `zever` (TTY or not) prints usage to stderr and exits 1
  (`zever: missing subcommand`) instead of opening an interactive
  dashboard. `bubbletea`/`bubbles`/`lipgloss` removed from production
  dependencies (`bubbletea`/`teatest` remain as test-only dependencies
  driving the kept `huh` prompt tests); `-i`/`--interactive` (`ZEVER_INTERACTIVE`) `huh` guided prompts
  (`prompt.go`) are kept.

### Added

- `zever --version` (`-V`) prints the CLI release version (`zever vX.Y.Z`) to stdout.

## [v0.3.0] - 2026-09-25

### Added

- `config/schema/`: a hand-authored JSON Schema (draft 2020-12) for
  `zever.yaml`/`.yml`/`.json`, covering every one of `config.Config`'s 34
  services' adapters and typed options, plus mechanically-generated
  `.yaml`/`.yml`/`.toml` equivalents for tooling that expects a schema file
  in its own format (`config/schema/README.md` covers editor wiring and the
  caveat that `.toml` is schema-only -- zever itself still only loads
  `.yaml`/`.yml`/`.json`). `config.SchemaJSON` embeds the canonical file for
  in-process use. `zever new` now scaffolds a `zever.schema.json` into every
  new project and points the generated `zever.yaml` at it via a
  `yaml-language-server` modeline comment, so scaffolded projects get
  config autocomplete/validation out of the box.

### Changed

- **Breaking:** `zever new` no longer accepts `--framework-path`. It only
  ever existed to replace a scaffold's `go.mod` against a local zever
  checkout for framework development -- now that the framework is a
  published module with real tagged releases, auto-detection (walking up
  from the working directory for a `go.mod` declaring "module
  github.com/zenta-dev/zever") still covers that case with no flag needed,
  and `--framework-version` still covers pinning a scaffold to a specific
  published release. `--framework-version` is unchanged.
- **Breaking:** `zever generate module` no longer accepts `--lang`. It only
  ever accepted `"go"` (its own help text said so) -- pure speculative
  surface for a language target that doesn't exist yet; it can come back
  without a breaking change once one does.
- `zever breaking <old> -- <new>` now accepts a bare directory on either
  side of `--`, recursively discovering every `.zen` file under it (flat,
  versioned, or arbitrarily nested layouts), instead of requiring an
  explicit glob or file list -- matching every other schema-consuming
  command's auto-discovery.
- `zever db migrate` now falls back to the same recursive schema-dir
  auto-discovery as `compile`/`check`/`routes`/etc. when run
  non-interactively with no file arguments, instead of failing immediately.

- **Breaking:** new `internal/providers` package (mirrors the existing
  `internal/s3opts` pattern shared by `storage/s3`/`storage/r2`/`media/s3`)
  holds the connection-option fields and SDK client construction
  `billing`/`payment`'s Stripe and Paddle adapters were independently
  duplicating: `billing.Options`/`payment.Options` now embed
  `providers.Common` (`SecretKey`/`APIKey`/`Endpoint`/`Sandbox`) instead of
  redeclaring those fields (JSON/TOML/YAML shape unchanged -- embedding
  promotes the same field names); both packages' `DefaultHTTPTimeout` are
  now aliases of `providers.DefaultHTTPTimeout` instead of two independently
  declared identical constants; all 4 adapters (`billing/stripe`,
  `payment/stripe`, `billing/paddle`, `payment/paddle`) now call
  `providers.NewStripeClient`/`providers.PaddleEndpoint` instead of each
  separately constructing the same `*stripe.Client`/resolving the same
  sandbox-vs-production URL. `billing.Options.Validate()`'s endpoint error
  messages are now as specific as `payment.Options.Validate()`'s always
  were (separate "must include scheme"/"must include host" reasons instead
  of one generic "must be a valid url" covering every failure shape) --
  breaking only in the sense that error *text* changed; the `*InvalidOptionsError`
  type and `errors.Is(err, ErrInvalidOptions)` contract are unchanged.

- `internal/dsl/backend.Backend` gained an optional `ContextBackend`
  extension (`GenerateContext(ctx, schema)`); `internal/dsl/compile` gained
  `CompileContext`/`WithSchemaDirContext`, which call it when a backend
  implements it. `protogogen` (the only backend doing real I/O -- it shells
  out to `go tool protoc-gen-go-grpc`) now implements it, so a caller with a
  real deadline/cancellation source can bound that subprocess instead of it
  always running with a fabricated `context.Background()`. Additive: plain
  `Compile`/`WithSchemaDir` and every other backend are unchanged.
- `ai/ollama.New` now matches `ai.Factory` directly
  (`func(ai.Options) (ai.AI, error)`), so `ai.Register(ai.Ollama,
  ollama.New)` needs no translating closure, unlike before -- the old
  `Options`-taking constructor is renamed `NewWithOptions` (used when the
  `Transport` test-seam field is needed; `ai.Options` has no equivalent).
  `container/services.go`'s hand-written closure is removed.
- `router/stdhttp.New` now calls `Options.Validate()`, matching
  `router/fiber.New` -- previously an `AppName` invalid for every adapter
  succeeded silently against `stdhttp` while failing against `fiber`,
  breaking adapter-swap transparency.

### Fixed

- **Security:** `internal/dsl/backend/atlas`'s HCL renderer spliced an
  `@schema(...)` value unquoted into `schema = schema.<name>` (an HCL
  *reference* expression, which cannot be quoted like a string literal --
  unlike the properly-`%q`-quoted `schema "<name>" {}` *declaration* three
  lines above). `@schema(...)` accepts an arbitrary string literal at the
  resolver, so a value containing HCL-structural characters could inject or
  malform the generated migration file. Now validated against a bare-HCL-
  identifier regex at render time, erroring clearly instead of emitting
  unsafe HCL.
- **Correctness:** `internal/dsl/backend/openapi`'s synthesized request
  schemas were namespaced only by RPC name + module (not by service name,
  unlike the correctly-namespaced `OperationID`), with no collision guard —
  unlike every sibling `add*Schema` method and unlike `addOperation`'s own
  path/method collision check. Two services in the same module both
  declaring a same-named RPC with body params silently produced ONE
  component schema (whichever was processed last); regenerating
  `examples/showcase`'s output surfaced a real instance of this — a
  self-referencing `shop_CheckoutRequest` schema. Request schemas are now
  namespaced by `<Service><RPCName>Request` and a collision returns a clear
  error naming both owning RPCs, matching `addOperation`'s existing
  contract. All affected example `openapi.json` outputs regenerated.
- `internal/dsl/backend/openapi`'s `toInt64`/`toFloat64` silently defaulted
  to `0` on an unexpected `@validate` argument type, emitting a bogus
  `minLength: 0`/`minimum: 0` constraint into generated OpenAPI with no
  diagnostic (same root-cause class as gogen's `numericLiteral` bug fixed
  earlier, but this one wasn't). Now panics on the invariant violation,
  matching gogen's fix.
- **Security:** `ai/openai.New` silently dropped the configured
  `ai.Options.Timeout` (`newHTTPClient()` always passed `0`, meaning no
  client-side timeout) while every sibling `ai/*` adapter honored it — a
  hung/slow upstream could block a goroutine indefinitely. Now threads the
  configured timeout through, like `anthropic`/`gemini`/`ollama` already
  did.
- `ai/ollama.Stream` buffered the entire NDJSON response (up to 16 MiB)
  before decoding/emitting anything, giving zero time-to-first-token
  improvement over `Generate` and defeating the point of a streaming API.
  Now reads and emits one line at a time as bytes arrive, with the same
  total-byte cap enforced incrementally instead of via one buffered read.
- **TUI:** pressing esc during a running `db migrate`/`rollback`/`seed`
  screen only changed local UI state and popped the screen; the already-
  running operation (a real subprocess/in-process capture) kept executing
  unattended in the background with no way to stop it — the "esc cancel"
  hint promised behavior the code didn't deliver. `ExecModel` now creates a
  cancelable context in `NewExec` and cancels it on esc, so any `ExecFunc`
  that honors `ctx` (as every `ExecFunc` signature already requires) is
  actually canceled. Note: `runDBMigrate`/`runDBRollback`/the seed launcher
  themselves don't currently honor `ctx` (synchronous CLI entrypoints /
  signal-forwarding launcher, by established repo convention) — this fix
  makes cancellation reach any `ExecFunc` that respects it, but making
  those specific CLI entrypoints interruptible mid-flight is a separate,
  larger design question not addressed here.
- **TUI:** a late `ExecDoneMsg`/`ExecErrMsg` (from an operation finishing
  after the user already canceled) could silently flip a canceled
  `ExecModel` back to done/failed. `Update` now ignores both once
  `state == ExecCanceled`.

- `session.RedisOptions`/`ratelimit.RedisOptions`/`idempotency.RedisOptions`/
  `eventbus.RedisOptions` now embed `zredis.Options` (connection AND
  pooling: `PoolSize`, `MinIdleConns`, `PoolTimeout`, `MaxConnIdleTime`,
  `MaxConnLifetime`) instead of just `zredis.ConnectOptions`. Previously
  every Redis-backed adapter silently dropped pooling configuration to
  go-redis's defaults with no way to tune a hot ratelimiter/idempotency
  store/session backend/eventbus under load. Existing field access
  (`opts.Redis.Addr`, etc.) is unaffected since `Options` still embeds
  `ConnectOptions`.
- `orm/dialect.JSONTableDialect` removed: implemented by neither in-tree
  dialect and asserted nowhere in the repo -- dead interface surface.
- `vectorstore/pgvector.Delete` now does one round trip (`Exec` +
  `RowsAffected`) instead of a SELECT-then-DELETE pair, matching
  `search/postgres.Delete`'s existing pattern.
- Removed `vectorstore/pgvector.formatVector`: dead code exercised only by
  its own test, duplicating (and at risk of drifting from) the real
  `embeddingCodec.Encode` path `Upsert`/`Query` actually use.

- **Breaking:** `orm`'s dozen-plus mirrored enums (`Op`, `NodeKind`,
  `CompoundOp`, `LockMode`, `NullsOrder`, `WindowFunc`, `FrameMode`,
  `FrameBoundKind`, `JoinType`, `AggFunc`, `JSONOp`, `FTSOp`, `FTSMode`, the
  internal set-op kind) are now type aliases onto their `orm/render`
  counterparts instead of independently redefined types kept in sync only
  by comment and matching `iota` order. Every hand-written boundary
  conversion (`render.Op(n.Op)` and friends) is removed; a future reorder/
  insert mistake in either package's constant list is now a compile error
  instead of a silent SQL-mis-rendering bug. Public names/values are
  unchanged for `orm` package consumers -- `JoinType.String()` moved to
  `render.JoinType` (a type alias cannot declare its own methods) but
  behaves identically.
- `orm/dialect` gained `CheckDistinctOn`, the single DISTINCT ON validation
  rule now shared by `orm.Query`'s build-time gate and
  `orm/render`'s render-time gate (previously two independent
  implementations with two different error-message prefixes that could
  drift on what "supports DISTINCT ON" means).

- **Breaking:** `orm.CursorKeyValue` no longer accepts defined/enum types
  via `~` (e.g. a codegen'd `type Status string`) -- only the exact types
  `string | []byte | int64 | int32 | float64 | float32 | bool | time.Time`.
  Cursor encode/decode now dispatch with a plain type switch on the boxed
  dynamic type instead of `reflect`; a cursor over an enum-typed column
  must build its keyset predicate by hand (`AfterTuple` or a manual
  `Predicate`) instead.
- **Breaking:** `gogen.New()`'s zero-config import-path formula now matches
  `NewWithPBImportRoot`'s flat, protogogen-accurate layout
  (`<root>/<module>`, bare `<root>` for the implicit module) instead of the
  legacy `<root>/zeverv1` / `<root>/zever/<module>` convention. `Backend`
  no longer has two formulas to choose between; all committed goldens and
  example outputs regenerated.
- **Breaking:** `payment.Payment.Refund` now takes an idempotency key
  (`Refund(ctx, id, amount, key)`); `payment.Options` gained an
  `Idempotency idempotency.Store` field (nil skips the guard). Paddle has
  no native idempotency mechanism, so the guard lives at the adapter
  layer: `Begin` (fingerprint `sha256(id|amount)`, 24h TTL), replay
  returns nil without re-calling the provider, failure calls `Forget` so
  genuine retries aren't blocked. Stripe `Refund` additionally sets the
  native `Idempotency-Key`. Empty key preserves the old behavior.
- `vectorstore.Vector`/`search.Document` gained `Validate()` rejecting
  non-JSON-marshalable `Metadata` at the package boundary
  (`ErrInvalidMetadata`), wired into every adapter write path
  (pgvector/qdrant/sqlite `Upsert`/`UpsertBatch`,
  postgres/sqlite/meilisearch `Index`/`IndexBatch`) before any DB/network
  call.
- `internal/httpclient.NewClient` now sets `MaxIdleConnsPerHost: 32`
  (new `DefaultMaxIdleConnsPerHost`) on both cloned transports instead of
  Go's default of 2, so repeated calls to one provider host reuse
  connections.
- `search/postgres.DefaultDDLTimeout` raised 5s to 10s to match
  `vectorstore/pgvector` (shared 10s floor: pgvector ivfflat index builds
  are slower than plain B-tree/GIN; single connect-plus-DDL budget so the
  two can't drift).
- `workflow.Workflow`/`StepFunc` docs now state the JSON-marshalable
  contract: non-marshalable input/signal values fail lazily at `Query`
  time, not at `Start`/`Signal` time.
- `workflow.StepFunc` and the `StepRegistrar` interface are now exported
  so hosts register steps through the interface (the memory adapter
  implements it) instead of reaching into a concrete adapter type.
- `authz.Policy` now carries `OwnerField` from the resolved
  `permission: check(..., owner_field: ...)` through to the `gogen`
  output (`OwnerField: "user_id"`), with the proto and OpenAPI backends
  rendering the same `owner_field` value.

### Fixed

- **Bug:** `examples/bookings` `handleListBookings` did `defer func() { _ =
  rows.Close }()` -- taking the *method value* of `rows.Close` and never
  calling it -- leaking the `db.Rows`/statement handle on every call. Every
  other `rows.Close()` in the same file was already correct.
- **Security/correctness:** `examples/showcase`'s checkout read stock,
  validated in Go, then decremented later with no transaction and no
  `WHERE stock >= ?` guard: two concurrent checkouts for the same product
  could both pass validation and both decrement, overselling inventory.
  `handleCheckout` now wraps order + line items + stock decrements in
  `db.WithTx`, and the decrement itself carries `WHERE stock >= ?`, checked
  via `RowsAffected`, so only one of two racing requests can win. Added a
  concurrency regression test.
- **Correctness:** `examples/demoapp`'s `createOrder` inserted the order and
  its line items as separate, non-transactional statements, explicitly
  discarding each line-item insert's error -- a failed insert silently
  produced an order with missing line items and no visible error anywhere.
  Now wrapped in `db.WithTx`; a failed insert rolls back the whole order.
- **Security:** none of the 4 example apps' `decodeJSON` bounded the
  request body size, letting a client force unbounded buffering before any
  validation ran. All four now wrap the body in `http.MaxBytesReader`
  (1 MiB).
- `webhook/http`'s delivery-failure error included only the numeric status
  code; sibling adapters (`document/remote`, `geo/osm`) already include a
  truncated response body. `webhook/http` now does too (512 bytes), making
  "why didn't my webhook fire" debuggable from the error text alone.
- `vectorstore/pgvector.Query` converted the same embedding bytes to
  `string` twice; now converts once and reuses the value.
- `orm/capability_test.go` gained compile-time `var _ dialect.XDialect =
  New()` assertions for the 6 capability interfaces
  (`ExplainDialect`/`DistinctOnDialect`/`ExtendedLockingDialect`/
  `TablesampleDialect`, plus the 2 already covered by other test files)
  that previously had none, closing the only real gap behind the "25
  capability interfaces" structural concern raised in an earlier review
  round -- both in-tree dialects already implement the full capability
  surface identically, so a full `Capabilities`-struct rewrite (evaluated
  and rejected this round; see the design note in `orm/dialect/dialect.go`
  history) would have touched ~20 files for no measurable safety gain over
  this 6-line fix.
- `cmd/zever new -h`'s help text hardcoded a stale framework version
  (`v0.1.1`); it now interpolates `defaultFrameworkVersion` (currently
  `v0.2.0`).
- `cmd/zever queue:work -h`/`db seed -h` printed a `--once` example flag
  that has never existed (scaffolded worker/seed entrypoints never call
  `flag.Parse()`); the misleading example is removed.
- **Security:** `cmd/zever generate tinker --dir` accepted a path-traversal
  directory (e.g. `../../../../etc/cron.d/x`): the guard checked
  `isTraversalName(dir) && (dir == ".." || filepath.IsAbs(dir))`, which
  rejects a literal `..` or an absolute path but not a `../`-relative one.
  Replaced with `hasParentTraversal`, which accepts legitimate multi-segment
  dirs (`./tinker/shim`) while rejecting any path that escapes upward.
- **Security:** a zenorm module name containing `/` or `..` (e.g. from an
  unusual schema directory layout) could produce a generated output path
  that escaped the intended `orm/gen/<module>/` tree; `zenorm.moduleNaming`
  now rejects a path-unsafe name outright, and `cmd/zever generate`'s
  `writeExtractedORM` adds a defense-in-depth confinement check at the
  actual write site.
- **Security:** `orm.UnsafeRaw`/`render`'s raw-fragment renderer silently
  dropped extra bound args (or left a `?` marker unbound) when a caller's
  arg count didn't match the fragment's marker count -- e.g.
  `UnsafeRaw("status = ?", "active", tenantID)` (a typo missing a second
  `?`) would silently drop `tenantID`, broadening the query. It now errors
  on any marker/arg count mismatch in either direction.
- `internal/dsl/resolver`: `validateFieldsIn`'s Ref-chain walk had no depth
  guard (only a cycle guard), so a deep chain of distinct message/entity
  declarations could stack-overflow the resolver; added a 200-level depth
  cap mirroring the parser's `maxValueDepth`.
- `internal/dsl/resolver`: `dispatch: Job(build_payload(x, y))` silently
  collapsed the nested call argument to the bare string `"build_payload"`,
  discarding `x`/`y` with no diagnostic. A nested-call dispatch argument is
  now rejected with a diagnostic instead.
- `internal/dsl/breaking`: `Compare` skipped any module present in only one
  of the two schemas, so a fully deleted module (every entity/service/RPC
  it declared) produced zero `Change` entries and `HasBreaking` wrongly
  reported "not breaking". Whole-module removal/addition are now reported
  as `KindModuleRemoved` (breaking) / `KindModuleAdded` (non-breaking).
- `internal/dsl/breaking`: field comparison now checks `Optional`
  (optional -> required is breaking) and consults the resolver-populated
  `RenamedFrom`, so a schema-author-declared `@renamed_from` rename is
  reported as a rename (`KindFieldRenamed`) instead of a false-positive
  remove+add pair.
- `orm/option.go`'s `parseInt64`/`parseFloat64` used `fmt.Sscanf`
  (reflection-backed) instead of `strconv.ParseInt`/`ParseFloat` on every
  nullable-numeric-column row scan, contradicting the package's own
  "zero-reflection Scan" design goal.
- `cmd/zever doctor` gained `--strict` (exit non-zero if any battery fails,
  for CI/pre-deploy gating -- the default stays always-nil, since
  `config.Default()` legitimately fails some batteries, e.g. auth/jwt with
  no configured secret) and now closes its `Container` before returning
  instead of leaking any real connections a battery opened.
- Removed dead `-i`/`--interactive` flag definitions from 10 more CLI
  command sites (`doctor`, `generate entity/job/adapter/server/schedule/
  tinker`, `compile`, `fmt`, the shared entrypoint/module-scaffold flag
  parsers) that were always `false`: `peelInteractive` already strips
  `-i`/`--interactive` before `flag.Parse` and sets the global
  `interactiveMode`, matching the pattern `migrate.go`/`tinker.go` had
  already adopted. The long `--interactive=value` form is no longer
  accepted anywhere (it never worked via `peelInteractive` either); use the
  bare flag.
- `config.UnknownServiceError`/`UnknownFieldError` now carry a `Suggestion`
  field and include a "did you mean %q?" hint in their `Error()` text (e.g.
  a typo'd `evenbus:` service block now suggests `eventbus`), matching the
  hint every CLI-facing error path already had.
- `config.Redact`/`RedactedServices` now scan `[]string` values too (e.g. an
  HTTP header list `["Authorization: Bearer xxx", ...]` nested under a
  non-sensitive key like `headers`), redacting `"key: value"`-shaped
  entries whose key half looks sensitive; previously such slices passed
  through untouched. `[]string` values are also now deep-copied rather than
  shared with the caller's original map.
- `auth/jwt`: a token carrying a non-string `jti` claim is now rejected
  outright instead of silently skipping the revocation-store check.
- `internal/dsl/backend/gogen`: `numericLiteral` and `writeFormatCheck` now
  panic on an unexpected argument type instead of silently splicing an
  unescaped `%v`-stringified value into generated Go source or silently
  dropping the validation check -- both cases previously depended entirely
  on an unenforced resolver invariant.
- `internal/dsl/diag.List.Error()` now sorts diagnostics by
  `(File, Line, Col)` (matching `Sorted()`) instead of returning them in
  lexer-then-parser-then-resolver phase order.

- Naming and consistency pass (breaking where noted): canonical `EventBus`
  (`Eventbus` alias retained) and `RateLimit` (`Ratelimit` alias retained)
  accessors; `ErrDuplicateAdapter` canonical sentinel with `ErrDuplicate`
  compatibility alias in `queue`/`cache`/`log`; `CloseTimeoutError.Unwrap`
  now joins `ErrCloseTimeout` so `errors.Is` matches; `queue`/`cache`
  invalid-adapter text built from the sentinel; `log` error text no longer
  embeds `.Error()` calls; `session/cookie.Config` renamed to `Options`
  (`Config` alias retained); `internal/s3opts.Config` renamed to `Options`
  (`Config` alias retained); explicit `json`/`toml`/`yaml` tags on
  `ai`/`db`/`auth`/`cache`/`queue`/`log` top-level `Options`; unified
  `Validate` doc phrasing; `secrets` env-prefix exception documented in
  `config/README.md` and `secrets/doc.go`; CLI separator standard
  documented with `check:boundaries` alias for `check-boundaries`;    DSL
  `datetime` alias for `timestamp`; predecessor-comparison comments
  de-branded.
- **Security:** `geo/static` rejected no path traversal: a `..` element
  in the dataset path could escape the configured directory. Paths are
  now checked for `..` elements before cleaning and rejected with
  `ErrInvalidOptions`.
- `examples/demoapp` maps malformed session ids to 400 (`malformed
  session id`); unknown-but-well-formed ids stay 404.
- `orm` on sqlite now binds `time.Time` as RFC3339Nano UTC text and
  scans timestamps back with sub-second precision intact; previously
  fractional seconds were silently dropped. Second-precision text
  written by older versions still parses.
- `zenorm` maps `json` columns to `orm.JSONText` (which implements
  `sql.Scanner`/`driver.Valuer`) instead of `json.RawMessage`, which
  `database/sql` cannot scan into on recent Go toolchains.

### Changed

- **Breaking:** predecessor `zengo` wire namespace renamed to `zever`
  across the DSL codegen backends: proto package `zengo.*.v1` →
  `zever.*.v1`, `go_package` `gen/zengo/...` → `gen/zever/...`, companion
  import `zengo/annotations.proto` → `zever/annotations.proto`, extension
  options `zengo.annotations.v1.*` → `zever.annotations.v1.*`, gRPC method
  prefixes `/zengo.v1/*` → `/zever.v1/*`, and gogen default import paths
  `<root>/zengov1`, `<root>/zengo/<module>` → `<root>/zeverv1`,
  `<root>/zever/<module>`. All committed goldens and example outputs
  regenerated. `storage/local` env switch `ZENGO_ENV` → `ZEVER_ENV`.
  Historical release notes and absence-guard tests still reference the old
  name intentionally.

- Naming and consistency pass, round two (breaking where noted): scaffold
  and citation versions aligned (`defaultGoVersion` 1.27, framework
  v0.2.0, CITATION 0.2.0, lsp server 0.2.0); `config` fields
  `Eventbus`/`Ratelimit` renamed to `EventBus`/`RateLimit` (yaml tags
  unchanged, files keep loading; compat accessor aliases retained);
  `secrets` enum trimmed to `env` (Vault/GCP/AWS removed) with
  `ErrDuplicateAdapter` canonicalization; `Adapter.String()` unknown
  fallback unified to `"unknown"` (was `Adapter(%d)` in
  cache/lock/queue/storage/workflow); `Register`/`Open`/`ParseAdapter`/
  `Factory` params unified (`adapter`, `factory`, `s`, `opts`); every
  service `Open` now calls `opts.Validate()` first (added missing
  `Validate` to cache/queue/log); file option keys unified to snake_case
  with a migration table in `config/README.md` (env names unaffected);
  close-shape `Shutdown` probe and `secrets.Close(ctx)` rationale
  documented; billing/payment facade boundary, cookie `MaxAge` seconds
  rationale, and typed-UUID ID rule documented.

- Consistency pass, round three (breaking where noted):
  `DuplicateError` renamed to `DuplicateAdapterError` in 21 packages
  (alias retained); `Adapter.String()` unknown fallback unified to
  `"unknown"`; option keys unified to snake_case across all services
  (migration table extended); `ParseOptions` removed from lock/secrets
  (raw maps enter only via strict file decode + env); `Register`/`Open`/
  `ParseAdapter`/`Factory` doc first-lines unified;
  `Validate` doc first line unified to
  "checks options for consistency, joining all violations";
  mdx/CLI docs switched to canonical accessors; `check:boundaries` help
  made explicit; `new_gomod.golden` go 1.27; AGENTS probe chain and
  container no-op notes updated.

- Consistency pass, round four (breaking where noted): `ErrDuplicate` /
  `DuplicateError` mirror aliases added in all 13 `ErrDuplicateAdapter` /
  `DuplicateAdapterError` packages so both spellings resolve everywhere;
  tags completed for s3opts/cookie/revocation; `Factory`/`Open`/
  `ParseAdapter` doc first-lines unified; `AI` methods documented;
  `breaking` added to CLI suggestion list; dashboard db-screen CLIs fixed;
  cli.mdx `config` row added; golden go 1.27; example README prefixes and
  bookings DSN fixed;   test doubles unified to `stub*` with helper roles
  (`stub`/`must`/`freshAdapter`) codified in AGENTS.md; `time.Sleep` sync
  waits converted to event-based waiting across 48 test files (lrucache
  gains unexported clock injection; remaining sleeps are poll ticks or
  sanctioned teardown).

- Consistency pass, round six: mirror `ErrDuplicateAdapter` aliases in
  17 bare-`ErrDuplicate` packages; forgotten-import hint on all
  `UnknownAdapterError`s; `Reason` strings snake_case + lowercase;
  webhook wrong-service wraps fixed; `config.Validate` covers all 34
  (nil skips removed); backend registry + gogen file-count docs fixed;
  storage gains `ErrInvalidOptions`/`InvalidOptionsError`; dedicated CLI
  help for routes/explain/graph/check-boundaries; lsp pins to v0.2.0.

- Consistency pass, round five: mirror aliases both directions
  (`ErrDuplicate` + `DuplicateError` in all 13 canonical packages);
  tags completed (s3opts, cookie, revocation); `breaking` in CLI
  suggestions; dashboard db-screen CLIs; cli.mdx `config` row; golden go
  1.27; example README fixes; qdrant fakes to `stub*`; serialization-tag
  rule codified (Options + wire types tagged, Go-only structs untagged,
  backends map storage keys explicitly).

- Consistency pass, round six: receivers unified (`a` adapter, `l` logger,
  `s` store/geo, `c` client); webhook `Open` validate error direct;
  `ConnMax*` → `MaxConn*` prefix order; named duration consts per package;
  `t.Context()` in all tests (`b.Context()` in benchmarks, t-taking
  helpers elsewhere); test-style + time rules codified in AGENTS.md.

- Consistency pass, round seven: every Redis-backed `Open` validates options
  first (fail fast before dialing); webhook `Open` validate errors returned
  directly with named duration consts; receiver/acronym/adapter-name sweep
  completed; `ErrDuplicate` mirror aliases completed in the remaining
  packages; scaffold/docs versions aligned to framework `v0.2.0` (go `1.27`,
  lsp `0.2.0`, CITATION `0.2.0`); remaining `time.Sleep` test sync converted
  to event-based waiting.


## [v0.2.0] - 2026-09-23

### Added

- `examples/showcase`: full-capability shop app exercising every DSL
  feature in one schema (all scalars, named + inline enums, `?`, all four
  relations, `@schema`, `@renamed_from`, messages, all verbs/auth shapes,
  `errors:`, `paginated:`, job params, schedules) with generated output for
  all six backends, runnable server/worker/seed, and tests.
- `examples/transactions`, `examples/bulk_upsert`, `examples/pagination`,
  `examples/recursive_cte`, `examples/json_query`, `examples/fts`: focused
  ORM ports (nested transactions, upserts with RETURNING, offset + keyset
  pagination, recursive CTEs, JSON1 queries, FTS5 search), each with schema,
  runnable demo, committed `zenorm` output, and deterministic tests.
- `examples/ormdrill`: no-codegen raw-`orm` drill as per-topic `Example`
  tests (cursors, preload, joins, subqueries, expressions, mutation gates).
- `examples/demoapp`: batteries-included HTTP showcase (register/login,
  products/orders/posts, per-battery `/demo/*` endpoints, jobs + schedules)
  with black-box API tests; `demoapp` is battery breadth, `showcase` is
  DSL depth.
- `examples/todo`: cross-transport ownership check — non-owner Delete now
  returns 403/`PermissionDenied` identically over HTTP and gRPC; notes and
  digest job converted from raw SQL to the typed `orm` builder.

### Fixed

- `examples/showcase/data/cities.json` used `city` keys while `geo/static`
  expects `name`, so geocoding silently matched nothing.
- `zenorm` backend emitted references to named-enum Go types (e.g. `Role`)
  without ever declaring them, so any schema using `enum Name {}` failed to
  compile downstream. The backend now emits one string-kind type plus typed
  constants per referenced enum, resolved through a schema-wide index so
  cross-module references work too.

## [v0.1.2] - 2026-09-21

- docs: add Astro Starlight site in docs/ with Laravel/RoR-structured guides deployed to GitHub Pages.

### Changed

- `zever new` now defaults to `require github.com/zenta-dev/zever v0.1.1` (upstream)
  when run outside a framework checkout; previously it emitted a local
  `replace => .` with pseudo-version. Use `--framework-path` for local development.

## [v0.1.1] - 2026-09-20

### Added

- `tools/zever-lsp` is now an independently versioned Go module: it requires the published `zever` release instead of a local `replace` directive, and is tagged `tools/zever-lsp/v0.1.0`. Install with `go install github.com/zenta-dev/zever/tools/zever-lsp@v0.1.0`.

### Fixed

- `orm`'s streaming peak-memory test (`TestQueryStreamPeakMemoryFlatVersusAll`) measured the live heap with a background goroutine forcing a GC every 500µs. Under CI CPU contention the sampler stalled, letting transient per-row garbage inflate readings by megabytes and fail the flatness assertion. Sampling is now driven synchronously by row count (every 1000 rows, plus a `KeepAlive`-held single sample for `All`), so the measurement no longer depends on goroutine scheduling. Test-only change; streaming behavior is unchanged.

## [v0.1.0] - 2026-09-20

### Added

- Initial project scaffolding: Go module, CI, lint configuration, Makefile, and
  open source project files.
- Generic `codec` package: `Encoder[V]` / `Decoder[V]` / `Codec[V]` interfaces
  with a `JSONCodec[V]` implementation backed by `encoding/json/v2` and
  sentinel `ErrEncode` / `ErrDecode` errors.
- Generic `log` package: `Logger` / `Event` / `Context` facade with a
  registration-based adapter system and `noop`, stdlib `slog`, `zerolog`,
  and human-readable `pretty` backends.
- Generic `cache` package: `Cache` facade with `Typed[K, V]` codec helpers and
  in-memory LRU plus Redis-backed adapters.
- Generic `queue` package: `Queue` facade with in-memory and Redis-backed
  adapters for topic-based messaging.
- Generic `job` package: job-queue orchestration with typed registration,
  dispatch (delay, scheduling, uniqueness), worker execution with retry and
  dead-letter handling, batch progress, and cron scheduling.
- Generic `storage` package: `Storage` facade for presigned uploads and
  downloads with local filesystem, S3, and R2 backends plus bucket policies.
- Generic `observability` package: telemetry facade with `noop`, `stdout`,
  and OTLP adapters for traces and metrics.
- Generic `permission` package: fail-closed authorization facade with
  `noop`, RBAC, and Casbin adapters.
- Generic `eventbus` package: publish/subscribe facade with in-memory
  fan-out and Redis PubSub adapters.
- Generic `mailer` package: mail facade with JSON log and SMTP adapters.
- Generic `idempotency` package: reserve-then-complete execution store
  with in-memory and Redis backends.
- Generic `scheduler` package: cron `Scheduler` facade with an in-process
  embedded adapter dispatching registered jobs.
- Generic `ratelimit` package: token-bucket limiter with in-memory
  and Redis (Lua) backends.
- `eventbus` gains a pull API: `SubscribeChan` returns a buffered Go
  channel per subscriber (slow subscribers drop newest, others
  unaffected), `Unsubscribe` detaches and closes it with
  `ErrNotSubscribed` for unknown topics, and `Message.ReceivedAt`
  records the publish timestamp. The Redis wire envelope
  (`id`/`payload`/`headers`) is unchanged.
- Generic `notification` package: notifier facade with JSON log,
  Twilio SMS, and FCM push adapters.
- Generic `i18n` package: internationalization facade with embedded
  catalog and remote service adapters.
- Generic `flag` package: feature-flag facade with static file-backed
  and Firebase Remote Config adapters.
- Generic `session` package: server-side session store facade with
  in-memory and Redis adapters.
- Generic `auth` package: authentication facade with HS256 JWT, OIDC,
  and session-backed adapters.
- Generic `authz` package: authorization bridge wiring `auth` verification
  to `permission` decisions with HTTP middleware and a gRPC interceptor.
- Generic `analytics` package: event-tracking facade with JSON-lines log
  and PostHog adapters.
- Generic `payment` package: payment-processing facade with in-memory
  stub, Stripe, and Paddle backends.
- Generic `billing` package: subscription and invoice facade with
  in-memory stub, Stripe, and Paddle backends.
- Generic `document` package: document-rendering facade with local
  headless-Chrome, remote, and LaTeX backends.
- Generic `media` package: media-asset facade with local filesystem
  and S3 backends plus an ffmpeg probe/transform helper.
- Generic `tenant` package: multi-tenant resolution facade with
  single fixed-ID and header-based backends.
- Generic `search` package: full-text search facade with SQLite,
  Postgres, and Meilisearch backends.
- Generic `vectorstore` package: vector-similarity facade with SQLite,
  pgvector, and Qdrant backends.
- Generic `webhook` package: webhook-delivery facade with HTTPS,
  queue fan-out, and durable SQLite backends.
- Generic `workflow` package: run-lifecycle facade with an in-memory
  step engine.
- Generic `password` package: password-hashing facade with an Argon2id
  backend.
- Generic `crypto` package: encryption and signing facade with a local
  AES-256-GCM backend.
- Generic `ai` package: LLM facade with Anthropic, OpenAI, Gemini,
  and local Ollama backends.
- Generic `geo` package: geocoding facade with Google Maps, static
  JSON, and OSM Nominatim backends.
- Generic `router` package: HTTP router facade with Fiber and
  stdhttp backends.
- Generic `apperror` package: typed error vocabulary with gRPC codes
  and HTTP mappings.
- Generic `db` package: SQL database facade with SQLite and
  Postgres backends.
- Generic `middleware` package: HTTP middleware and gRPC interceptors
  for logging, recovery, rate limiting, and tracing.
- Generic `config` package: layered service configuration with
  defaults, files, environment, and redaction.
- Generic `container` package: lazy service container with ordered
  shutdown.
- Generic `orm` package: typed SQL query builder with dialect-gated
  rendering for Postgres and SQLite.
- Internal `dsl` schema compiler frontend: lexer, parser, resolver, IR,
  formatter, breaking-change checker, and deterministic compile harness
  with golden fixtures.
- `zever-lsp` tool: stdio LSP server for the schema DSL (separate
  `tools/zever-lsp` module).
- Generic `lock` package: distributed lease facade with in-memory
  and Redis backends.
- Generic `secrets` package: secret-management facade with an
  environment-variable backend.
- `zen` migration engine: live-schema-diff Plan/Apply with rollback
  for Postgres, SQLite, and MySQL.
- Internal `opts` helpers: typed readers for adapter option maps.
- `zever` CLI (`cmd/zever`): TUI-first toolkit shell with dashboard
  and schema/scaffold commands.
- Editor support for the schema DSL: Neovim plugin (`editors/nvim`)
  with filetype detection and syntax highlighting.
- Editor support for the schema DSL: VS Code extension
  (`editors/vscode`) with syntax highlighting and LSP client.
- Internal `dsl/gengrammar` generator (run via `make generate`,
  `tools/gengrammar`): derives the Neovim and VS Code grammar files'
  keyword/boolean/scalar-type word lists from `token.Keywords` and
  `resolver.ScalarTypeNames` instead of three independently
  hand-maintained copies; `editors/parity_test.go` now diffs the
  generator's output against the checked-in grammar files.
- `zever config show`: prints the resolved, redacted service configuration,
  with a matching TUI dashboard screen.
- `middleware.Timeout` / `middleware.TimeoutUnaryServerInterceptor`: bound a
  handler's execution to a fixed duration, responding with the fixed error
  envelope (504 HTTP, `codes.DeadlineExceeded` gRPC) if it hasn't finished
  in time. The handler's context carries the deadline, so downstream calls
  using it are canceled too.

### Changed

- **Breaking:** `billing.Billing.CreateCustomer` and `CreateSubscription`
  gain a required `idempotencyKey string` parameter. Neither method
  previously had any way to prevent a client-side retry (timeout,
  transient network error) from creating a second, separately-billed
  subscription or a duplicate customer -- `payment/stripe` already
  supported this via `Meta["idempotency_key"]`, but `billing/stripe` had
  no equivalent. Pass `""` for the previous (non-idempotent) behavior.
  `billing/stripe` wires it to Stripe's `Idempotency-Key` header;
  `billing/paddle` and `billing/stub` accept and ignore it (no equivalent
  mechanism). Any external implementation of `billing.Billing` must add
  the new parameter.
- `authz.Authorize` no longer asserts `Policy.Roles` for an unauthenticated
  caller: `AuthRequired: false` skipped token verification but the
  permission check still ran with the policy's static `Roles` list, so a
  `Policy` meant as "public route, still gated" (`AuthRequired: false`,
  `PermissionCheck` set, `Roles` non-empty) silently granted every anonymous
  caller those roles. Roles now only apply when `AuthRequired` is true.
- `scheduler.NewEmbedded` moved to `scheduler/embedded.New`; register
  via `schedulerembedded.New`. No behavior change.

- `job.Scheduler.Every` now returns the cron entry ID
  (`(job.EntryID, error)`); use `Remove`/`Entries` to manage schedules.
  A nil `Locker` means single-instance mode without slot locks.
- `eventbus` memory `Close` abandons in-flight handlers on timeout and
  returns nil instead of `DeadlineExceeded`, matching the redis adapter.
- `internal/retry`'s jitter reseeded a fresh `math/rand` source from
  `time.Now().UnixNano()` on every call; concurrent callers landing in the
  same nanosecond could get identical seeds and therefore identical
  "random" delays, defeating the point of jitter. Now uses `math/rand/v2`'s
  concurrency-safe top-level functions.
- **Breaking:** the live-schema-diff migration engine moved from
  `zen/migrate` to `orm/migrate` (import path
  `github.com/zenta-dev/zever/orm/migrate`), matching this repo's `orm/`
  query builder package name; `zen/` no longer exists. Update imports
  accordingly.
- DSL codegen backends retargeted from `github.com/zenta-dev/zen-go`
  to `github.com/zenta-dev/zever`: the proto backend's `go_package`
  options, the `zengo/annotations.proto` companion file, protogogen's
  generated Go imports, the atlas header, and the OpenAPI title
  (`zever API`).
- **Breaking:** `vectorstore.VectorStore` gains a required
  `UpsertBatch(ctx, []Vector) error` method, and `search.Search` gains a
  required `IndexBatch(ctx, []Document) error` method, so callers indexing
  many documents/vectors no longer need N separate round-trips. All
  adapters implement the new methods: `pgvector` and `search/postgres` use
  multi-row `INSERT ... ON CONFLICT` statements chunked to stay under
  Postgres's bind-parameter limit, `qdrant` uses its native batch `Upsert`
  points API, `search/meilisearch` uses its native bulk `AddDocuments` API
  grouped by index, and `vectorstore/sqlite` and `search/sqlite` wrap the
  existing single-item logic in one transaction. Any external
  implementation of `VectorStore` or `Search` must add the new method.
- `internal/redis` no longer holds a shared package-level client singleton:
  every `New` call returns an independently owned `*goredis.Client`, and
  `Close` now takes the client to close. Previously `cache/redis` and
  `queue/redis` (and other redis-backed adapters) reused or replaced one
  global client keyed by connection options, so constructing a second
  adapter with different options would silently close the first adapter's
  live connection, and closing either adapter tore down the shared client
  for all of them. `cache/redis`, `queue/redis`, `idempotency/redis`,
  `ratelimit/redis`, `session/redis`, `eventbus/redis`,
  `auth/jwt/revocation/redis`, and `lock/redis` now each own and close an
  independent client.
- Enable the `exhaustive` linter for switches over closed enum types (e.g.
  `internal/dsl/ir.ScalarType`/`ErrorCode`), so a missing case on a future
  enum member is caught at lint time instead of at runtime.
- Enable `contextcheck`, `sqlclosecheck`, `rowserrcheck`, `zerologlint`,
  and `spancheck` linters, matching this repo's use of `database/sql`/pgx,
  zerolog, and OpenTelemetry spans; `contextcheck` also runs on test files.
  Related fallout: `tenant/header` documents its required trust boundary
  (deploy only behind a gateway that authenticates callers and owns the
  tenant header).
- `queue/redis`'s poll loop throttles its due/stale message sweep
  (`promoteDue`/`reclaimStale`) to once per 250ms instead of running both on
  every poll tick (~every 100ms), cutting idle Redis round trips at the cost
  of up to 250ms of added latency before a due or stale message is
  recovered.
- `router/fiber`'s `ServeHTTP` and route handlers built a fresh
  `net/http` adaptor closure on every request instead of once at
  construction/route-registration time; both are now hoisted, cutting two
  redundant closure allocations per request.
- `geo/osm` had no rate limiting against Nominatim's usage policy (max 1
  request/second for unauthenticated use); a caller geocoding in a loop
  could get the application's IP blocked. `New` now paces requests to at
  most 1/sec by default.

### Fixed

- `internal/dsl/naming`'s snake_case conversion inserted an underscore
  before every capital letter, mangling acronyms in generated identifiers
  (`APIKey` -> `a_p_i_key` instead of `api_key`). Acronym runs are now
  treated as a single word, matching protobuf's own style guide. Affects
  generated Postgres/SQLite table names, Go/zenorm file and type names, and
  proto enum value prefixes for any schema using an acronym in an
  entity/field/enum name.
- `container.Container.Close` now flushes the `observability` service on
  shutdown: `closeAny` probes for a fourth shutdown shape,
  `Shutdown(context.Context) error`, which `observability.Provider` (and its
  `Tracer`/`Metrics` sub-interfaces) implement instead of `Close`/`Stop`.
  Previously `closeAny` silently no-oped for observability, so `otlp`'s
  buffered spans and metrics were never flushed on `Container.Close`.
- `vectorstore/pgvector`'s `UpsertBatch` and `search/postgres`'s
  `IndexBatch` built one unbounded multi-row `INSERT` sized to the full
  input slice, which could exceed Postgres's 65535 bind-parameter limit on
  large batches and fail with an opaque driver error. Both now chunk the
  input into multiple sequential `INSERT` statements (at most 20000 rows
  per statement for `pgvector`, 15000 for `search/postgres`), still far
  fewer round trips than one-by-one calls.
- `permission/rbac`'s owned-only check incorrectly allowed a request through
  when the subject had an empty ID and the resource was missing (or had an
  empty) owner attribute, since both sides of the comparison were the empty
  string. The check now explicitly rejects an empty subject ID.
- `vectorstore/qdrant`'s `ensureCollection` no longer lets every concurrent
  caller during cold start race its own `CollectionExists`/
  `CreateCollection` round trip: the first caller now leads while others
  wait, and a failed attempt is still retried by the next caller.
- `ai/gemini` adapter's `Stream` sent chunks to its output channel with a
  bare `ch <- ...` at every yield point (text delta, tool call, finish
  reason, usage, and the final done marker). If the caller cancelled its
  context and stopped draining the channel — the expected behavior on
  cancellation — the goroutine blocked forever on the next send, leaking
  it along with the underlying SSE response. Every send now uses
  `select { case ch <- ...: case <-ctx.Done(): return }`, matching the
  guard already used by the `ai/anthropic` and `ai/openai` adapters.
- `ai/openai`'s `Stream` never closed the underlying SSE stream on any
  exit path (early return on context cancellation, error, or normal
  completion), leaking the HTTP response body every time a caller aborted
  generation or a stream errored mid-flight.
- `ai/anthropic`'s `Stream` silently degraded to full-buffering: it called
  the synchronous `Generate` and trickled the already-complete response out
  as one chunk, unlike `ai/openai`/`ai/gemini`/`ai/ollama`, which all
  stream incrementally. Now uses the SDK's real SSE streaming
  (`Messages.NewStreaming`) with its official event accumulator, so
  content arrives token-by-token as the model generates it.
- `document/latex` now compiles with `-no-shell-escape`, disabling LaTeX's
  `\write18` arbitrary-shell-command primitive regardless of the host's
  `texmf.cnf` defaults. Source is caller-supplied LaTeX (e.g. an invoice
  template with interpolated data), which had no legitimate need for
  shell-escape-dependent packages.
- `notification/fcm`'s `Notify` now retries a transient send failure
  (network blip, FCM 5xx) up to 3 times with short backoff, matching every
  other outbound-network adapter in the codebase
  (`webhook/http`/`webhook/queue`/`ai/anthropic`/`ai/openai`/`queue/redis`).
  A duplicate push notification on a spurious retry is a low-severity
  nuisance, so this is safe unconditionally.
- `geo/osm`'s request pacer anchored the next slot to the intended rather
  than the actual send time, so a timer wakeup that arrived late under load
  (or any scheduler delay between the wakeup and the request hitting the
  wire) shrank subsequent gaps below `minInterval`, risking violation of
  Nominatim's 1-request-per-second usage policy. The schedule is now
  re-anchored to the observed request time after each round trip.
