// Package zevertest provides test helpers for zever applications: a
// per-test container wired to zero-infrastructure adapters, a fake clock and
// ID source for deterministic time and identifiers, and an HTTP test client
// for exercising routes in-process.
//
// Everything here runs offline: memory, local and sqlite adapters only, with
// per-test temp directories, so tests stay deterministic, parallel-safe and
// free of Docker or network access. The helpers are for tests only and are
// never imported by production code.
package zevertest
