// Package container builds one lazily resolved service instance per process
// from a config.Config, with no globals and no init-time wiring. Each
// process constructs its own Container with New, and New(nil) falls back
// to config.Default.
//
// Every service resolves at most once through the internal lazy[T] helper:
// concurrent callers share a single in-flight build, and a failed build is
// retried on the next call instead of being cached. Container is a sealed
// struct: unexported fields plus an unexported seal method keep external
// packages from constructing or impersonating it.
//
// Close shuts services down in reverse dependency order; see Close for the
// exact ordering and timeout contract.
package container
