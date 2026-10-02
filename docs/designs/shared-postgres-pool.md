# Shared Postgres pool (connshare Phase 6) — design proposal

Status: **proposal, needs human sign-off. No code in this phase.**
Branch: `consol/connshare` on top of `main` at `65b904b`
(containing merged PRs #273–#281). Implementation is a later pass.

## 0. Why this file lives here

`docs/` is the Astro user site (`astro.config.mjs`, content under
`src/content/docs/...` — tutorials, digging-deeper, reference). There is
no `plans/`, `designs/`, `proposals/`, or `adr/` directory anywhere in the
repo (verified by filename search). This proposal is contributor-facing
(container lifecycle + pool ownership), not end-user documentation, so it
must not publish to the site and must not hide as an appendix of
`docs/writing-a-plugin.md` (wrong topic: plugins, not container
lifecycle). Standalone `docs/designs/shared-postgres-pool.md` keeps
design-only artifacts separate from shipped user docs. If maintainers
later adopt an ADR/plans layout, move this file there.

## 1. Goal

Let the container resolve **one pool per DSN** and share it across every
battery pointing at the same Postgres DSN, instead of opening one pool per
battery today. Existing `zever.yaml` files keep working unchanged; sharing
activates only on exact-DSN match (or explicit flag — see §5 and sign-off
Q2). Borrowed connections are never closed by adapters (existing `owns`
precedent). Design ends at proposal.

Non-goals: changing public battery interfaces, changing query behavior,
cross-DSN pooling, automatic WAL enforcement, metrics backends.

## 2. Current state (verified on main)

Every db-backed battery opens its **own** pool in `New`/`Open` today, and
every one already exposes a borrowed-connection seam (`NewFromDB` /
`OpenFromDB` with `owns=false`) added by the consolidation streams. The
design builds on those seams; no new seam shape is needed.

| Battery | Own pool today (`New`/`Open`) | Borrowed seam | Container accessor (each opens independently via `openService`) | Config entry |
|---|---|---|---|---|
| db/postgres | `adapters/db/postgres/postgres.go:332` `New` → `pgxpool.ParseConfig` (`:337`) → `pgxpool.NewWithConfig` (`:373`); pool knobs at `:354-355` | n/a (pool owner) | `container/services.go:110-115` `DB()` via `openService` (`:50-66`) | `config/config.go:60` `DB Service[db.Options]`; `core/db/options.go:16-29` (`DSN`, `MaxConns`, `MinConns`, lifetimes, `Path`) |
| search/db | `adapters/search/db/postgres.go:87-92` `New`→`Open`; `Open` switches `isPostgresDSN`→`dbpostgres.New` else `dbsqlite.New`; `owns:true` at `:174` | `NewFromDB` `:137`, `OpenFromDB` `:151`, `openFromDB(..., owns bool)` `:155-157` | `container/services.go:322-327` `Search()` | `config/config.go:80` `Search Service[search.Options]`; pool knobs embedded `adapters/search/db/options.go:14-22`, `isPostgresDSN` `:41`, `dbOptions` `:50` |
| vectorstore/db | `adapters/vectorstore/db/pgvector.go:77-82` `New`→`Open`; `isPostgresDSN`→`dbpostgres.New` else `dbsqlite.New`; `owns` at `:167` | `NewFromDB` `:126`, `OpenFromDB` `:145`, `openFromDB` `:149-151` | `container/services.go:357-362` `VectorStore()` | `config/config.go:85`; `isPostgresDSN` `:327`, `dbOptions` `:336` |
| workflow/db | `adapters/workflow/db/postgres.go:166-171` `New`→`Open`; DSN set→`dbpostgres.New` else `dbsqlite.New`; `owns` at `:267` | `NewFromDB` `:211`, `OpenFromDB` `:225`, `openFromDB` `:230-232` | `container/services.go:371-376` `Workflow()` | `config/config.go:87`; pool-knob comment `adapters/workflow/db/options.go:29` |
| scheduler/postgres | `adapters/scheduler/postgres/postgres.go:164-169` `New`→`Open`; `PoolOptions.DSN` set→`dbpostgres.New` else `dbsqlite.New`; `owns` at `:276` | `NewFromDB` `:209`, `OpenFromDB` `:223`, `openFromDB` `:228-230` | `container/services.go:302-320` `Scheduler()` (additionally resolves `Job()`→`Queue()` at `:309-316`, sharing the queue instance — precedent for container-level sharing) | `config/config.go:79`; `PoolOptions coredb.Options` embed `adapters/scheduler/postgres/options.go:26-35` |
| session/db | `adapters/session/db/session.go:49` `New`; DSN set→`dbpostgres.New` else `dbsqlite.New`; `owns` at `:130` | `NewFromDB` `:88`, `OpenFromDB` `:102`, `openFromDB` `:107-109` | `container/services.go:336-341` `Session()` | `config/config.go:82`; pool-knob comment `adapters/session/db/options.go:29` |
| idempotency/db | `adapters/idempotency/db/idempotency.go:47` `New`; same DSN switch; `owns` at `:128` | `NewFromDB` `:86`, `OpenFromDB` `:100`, `openFromDB` `:105-107` | `container/services.go:184-189` `Idempotency()` | `config/config.go:66`; pool-knob comment `adapters/idempotency/db/options.go:30` |
| cache/db | `adapters/cache/db/cache.go:33` `New`; same DSN switch; `owns` at `:109` | `NewFromDB` `:72`, `OpenFromDB` `:86`, `openFromDB` `:91-93` | `container/services.go:96-101` `Cache()` | `config/config.go:58`; pool-knob comment `adapters/cache/db/options.go:28` |
| queue/db | `adapters/queue/db/queue.go:168` `New`; same DSN switch; `owns` at `:281` | `NewFromDB` `:208`, `OpenFromDB` `:222`, `openFromDB` `:227-229` | `container/services.go:276-281` `Queue()`; `Job()` at `:202-211` builds a dispatcher over the resolved queue (no adapter, shares instance — closest existing sharing precedent) | `config/config.go:76`; pool-knob comment `adapters/queue/db/options.go:44` |

Lifecycle precedent (all nine): `Close` is a no-op when borrowed —
`if !d.owns { return nil }`, e.g. search `:621-635`, queue `:770-783`
(`Close` idempotent via `closed.CompareAndSwap`), same shape in
vectorstore `:749-757`, workflow `:668-672`, scheduler `:649-657`,
session `:339`, idempotency `:302`, cache `:265`. Failed `New` closes the
owned conn (`_ = conn.Close(...)`), failed `NewFromDB` never closes the
caller's conn.

Container close today: `container/close.go:185-315` `Close` closes
scheduler/job first, then `snapshots()` (`:116-150`, 31 entries incl.
`c.db` at `:123`, `search`/`session`/`vectorstore`/`workflow`/
`idempotency` inside snapshots), then `cache`/`queue` last (`:275-281`),
grpc separate. `tryClose` dedups identical pointers (`:196-207`,
`dedupKey`), `closeAny` probes `Close(ctx)`→`Close()`→`Stop()`→
`Shutdown(ctx)` (`:317-339`), per-service timeout `DefaultCloseTimeout`
(`:19`). Container lazy fields: `container/container.go:52-87`;
retry-on-failure + singleflight: `container/lazy.go` (`get` retries after
failure, `getIfResolved` never builds).

Findings worth recording:

- `adapters/db/postgres/postgres.go:324-332` carries the pool-cap guidance
  ("each adapter opens its own pool … keep the sum of per-adapter
  `MaxConns` below the server's `max_connections`; search and vectorstore
  default to 4 each (see their PoolConfig helpers)"). The referenced
  `PoolConfig` helpers **do not exist** (repo-wide search finds only that
  comment). Treat the "default 4 each" as stale; the live rule is
  `coredb.Options.MaxConns` (0 = driver default) per adapter. The
  implementation pass should either delete or correct that comment.
- `adapters/db/sqlite/sqlite.go:52-56`: `:memory:` forces a **single**
  pooled connection because extra connections to an in-memory DB get
  isolated databases. `buildDSN` (`:489-492`) maps `:memory:` to
  `file::memory:?cache=shared` with FK + busy-timeout pragmas. Consequence
  for sharing: `:memory:` DSNs must **never** be shared across batteries
  (would merge isolated DBs); file paths may share (see §7).
- Solid Queue guidance is quoted in `adapters/queue/db/doc.go:17-20`:
  durable jobs on a DB you already run, "a separate queue database with
  3–5 connections per worker is recommended there, and applies here too".

## 3. Approaches considered

**A. Share-by-default on exact-DSN match + `DedicatedPool` opt-out
(recommended).** Container keeps a DSN→pool registry. When a db-backed
battery resolves with a DSN already in the registry (exact string match
after trim) and its options do not set `DedicatedPool`, the container
borrows the pooled `db.DB` via the existing `NewFromDB`/`OpenFromDB`
seam (`owns=false`). Setting `DedicatedPool: true` (or a differing DSN)
restores today's behavior verbatim. Migration: zero — existing files keep
working; sharing activates only where the operator already pointed two
batteries at the same DSN.

**B. Opt-in sharing flag.** Same registry, but sharing happens only when
the operator sets `SharedPool: true` (or names a pool). Safer against
surprise, but every existing multi-battery deployment keeps N pools until
edited, and two flags (`SharedPool` on one side, nothing on the other)
invite half-migrated states. Rejected: the exact-match condition in A is
already explicit operator intent ("these two DSNs are the same
database").

**C. Named-pool / DSN-alias indirection.** New top-level config section
(`pools: {main: {dsn, max_conns...}}`) with per-service `pool: main`
references. Most expressive (per-pool caps in one place), but a new config
schema, new env-var surface, new validation, and a migration story for
every existing file. Rejected for Phase 6; can be layered later with the
registry as the backing store (the registry key becomes the pool name).

Recommendation is A: smallest config delta (one bool per db-backed
options), zero-touch migration, escape hatch built in (§9).

## 4. Registry: where the DSN→pool mapping lives

Container-owned, unexported, in the `container` package (new file in the
implementation pass, e.g. `container/pools.go` — not created here):

- Key: normalized DSN — `strings.TrimSpace` applied; **no other
  normalization** in Phase 6 (no URL reordering, no password stripping for
  the key — the key never leaves the process). Exact-match means
  `postgres://...?sslmode=disable` and the same URL without the query are
  different pools. Document this; revisit canonicalization only if
  operators complain (sign-off Q4).
- Backend split: Postgres DSNs share via one `dbpostgres.New` pool;
  sqlite **file paths** share via one `dbsqlite.New` pool keyed by
  cleaned path; `:memory:` never shares (see §7). The registry stores the
  opened `db.DB` plus the pool knobs it was built with, a refcount, and a
  `shared bool` marker for observability.
- Concurrency: guard with a mutex + per-key singleflight (same shape as
  `container/lazy.go` — first opener builds, concurrent borrowers wait;
  build failure is not cached and retries). `container.New(nil)` stays
  valid: registry is lazily created, no globals (AGENTS.md: no globals).
- Refcounting: increment on borrow, decrement never needed for
  correctness of `Close` (container closes each registry pool exactly once
  at the end), but keep a count for logs/tests (`shared by N services`).
- What the registry does **not** do: no query routing, no per-tenant
  splitting, no cross-DSN failover.

## 5. Config shape

Add one opt-out field to each db-backed options struct (or to
`coredb.Options` if maintainers prefer a single definition — sign-off
Q1):

```yaml
search:
  adapter: db
  options:
    dsn: "postgres://app:secret@db:5432/app?sslmode=require"
    max_conns: 10
    dedicated_pool: false   # default false → share; true → today's behavior
```

- Share-by-default justification: the DSN already names the database;
  two identical DSNs are unambiguous operator intent; default-false keeps
  every existing file byte-compatible (zero value = share, and sharing
  only triggers on exact match, so single-battery deployments see no
  behavior change at all).
- Env mapping falls out of the existing rule (`<SERVICE>_<FIELD>`, e.g.
  `SEARCH_DEDICATEDPOOL`), best-effort unknown-var ignore preserved.
- Alternative (separate DSN alias to force a dedicated pool, e.g.
  duplicate the DSN string with a `#comment` fragment): rejected —
  fragile, invisible in redacted output, and breaks the exact-match rule's
  honesty. One explicit bool beats string tricks.
- Serialization: `json`/`toml`/`yaml` snake_case tags required per repo
  convention (`dedicated_pool`).

## 6. Pool-cap policy: single cap, not split

Today's rule (`adapters/db/postgres/postgres.go:329-331`) says keep the
**sum** of per-adapter `MaxConns` below `max_connections`. With sharing,
that rule inverts: the shared pool has **one** cap, sized once.

Proposed rule: the shared pool is built with the **first opener's**
pool knobs (`MaxConns`, `MinConns`, lifetimes), where "first" is
deterministic container resolution order — `DB()` first if the app
resolves it, else whichever battery resolves first. Later borrowers'
pool knobs are ignored on the shared path (their non-pool options —
table names, prefixes, TTLs — still apply). If any borrower sets a
nonzero `MaxConns` differing from the shared pool's, emit a one-line
stderr/log warning naming the service (never the DSN — see §8) so the
silent-ignore is visible.

Worked example: `max_connections=100` on the server. Today: db 25 +
search 10 + vectorstore 10 + workflow 10 + scheduler 5 + session 5 +
idempotency 5 + cache 10 + queue 10 = 90 (under 100, barely). Shared:
one pool `MaxConns=25` (or tuned once, e.g. 30) serves all nine —
headroom restored, connection-churn gone. Operators who need isolation
(queue under load) set `dedicated_pool: true` on that battery and go back
to budgeting its cap separately (§9).

Open cap questions (sign-off Q3): first-opener-wins vs
max-of-participants vs require-`db`-service-as-owner. Recommendation is
first-opener-wins for determinism + simplicity; max-of-participants makes
pool size depend on resolution order anyway (whichever max arrives first
vs late-joiners), and db-as-owner breaks apps that never resolve `DB()`.

## 7. SQLite file DSNs

- Same-file sharing: two batteries with identical `Path` (after
  `filepath.Clean`, relative resolved against the same CWD — document the
  CWD caveat) share one `dbsqlite` pool. Safe: file locking already
  arbitrates multi-pool access today; one pool only reduces contention.
  Schema setup per battery is unchanged (each adapter still ensures its
  own tables over the shared conn — the `openFromDB` path already does
  this).
- `:memory:` isolation: never share. Each `:memory:` opener keeps its
  private single-conn pool (`sqlite.go:52-56`). Sharing would silently
  merge logically separate stores (cache rows visible to sessions, queue
  claims colliding with workflow leases). Exact-match on the string
  `":memory:"` must be excluded from the registry, not just unlikely.
- WAL note: sharing does not change journal mode. Concurrent writers
  (queue polling + workflow leases + scheduler claims on one file) benefit
  from WAL; today WAL is opt-in via `?_pragma=journal_mode(WAL)` query
  params (`sqlite_test.go:1265` shows the spelling) while `buildDSN`
  enforces only FK + busy-timeout. Phase 6 recommends **documenting**
  the WAL opt-in for shared-file deployments, not forcing it (forcing
  rewrites operator pragma choices and breaks read-only media).
- `file::memory:?cache=shared` is sqlite-`:memory:` under another name:
  treat as non-shareable, same as `:memory:`.

## 8. Observability: shared vs dedicated without leaking the DSN

Precedent: `config.Redact` / `Config.RedactedServices`
(`config/redact.go:175`, `config/config.go:139`); `dsn` is an exact-match
sensitive key (`isSensitiveKey`); container errors name the service only,
never resolved values (`container/services.go:50-66` comment,
`container/close.go:152-163` names list).

- Logs: on first open, one line per registry entry —
  `container: pool shared postgres host=db dbname=app services=[db,search,...]
  max_conns=25` — built from the **parsed URL host/dbname only**, never
  userinfo or query. On dedicated open — `container: pool dedicated
  service=queue backend=postgres ...`. Reuse the redacted-preview
  discipline (host + dbname is the display contract; full DSN never
  logged).
- Errors: wrap with service name as today (`container: search: ...`);
  add `shared`/`dedicated` marker to the message prefix, e.g.
  `container: search (shared pool): ...`. No DSN content in errors
  (sqlite `New` already guarantees "No Path/DSN content is included in
  errors" — `sqlite.go:52-66` comment; postgres `New` "never logs the
  DSN" — `postgres.go:324-327`).
- `RedactedServices` output: add `dedicated_pool` (non-sensitive bool,
  shown) and keep `dsn` redacted. A `shared_with: [...]` derived field is
  explicitly out of scope (derived state, not config).

## 9. Lifecycle and Close ordering

- Borrowers keep `owns=false` (existing precedent, §2 table): their
  `Close` stays a no-op; idempotency via existing `closed.CompareAndSwap`
  is untouched. The registry is the sole owner and closes each pool once.
- Close sequence (extends `container/close.go:185-315`): scheduler/job
  first → snapshots (borrowers; no-op closes, harmless) → `cache`/`queue`
  → **registry pools** (after every borrower snapshot, so no
  use-after-close; `db` pool included here rather than its current
  snapshot slot at `:123`) → plugins → grpc. `TestContainer_Snapshots_CoversAllServices`
  must be updated to account for the registry (or the registry closes
  outside `snapshots()` with its own coverage test — implementation
  detail, sign-off Q5).
- Double-close safety today leans on pointer dedup (`:196-207`). With
  sharing, borrowers never close, so the shared pool pointer is reached
  exactly once (via the registry), and per-battery `Close` idempotency is
  unchanged. No `Close` signature changes; no interface changes
  (AGENTS.md: don't add methods to public interfaces).
- Failure paths: shared-pool build failure clears that registry key and
  surfaces `container: <service>: ...` for the triggering service; other
  services retry on next access (matches `lazy.get` retry semantics).
  Failed `NewFromDB` never closes the shared conn (existing guarantee).

## 10. Migration path

1. Upgrade: no config change. Single-battery deployments: byte-identical
   behavior (registry holds one entry, no borrower).
2. Multi-battery same-DSN deployments: pools collapse automatically on
   exact match. Operators should previously have budgeted
   sum-of-caps < `max_connections`; after upgrade they hold one pool at
   first-opener size — strictly fewer connections. The cap-mismatch
   warning (§6) tells them if a battery's tuning is now ignored.
3. Opt-out: set `dedicated_pool: true` on any battery to restore a private
   pool (e.g. queue under load — §11). Distinct DSNs (different DB name,
   user, or query string) never share, no flag needed.
4. Rollback: set `dedicated_pool: true` everywhere, or downgrade — old
   binaries ignore the unknown field only if the file loader is
   non-strict for unknown fields; `Load` is strict (`AGENTS.md`), so
   downgrade with the new field present fails closed with a decode error.
   Document: remove `dedicated_pool` lines before downgrading.

## 11. Escape hatch: Solid Queue's dedicated-DB recommendation

`adapters/queue/db/doc.go:17-20` records the upstream guidance: Solid
Queue recommends a **separate queue database with 3–5 connections per
worker**. The design honors this as the documented escape hatch:

- Queue (and workflow/scheduler, which share the polling-transport
  trade-off) under sustained load should run with `dedicated_pool: true`
  and, for full isolation, a **separate database** (distinct DSN), not
  just a dedicated pool on the same server.
- Docs for the implementation pass must carry this recommendation into the
  queue/workflow/scheduler user docs; the design does not auto-detect
  load and does not force dedication.

## 12. Open questions (need explicit sign-off before any code)

1. `DedicatedPool` placement: per-service options structs (9 touch
   points, explicit per battery) vs once on `coredb.Options` (single
   definition, inherited everywhere, but also lands on `db` itself where
   it is meaningless)? Recommendation: `coredb.Options` + ignore on
   `db`.
2. Activation: share-by-default on exact match (recommended, §3 A) vs
   opt-in flag? Confirm default-false `dedicated_pool` is acceptable.
3. Cap rule: first-opener-wins (recommended, §6) vs max-of-participants
   vs db-service-owns-the-pool?
4. Key normalization: exact string match after trim (recommended) vs
   URL-canonicalized match (reorders query, lowercases host)? Exact is
   honest but can surprise (`?sslmode=require` vs bare); canonical risks
   merging DSNs the operator considers distinct.
5. Close coverage: move `db` out of `snapshots()` into the registry-close
   section and update `TestContainer_Snapshots_CoversAllServices`, or
   keep snapshots untouched and close registry pools in the
   cache/queue-last band? Former is cleaner; latter is a smaller diff.
6. SQLite file-key cleaning: `filepath.Clean` + CWD-relative resolution
   documented, or absolute-path-only sharing (relative paths never
   shared)? Former shares more; latter is safer against CWD ambiguity.
7. Stale comment fix: correct `postgres.go:329-331` (`PoolConfig`
   helpers don't exist) inside the implementation pass, or separate
   docs commit? Recommendation: fix in-pass, one line + this doc's
   finding as citation.

## 13. Out of scope (later passes, not this design)

- Implementation itself (`container/pools.go`, accessor rewiring,
  `DedicatedPool` fields, tests, `container/README.md` + user-docs
  updates).
- Named pools / pool aliases (§3 C) — viable follow-up on the registry.
- Pool metrics / health endpoints; per-service query tagging.
- Forced WAL, pragma changes, sqlite multi-writer tuning beyond docs.
- Canonical DSN normalization beyond trim (see Q4).
- `writing-plans` implementation plan — invoked only after this design
  is approved (per brainstorming workflow, held here: no code until
  sign-off).
