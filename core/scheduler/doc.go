// Package scheduler dispatches registered jobs on cron specs with swappable adapters.
//
// It registers job names on cron specs and fires them through a job dispatcher with per-slot dedup. It is not a queue and does not catch up missed ticks.
//
// Type safety: Scheduler plus typed Options plus Adapter enum plus Factory. EntryID aliases job.EntryID. Specs use cron standard format with MaxSpecLen 256 and fail at Schedule time. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env SCHEDULER_<FIELD> (no prefix, e.g. SCHEDULER_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Scheduler(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Stop ends ticks waiting up to CloseTimeout and Start is idempotent and non-blocking. There is no Close; Stop releases ticks and only resolved services close. Note scheduler shares Cache/Queue via the resolved Job dispatcher.
//
// Errors: sentinel errors, errors.Is compatible, prefixed scheduler:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Spec length is bounded and names are validated before dispatch.
//
// Performance: in-process cron with per-slot locking and at-most-once dispatch per slot. Bounded pools and timeouts via CloseTimeout defaulting to 5s.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package scheduler
