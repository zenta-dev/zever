// Package job provides job orchestration with swappable queue backends.
//
// It handles typed registration, delayed and unique dispatch, retries with
// backoff, batching, cron schedules, and concurrent worker execution. It does
// not run its own queue backend or scheduler clock, it builds on queue.Queue
// and cache.Cache instead.
//
// Type safety: generic Register[T] with JSON payload decoding plus Definition,
// Option, Priority, RetryPolicy, DispatchOption, Dispatcher, Worker, Scheduler,
// UniqueLocker, Batch, and BatchStore. There is no Adapter enum or Factory and
// no Open for this package. Unsupported features fail closed with unknown-job,
// nil-dispatcher, and non-positive TTL errors.
//
// DX: plain constructors over resolved instances instead of a registry: build
// Dispatcher over a queue.Queue, Worker over the same queue, Scheduler via
// NewScheduler, dedup via NewUniqueLocker over a cache.Cache, and typed jobs
// via Register. There is no adapter name to parse and no job section in config
// files; queue and cache settings drive the backends. See config/README.md.
//
// Container: container.New(cfg) then c.Job(). Job returns a Dispatcher over the
// resolved Queue, resolving the queue as a side effect. Lazy per-service
// singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO such as Dispatch, Run, Acquire, and
// Release, and is never stored except Scheduler.Run which captures ctx for
// later tick firing. Worker.Run and Scheduler.Run block until ctx is done.
// Worker detaches handlers with context.WithoutCancel bounded by DrainTimeout
// and waits for drain on shutdown. There is no Close on Dispatcher, Worker, or
// Scheduler; only resolved services close via the container.
//
// Errors: sentinel errors, errors.Is compatible, prefixed job:. Name service
// and field only in errors, with DuplicateJobError and UnknownJobError carrying
// the job name.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Only the job name is logged, never the payload. Unique keys are SHA-256
// hashes of job name and key. Validate job names, TTLs, and cron specs.
//
// Performance: semaphore-bounded concurrency defaulting to 10. Exponential
// backoff from 5s doubling per attempt capped at 24h. Poll backoff from 1ms to
// 5s, batch sweep every second, batch settle default 5m. Bounded pools and
// timeouts.
//
// Concurrency: safe for concurrent use unless noted. Registry is guarded by an
// RWMutex. No globals beyond the definition registry, no init wiring.
//
// Example: see ExampleRegister in example_test.go.
package job
