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
  registration-based adapter system and `noop`, stdlib `slog`, and `zerolog`
  backends.
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
- Generic `ai` package: LLM facade with Anthropic, OpenAI, and Gemini
  backends.
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

### Changed

- `job.Scheduler.Every` now returns the cron entry ID
  (`(job.EntryID, error)`); use `Remove`/`Entries` to manage schedules.
  A nil `Locker` means single-instance mode without slot locks.
- `eventbus` memory `Close` abandons in-flight handlers on timeout and
  returns nil instead of `DeadlineExceeded`, matching the redis adapter.
