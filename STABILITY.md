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
- **Where adapters live:** each battery is one small interface
  in `core/<b>` plus adapters in `adapters/<b>/<a>` with explicit
  `Register()` calls (for example
  `adapters/cache/memory`, `adapters/cache/redis`). The container is the
  sole resolver and wires nothing by itself
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
| `dsl` | `.zen` compiler frontend (compile-time only, no release version; schema `v1`/`v2` are API versions). |

## Extended

Everything not in Core, including all remaining configured services plus
framework helpers, middleware, and tooling. `0.x` may break freely with
release-note docs.

| Package | Notes |
|---|---|
| `ai` | Facade (`anthropic`, `openai`, `gemini`, `ollama`); no zero-infra default. |
| `agent` | Tool-calling loop over `ai`; no adapter registry, no `config` entry; `container.Agent()`. |
| `rag` | Retrieval-augmented generation over `ai` + `vectorstore`; no `config` entry; `container.RAG()`. |
| `billing` | Facade (`stub`, `stripe`, `paddle`). |
| `payment` | Facade (`stub`, `stripe`, `paddle`). |
| `storage` | Facade (`local`, `s3`, `r2`). |
| `media` | Facade (`local`, `s3`). |
| `document` | Facade (`local`, `remote`, `latex`). |
| `search` | Facade (`db`, `postgres`, `meilisearch`, `sqlite`). |
| `vectorstore` | Facade (`db`, `sqlite`, `pgvector`, `qdrant`). |
| `notification` | Facade (`log`, `twilio`, `fcm`). |
| `geo` | Facade (`google`, `static`, `osm`). |
| `flag` | Facade (`static`, `firebase`). |
| `observability` | Facade (`noop`, `stdout`, `otlp`); `Shutdown(ctx)` flush path. |
| `eventbus` | Facade (`memory`, `redis`). |
| `eval` | Dataset/scorer harness for agent and generation outputs. |
| `workflow` | Facade (`memory`, `db`). |
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
| `scheduler` | Facade (`embedded`); shares `Queue` via `Job()`. |
| `webhook` | Facade (`http`, `queue`, `sqlite`). |
| `apperror` | Typed error vocabulary (gRPC/HTTP mappings). |
| `authz` | `auth`-to-`permission` bridge (HTTP middleware, gRPC interceptor). |
| `codec` | Generic `Encoder`/`Decoder`/`Codec` + `JSONCodec`. |
| `mcpclient` | MCP stdio client (initialize, tools/list, tools/call). |
| `prompt` | Prompt builders (system, grounding, JSON repair, tool errors). |
| `middleware` | HTTP middleware + gRPC interceptors. |
| `cmd/zever` | Flags-only CLI toolkit; see `cmd/zever/README.md`. |
| `tools/zever-mcp` | Stdio MCP server exposing the schema compiler to agents; nested module. |

## 1.0 promotion criteria

Core graduates from "toward 1.0 promise" to the 1.0 compatibility promise
only when all of the following hold. Until then the pre-1.0 policy above
applies unchanged.

- **6 months without a core break:** no breaking change to any `Core`
  package below in the 6 months before the 1.0 tag, measured from
  `CHANGELOG.md` release-note entries.
- **100% core kits:** every `Core` battery with an adapter registry ships
  a conformance kit (`<b>test` package), and every shipped adapter runs
  it green in CI — including the redis suites (`adapters/cache/redis`,
  `adapters/queue/redis`).
- **External production user:** at least one application outside this
  repo runs a tagged release in production and its deployment is
  referenced from `docs/production.md`.

## Current standing (honest)

- Core breaks: still permitted with release notes; the 6-month clock has
  not started.
- Kits: 38 conformance kit packages ship under `core/*/*test/`; 32 of
  them ship an in-kit `conformance_test.go` suite (all pass). CI
  (`.github/workflows/ci.yml` `conformance` job) runs all 32 in-kit
  suites plus all 83 cross-adapter `-run Conformance` invocations
  (representative batteries: `ai`, `analytics`, `auth`, `authz`,
  `billing`, `cache`, `crypto`, `db`, `document`, `eventbus`, `flag`,
  `geo`, `i18n`, `idempotency`, `job`, `lock`, `log`, `mailer`,
  `media`, `middleware`, `notification`, `observability`, `password`,
  `payment`, `permission`, `queue`, `ratelimit`, `router`, `scheduler`,
  `secrets`, `session`, `storage`, `tenant`, `webhook`, `workflow`).
- Scheduler: `adapters/scheduler/postgres` is a working durable/leased
  second adapter; `embedded` remains the zero-infra default.
- External prod user: none known.
- Supply chain: SBOM per affected module in CI
  (`.github/workflows/ci.yml` `sbom` job); SLSA provenance attested on
  SBOMs in the same job, verifiable via
  `gh attestation verify sbom/<slug>.json --repo zenta-dev/zever`.
  Tagged releases additionally ship static binaries + SBOMs, attested
  and attached via `.github/workflows/release.yml`; verify with
  `gh attestation verify dist/zever-linux-amd64 --repo zenta-dev/zever`.

## Coverage note

Every top-level package is classified above. `config`/`container` cover
34 services (`ai`, `analytics`, `auth`, `billing`, `cache`, `crypto`,
`db`, `document`, `eventbus`, `flag`, `geo`, `i18n`, `idempotency`,
`lock`, `log`, `mailer`, `media`, `notification`, `observability`,
`password`, `payment`, `permission`, `queue`, `ratelimit`, `router`,
`scheduler`, `search`, `secrets`, `session`, `storage`, `tenant`,
`vectorstore`, `webhook`, `workflow`); `job` rides on `Queue` and `grpc`
is a lazy `*grpc.Server` singleton, so neither has a `config` entry.
