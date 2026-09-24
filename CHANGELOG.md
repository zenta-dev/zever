# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

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
