// Package db provides database adapter registration and lifecycle.
//
// DB is intentionally open (no unexported methods) so third-party adapters
// outside this module can implement it and register via Register, following
// the database/sql driver registry design. Adapters are selected via the
// Adapter enum and constructed from typed Options by a registered Factory.
package db
