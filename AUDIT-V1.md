# V1 Readiness Audit

Post-merge `main` (this track continues on `feat/v1-release-docs`).
Docs + `release.yml` only; no code changes. Cites are
`file:line` against this worktree.

## 1. Per-battery conformance-kit status

Fifteen kits ship in-tree (`core/*/*test/conformance.go`: `billing`,
`cache`, `crypto`, `eventbus`, `flag`, `idempotency`, `lock`, `log`,
`observability`, `password`, `payment`, `queue`, `ratelimit`,
`secrets`, `storage`); thirteen of them ship an in-kit
`conformance_test.go` suite (all except `billing` and `password`,
which ship the kit without a suite). The plugin guide names three:

- `core/cache/cachetest/conformance.go` — kit + `conformance_test.go`
  in-kit suite.
- `core/queue/queuetest/conformance.go` — kit + `conformance_test.go`.
- `core/storage/storagetest/conformance.go` — kit + `conformance_test.go`.
- `core/secrets/secretstest/conformance.go` — kit exists
  (`conformance.go:1` "conformance kit third-party secrets adapters run
  to prove backend parity") but `docs/writing-a-plugin.md:6-10` still
  says "Three batteries ship kits today" (cache, queue, storage).
  Doc drift: either list secrets as the fourth kit or state why it is
  excluded from the acceptance gate.

CI wires kits in `.github/workflows/ci.yml:164-190` (`conformance` job):
in-kit suites for cache/queue/storage (`ci.yml:182-186`) plus
`-run Conformance` invocations against both redis adapters
(`ci.yml:188-190`).

Redis cross-adapter conformance is currently skipped with reasons, not
passing:

- `adapters/cache/redis/conformance_cover_test.go:29-30` —
  `t.Skip("miniredis clock is frozen without FastForward ...")`.
- `adapters/queue/redis/conformance_cover_test.go:19-29` —
  `t.Skip("miniredis Lua cjson mangles empty headers ...")`.

So: 3 of 15 kits run green in CI; redis adapter parity is
wired-but-skipped pending miniredis fidelity. The 12 remaining kits
(`billing`, `crypto`, `eventbus`, `flag`, `idempotency`, `lock`, `log`,
`observability`, `password`, `payment`, `ratelimit`, `secrets`) run only
via their in-kit suites (13 ship one; `billing`/`password` ship the kit
without a suite) and per-adapter tests. Every other battery
(~30) has no shared kit; new adapters there prove parity only via
per-adapter tests.

## 2. Single-adapter list + second-adapter verdict

Exactly two batteries with an adapter registry ship one adapter:

| Battery | Adapter | Verdict |
|---|---|---|
| `password` | `argon2` (`adapters/password/argon2`) | By design. Password hashing is pure CPU work with no operational axis (no network, no persistence, no multi-instance coordination); `core/password/doc.go:12` already says "hashing is pure CPU work". A second adapter (bcrypt/scrypt) is possible but adds no deployment shape — proposal text only, do not build. |
| `scheduler` | `embedded` (`adapters/scheduler/embedded`) | Second adapter genuinely needed before 1.0 for multi-instance deploys. `embedded` is in-process cron (`core/scheduler/doc.go:1-2`); two replicas double-fire every slot. A durable/leased scheduler (postgres-backed, mirroring `adapters/workflow/postgres`) is the honest second adapter. Proposal text only in this track. |

Zero-registry packages are by design, not single-adapter gaps:

- `core/authz` — bridge (`auth`-to-`permission`), no `Open`/`Register`
  surface (`core/authz/doc.go:8-9`).
- `core/job` — dispatcher over resolved `Queue`, no adapter name, no
  config section (`core/job/doc.go:13-19`).
- `core/middleware` — constructors over resolved instances, no registry
  (`core/middleware/doc.go:1-10`, `core/middleware/README.md:1-5`).

`db` does NOT need a mysql adapter: `orm/migrate/mysql.go` already
implements the MySQL live-diff half (introspection, backtick quoting,
`MODIFY COLUMN` renderers) and `orm/migrate/doc.go:22` lists
`"sqlite"` and `"mysql"` as supported migrate dialects. The `db`
battery stays `sqlite`/`postgres` (`adapters/db/postgres`,
`adapters/db/sqlite`); MySQL support lives at the migrate layer.

## 3. DSL hook status (greenfield)

No `.zen` plugin hook exists, and that is a deliberate scope boundary,
not a gap: `docs/writing-a-plugin.md:171-184` ("`.zen` schema boundary")
states schemas cannot reference plugin batteries; the compiler resolves
only core batteries at compile time while plugins wire at runtime via
`config.Plugins` + `container.RegisterPlugin`/`Resolve`. Any DSL hook
for plugins is greenfield design work (schema syntax, resolver support,
codegen for `RegisterPlugin` wiring) with no existing partial
implementation to extend.

## 4. Bench files + published output

Four `*_bench_test.go` files exist (untouched per track scope):

- `core/middleware/middleware_bench_test.go` (chained
  RequestLogger+Tracing overhead).
- `adapters/router/fiber/fiber_bench_test.go` (Handle+ServeHTTP round
  trip).
- `container/container_bench_test.go`.
- `orm/preload_bench_test.go`.

Baseline output is now published in `docs/benchmarks.md` (Results
2026-10-02: router sequential vs parallel `ServeHTTP`, ORM sqlite
`:memory:` Preload below/above chunk size plus concurrent, queue
memory Push/Pop/Ack round trip — each with ns/op, B/op, allocs/op at
cpu=1,4 — plus repro commands, machine spec, and a staleness warning
that numbers are a point-in-time snapshot, not a deploy gate).
`.github/CONTRIBUTING.md:155-164` requires benchmark evidence only for
PRs that claim a performance change (`make bench`); numbers appear
inline in PR bodies and are reproducible from the committed commands.
Any 1.0 perf promise still needs a freshness policy first (committed
numbers older than a few weeks are stale by the file's own warning).

## 5. Auth-group review depth vs crypto group

Both groups sit under the same hard gate, so "crypto reviewed deeper"
is not accurate as a process claim:

- `tools/coverage-floor.sh:2-3` scopes the floor to
  `auth/crypto/payment`; `.github/workflows/ci.yml:293-318`
  (`coverage-floor` job) re-runs race+coverage on
  `core/auth core/crypto core/password core/authz core/payment
  adapters/payment/stripe adapters/payment/paddle` (`ci.yml:316`) and
  fails main/tags below 80% (`tools/coverage-floor.sh:44`,
  `ci.yml:318`; header notes stripe measured 83.0 so 85 would ship red).
- Core test volume is comparable: `core/auth/*.go` ~840 lines incl.
  `auth_test.go` (327 lines); `core/crypto/*.go` similar shape with
  `crypto_test.go` (289 lines) plus `example_test.go`. No separate
  human-review gate exists for either group beyond the shared
  maintainer-review rule for public API changes
  (`.github/CONTRIBUTING.md:176`).

Honest statement: auth and crypto share one coverage floor and one
review rule; neither is singled out.

## 6. CI artifacts (SBOM yes, attest yes, sign no)

- SBOM: yes. `sbom` job (`.github/workflows/ci.yml:344-375`) runs
  `make sbom` (CycloneDX per affected module, `Makefile:137-139`) and
  uploads `sbom/` as `sbom-cyclonedx-*` artifacts (`ci.yml:372-375`).
- Attestation: yes, since the readiness-bar commit. `sbom` job attests
  the SBOMs via
  `actions/attest-build-provenance@62fc1d596301d0ab9914e1fec14dc5c8d93f65cd`
  (`ci.yml:380-386`), verifiable with
  `gh attestation verify sbom/<slug>.json --repo zenta-dev/zever`.
  No cosign/sigstore signing exists (provenance only).
- Release: `.github/workflows/release.yml` (this track) builds static
  binaries (`zever` CLI + showcase server/worker, mirroring
  `examples/showcase/Dockerfile` builder flags) plus `make sbom`,
  attests BOTH binaries and SBOMs with the same pinned
  `actions/attest-build-provenance` action, and attaches everything to
  the tag release (`gh release upload`). Verify with
  `gh attestation verify dist/zever-linux-amd64 --repo zenta-dev/zever`.

## 7. STABILITY promotion language (verbatim)

`STABILITY.md:11-28`:

> - **Core (toward 1.0 promise):** breaking changes are minimized, called out
>   explicitly in the PR and release notes, and need maintainer review.
>   Goal is a stable foundation for 1.0. Pre-1.0 `0.x` may still break core
>   with documented release notes, but the bar is high.
> - **Extended (0.x breaks freely):** breaking changes are expected during
>   pre-1.0 development. They are still documented in release notes
>   (`CHANGELOG.md` `## [Unreleased]` before PR), but no stability promise
>   is made before 1.0.

There is no 1.0 promotion criteria anywhere in the file: no time
threshold, no kit-coverage requirement, no external-user requirement.
"Goal is a stable foundation for 1.0" is aspirational with no exit
condition. Phase8 of this track adds explicit criteria plus an honest
current-standing section, keeping the pre-1.0 policy intact.

## 8. Reference-app positioning (todo vs bookings vs showcase)

`examples/README.md:10-25` lays out three Stage-2 apps; `README.md:66-67`
currently sends newcomers to `examples/todo`:

- `examples/todo/` — auth-gated notes, HTTP+gRPC parity tests. Smallest
  full app; the current "clone first" target by virtue of the README
  link. No Dockerfile.
- `examples/bookings/` — marketplace domain reaching payment/billing/geo/
  media/search/vectorstore, but `examples/bookings/README.md:1-5` states
  "This PR covers schema + CLI-generated output only. No business logic,
  no handlers yet." Not newcomer-ready.
- `examples/showcase/` — DSL-depth shop (all scalars, relations, jobs,
  schedules; `examples/showcase/README.md:1-8`), six backends generated,
  full `zever.yaml` zero-infra wiring, and the only Stage-2 app with a
  production image: `examples/showcase/Dockerfile` (multi-stage,
  `golang:1.27.1-bookworm` builder, `CGO_ENABLED=0` static binary,
  `gcr.io/distroless/static-debian13:nonroot`, `Dockerfile:1-27`).

Correction to the track brief: the brief claims showcase "has Dockerfile
+ compose.yaml". Verified: **no `compose.yaml` exists** under
`examples/showcase/` (only `Dockerfile`, `zever.yaml`, `data/`,
`db/`, `cmd/server`, `cmd/worker`). `docs/production.md:205-216`
documents the Dockerfile build (`docker build -f
examples/showcase/Dockerfile .`) with no compose reference either.
Phase9 positions showcase as "newcomer clone first" on the strength of
the Dockerfile + broadest battery coverage, and does not claim compose.

## Open questions

1. `core/secrets/secretstest` — RESOLVED yes: it is the fifteenth
   in-tree kit (`core/secrets/secretstest/conformance.go`), run by both
   secrets adapters (`adapters/secrets/env/conformance_cover_test.go:21`,
   `adapters/secrets/vault/conformance_cover_test.go:15`). Still
   unnamed in `docs/writing-a-plugin.md:6-10` (out of scope for this
   track) and not in the CI `conformance` job (cache/queue/storage
   only).
2. Scheduler second adapter (postgres-leased) — accept as 1.0-blocker
   proposal, or is embedded-only acceptable at 1.0 with documented
   single-instance limit?
3. Bench baseline — is any perf promise in scope for 1.0, or do the four
   bench files stay informational?
4. Attestation subject — RESOLVED both: CI `sbom` job attests SBOMs
   and `.github/workflows/release.yml` (this track) attests built
   binaries plus SBOMs, attached to the tag release.
5. `examples/README.md:17-19` says "There is deliberately no new Go
   module here" yet `examples/showcase/go.mod`, `examples/todo/go.mod`,
   and `examples/bookings/go.mod` all exist — stale doc, out of scope
   but flagged.
