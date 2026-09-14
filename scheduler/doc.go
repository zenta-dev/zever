// Package scheduler dispatches registered jobs on cron specs.
//
// The embedded adapter runs an in-process cron that enqueues due jobs
// through a job.Dispatcher. Each tick acquires a per-slot lock via
// job.UniqueLocker, giving at-most-once dispatch per slot across scheduler
// instances; ticks that lose the lock are skipped with no catch-up.
//
// Specs use cron.ParseStandard: 5 fields plus descriptors (@every, @daily,
// ...) with an optional CRON_TZ= prefix. Specs are validated at
// Schedule-time and fail fast. Times evaluate in the local timezone unless
// CRON_TZ= sets another. A DST leap-ahead skips ticks that fall in the gap.
package scheduler
