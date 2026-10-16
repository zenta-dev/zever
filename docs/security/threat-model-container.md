# Threat model: container

Audited against `container/` (`container.go`, `lazy.go`, `close.go`,
`pools.go`, `adapters.go`) at WS6 time.

## Assets

- Every resolved backend client in the process (DB pools, cache/queue
  connections, crypto keys in memory, auth verifiers).
- The process-wide `Container` itself: whoever holds it can resolve any
  configured service.

## Trust boundaries

- `container.New(cfg)` accepts nil cfg (resolves to defaults). Config is
  trusted after `config` validation; the container does no secret handling
  of its own.
- Service accessors are lazy: first call builds and caches; an error clears
  the cache so the next call retries. Concurrent waiters on a failing build
  share that build's error instead of each re-running the builder
  (CHANGELOG v0.7.1).
- `Job()` builds a dispatcher over the resolved `Queue` (no adapter of its
  own); `Scheduler()` shares `Cache`/`Queue`, resolving them as a side
  effect. Closing therefore has shared-ownership edges.

## What zever guarantees

- `Close(ctx)` closes only services actually resolved, in dependency order
  (dependents holding cache/queue refs, then snapshots, then cache/queue
  leaves, then gRPC server). Shared instances close once via
  pointer-identity dedup (`close.go`).
- Close shape is probed per service: `Close(ctx) error` → `Close() error` →
  `Stop() error` → `Shutdown(ctx) error` → no-op. Failures join via
  `errors.Join`; panics become `ClosePanicError`, timeouts become
  `CloseTimeoutError` (value types — `errors.As` against a pointer target
  never matches). A timed-out close that keeps running is flagged
  `Abandoned` and logged (CHANGELOG v0.7.0).
- `container` tests enforce `goleak.VerifyTestMain`: no goroutine may
  outlive package tests. SIGHUP reload (`ApplyReload`) opens replacements
  before swapping, so a failed reopen keeps the old instance.
- Pool sharing is per exact DSN with recorded `PoolMismatches()` diagnostics;
  `dedicated_pool: true` opts out (`pools.go`, CHANGELOG v0.7.0).

## What the caller must do

- Always `Close` the container (defer in tests via `zevertest`, cleanup in
  servers); never leak one per request.
- Build one container per process role (server, worker); do not hand a
  container across a trust boundary (e.g. to untrusted plugin code) — it
  grants every service.
- Never derive the container's root context from a request context
  (`//nolint:containedctx` in `container.go` is deliberate: the root is
  detached by design and canceled in `Close`).
- Heed `pool:` warnings from `zever doctor`: split pools (`dedicated_pool`)
  for queue/workflow/scheduler under backlog.

## Explicit non-guarantees

- The container does not authenticate callers of accessors and does not
  sandbox adapters: a malicious adapter with a registered factory runs with
  full process privileges. Only register factories from trusted code.
- Retry-on-error is for transient build failures, not for masking
  misconfiguration: persistent errors surface on every fresh call.
