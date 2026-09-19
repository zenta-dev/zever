# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
