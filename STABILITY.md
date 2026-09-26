# Stability tiers

Pre-1.0 policy for what may break in `0.x` releases. Docs-only; no API change.

See also [Versioning](.github/CONTRIBUTING.md#versioning) and
[API Compatibility](.github/CONTRIBUTING.md#api-compatibility).
Cross-checked against `config.Config` (`config/config.go`, 34 `Service`
fields) and `container` accessors (`container/services.go`,
`container/README.md`).

## Policy

- **Core (toward 1.0 promise):** breaking changes are minimized, called out
  explicitly in the PR and release notes, and need maintainer review.
  Goal is a stable foundation for 1.0. Pre-1.0 `0.x` may still break core
  with documented release notes, but the bar is high.
- **Extended (0.x breaks freely):** breaking changes are expected during
  pre-1.0 development. They are still documented in release notes
  (`CHANGELOG.md` `## [Unreleased]` before PR), but no stability promise
  is made before 1.0.
- **Where adapters live:** each top-level package is one small interface
  plus self-registering adapters in subpackages (for example
  `cache/memory`, `cache/redis`). The container is the sole resolver
  (`container.New(cfg)` plus lazy per-service accessors). Do not add
  methods to public interfaces (breaks implementers); prefer a new
  interface.

## Core

| Package | Notes |
|---|---|
| `config` | Layered typed config (`defaults < file < env`); `config/README.md`. |
| `container` | Lazy resolver + ordered `Close`; `container/README.md`. |
| `db` | SQL facade (`sqlite`, `postgres`). |
| `orm` | Typed builder over `db.DB`; dialect-gated, fails closed. |
| `router` | HTTP facade (`fiber`, `stdhttp`). |
| `cache` | Facade (`memory`, `redis`); leaf dependency, closed last. |
| `queue` | Facade (`memory`, `redis`); leaf dependency, closed last. |
| `session` | Facade (`memory`, `redis`). |
| `auth` | Facade (`jwt`, `session`, `oidc`). |
| `crypto` | Facade (`local`); dev key deterministic, never prod. |
| `password` | Facade (`argon2id`). |
| `secrets` | Facade (`env` only); `ZEVER`-prefixed env exception. |
| `internal/dsl` | `.zen` compiler frontend (internal-only, no release version; schema `v1`/`v2` are API versions). |

## Extended

Everything not in Core, including all remaining configured services plus
framework helpers, middleware, and tooling. `0.x` may break freely with
release-note docs.

| Package | Notes |
|---|---|
| `ai` | Facade (`anthropic`, `openai`, `gemini`, `ollama`); no zero-infra default. |
| `billing` | Facade (`stub`, `stripe`, `paddle`). |
| `payment` | Facade (`stub`, `stripe`, `paddle`). |
| `storage` | Facade (`local`, `s3`, `r2`). |
| `media` | Facade (`local`, `s3`). |
| `document` | Facade (`local`, `remote`, `latex`). |
| `search` | Facade (`postgres`, `meilisearch`, `sqlite`). |
| `vectorstore` | Facade (`sqlite`, `pgvector`, `qdrant`). |
| `notification` | Facade (`log`, `twilio`, `fcm`). |
| `geo` | Facade (`google`, `static`, `osm`). |
| `flag` | Facade (`static`, `firebase`). |
| `observability` | Facade (`noop`, `stdout`, `otlp`); `Shutdown(ctx)` flush path. |
| `eventbus` | Facade (`memory`, `redis`). |
| `workflow` | Facade (`memory`). |
| `tenant` | Facade (`single`, `header`). |
| `analytics` | Facade (`log`, `posthog`). |
| `i18n` | Facade (`embed`, `remote`). |
| `idempotency` | Facade (`memory`, `redis`). |
| `job` | Dispatcher over resolved `Queue`; no adapter registry, no `config` entry. |
| `lock` | Facade (`memory`, `redis`). |
| `log` | Facade (`noop`, `zerolog`, `slog`, `pretty`). |
| `mailer` | Facade (`log`, `smtp`). |
| `permission` | Facade (`noop`, `rbac`, `casbin`). |
| `ratelimit` | Facade (`memory`, `redis`). |
| `scheduler` | Facade (`embedded`); shares `Cache`/`Queue` via `Job()`. |
| `webhook` | Facade (`http`, `queue`, `sqlite`). |
| `apperror` | Typed error vocabulary (gRPC/HTTP mappings). |
| `authz` | `auth`-to-`permission` bridge (HTTP middleware, gRPC interceptor). |
| `codec` | Generic `Encoder`/`Decoder`/`Codec` + `JSONCodec`. |
| `middleware` | HTTP middleware + gRPC interceptors. |
| `cmd/zever` | Flags-only CLI toolkit; see `cmd/zever/README.md`. |

## Coverage note

Every top-level package is classified above. `config`/`container` cover
34 services (`ai`, `analytics`, `auth`, `billing`, `cache`, `crypto`,
`db`, `document`, `eventbus`, `flag`, `geo`, `i18n`, `idempotency`,
`lock`, `log`, `mailer`, `media`, `notification`, `observability`,
`password`, `payment`, `permission`, `queue`, `ratelimit`, `router`,
`scheduler`, `search`, `secrets`, `session`, `storage`, `tenant`,
`vectorstore`, `webhook`, `workflow`); `job` rides on `Queue` and `grpc`
is a lazy `*grpc.Server` singleton, so neither has a `config` entry.
