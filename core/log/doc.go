// Package log provides structured logging with swappable adapters.
//
// It carries Logger, Event, Context, Field, and Level for one event builder
// per log line with context-carried loggers. It does not emit metrics or
// traces, it only records structured log events.
//
// Type safety: Logger plus Event plus Context interfaces with typed Options
// plus Adapter enum plus Factory. Level and FieldType enums plus typed Field
// constructors cover string, int, float, bool, duration, time, error, and any
// values. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests, with Noop discarding all events. Config file
// plus env LOG_ADAPTER (no prefix, e.g. LOG_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Log(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Loggers travel in ctx via
// WithContext, FromContext, and ContextWithLogger without storing ctx in the
// logger. Sync flushes buffered output and takes no ctx. There is no Close;
// only resolved services close via the container.
//
// Errors: sentinel errors, errors.Is compatible, prefixed log:. Name service
// and field only in errors, with DuplicateError, UnknownAdapterError,
// InvalidLevelError, and InvalidAdapterError carrying the adapter or level.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Never put secrets, passwords, or tokens into fields or messages. Validate
// adapter and level names.
//
// Performance: check Enabled before building expensive events. Noop is a
// zero-cost discard path. Adapters buffer and flush on Sync. Bounded pools and
// timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init
// wiring.
//
// Example: see ExampleOpen in example_test.go.
package log
