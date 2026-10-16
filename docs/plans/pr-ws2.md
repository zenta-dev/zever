# PR WS2: verified quickstart with CI job (+ example app proposal)

## What / why

The quickstart doc (`docs/src/content/docs/start/quickstart.mdx`) drifted from
CLI reality: it carried a stale caution claiming `zever serve` exits with
`unknown adapter: jwt` on a fresh scaffold, listed a redundant `go mod tidy`,
and never showed what calling the API actually returns. This PR re-executes
every quickstart command in a temp dir, fixes the doc to match the observed
output, and adds a CI job that runs the same commands verbatim so future
drift fails loudly.

## Evidence (commands + results, run 2026-10-16 against this branch)

Built the CLI from the worktree (`go build -o /tmp/qs-zever ./cmd/zever`),
then in temp dirs:

- `zever new hello --dir /tmp/qs-cap/hello` → exit 0, scaffold + auto
  `go mod tidy`, `go build ./...` exit 0 with no extra tidy.
- Doc `Task` schema (`@validate`, `@default(now())`, `auth: required`,
  `paginated: true`) → `zever check schema/app.zen` → `OK 1 file(s) valid`.
- `zever compile --backend=zenorm,atlas,openapi --out ./generated
  schema/app.zen` → `compiled 1 file(s) → ./generated (4 outputs, ...)`.
- `zever db migrate --dry-run --adapter=sqlite schema/app.zen` → ends with
  `dry run — no changes applied` / `1 statement(s) would be executed`.
- `zever db migrate --adapter=sqlite --dsn=data/app.db schema/app.zen` →
  `OK applied 1 statement(s) via sqlite`.
- `zever doctor` → all 9 batteries OK; `zever serve` boots, serves HTTP
  `:8080` + gRPC `:9090`.
- `curl -i localhost:8080/healthz` → `HTTP/1.1 200 OK`, empty body.
- `zever routes schema/app.zen` → `POST /v1/tasks` and `GET /v1/tasks`
  registered; `GET /v1/tasks` returns `404 page not found` (no handlers yet —
  plumbing is generated, business logic is yours).

Total wall-clock for the flow (warm module cache): well under 10 minutes.

## Example app proposal (not built — proposal only)

**`examples/saas-starter`** — a realistic multi-tenant SaaS skeleton showing
batteries working together, all on zero-infra local adapters so it runs with
only Go + sqlite:

- **Schema** (`schema/app.zen`): `Tenant` (id, slug unique), `User`
  (tenant FK, email unique, password via `password` battery), `Task`
  (tenant-scoped, as in the quickstart), `BillingInvoice` mirroring the
  `billing/stub` shape.
- **Auth**: `auth=jwt` (dev secret in `zever.yaml`, env override documented)
  + `permission=noop` initially, with a comment pointing at the real
  adapter; login RPC issuing JWTs, tenant slug in claims.
- **Tenants**: per-request tenant scoping in the service layer (example
  middleware reading the claim into context); seed data creates two tenants
  to demonstrate isolation in tests.
- **Jobs**: one `queue=memory` job (`TaskReminder`) + `scheduler=memory`
  schedule (daily digest), run via the scaffolded `cmd/worker`.
- **Payments stub**: `billing/stub` checkout RPC returning the stub invoice;
  clearly labeled as stub with a pointer to the Stripe/Paddle adapters.
- **OTLP**: `observability=stdout` default with a one-line `otlp` swap
  documented (env endpoint only, no new deps).
- **Tests**: table-driven service tests on sqlite `:memory:` + `zevertest`
  HTTP client; no network, deterministic.
- **CI**: reuse the root quickstart pattern (new job or extend
  `quickstart.yml`): scaffold-independent — the example is committed, so CI
  runs `zever compile --check`, `go build ./...`, `go test ./...`, migrate
  + serve + healthz exactly like the quickstart job.

Non-goals: no frontend, no real payment provider, no postgres/redis services
in CI (local adapters only), no new third-party deps.

## Risk + rollback

Risk: low. Touches one docs page, one new workflow file, CHANGELOG, and this
plan file. No library code, no public API changes, no new dependencies.
Rollback: revert the PR; the docs site and CI return to their prior state.

## Breaking / security

- Breaking: no.
- Security: no. The quickstart uses the scaffold's deterministic dev-only JWT
  secret (already labeled dev-only by the scaffold); no secrets added.
  CI job permissions are `contents: read`, no external services.

## Unverified / TODO(verify)

- TODO(verify): full flow on a truly clean machine (only Go + install.sh
  release CLI) — verified here against a source-built CLI; the
  `--framework-version` default path for release users is untested.
- TODO(verify): `quickstart.yml` green on GitHub-hosted runners (only run
  locally so far); watch first CI run on the PR.
- TODO(verify): docs site build (`npm run build` in `docs/`) after the
  `.mdx` edit — no local Node run done.
- Human review: wording of the 404-handlers note; whether to keep `--force`
  in the doc command (harmless on fresh dirs, needed for reruns).
