package db

import (
	"errors"
	"time"
)

// Options holds typed configuration for database adapters.
// Fields form a union of all adapter options; each adapter consumes only
// the fields it needs. DSN holds the Postgres connection string.
// MaxConns caps the connection pool. MinConns semantics differ by adapter:
// Postgres treats it as the minimum pool size maintained by the pool, while
// SQLite applies it as the file-backed idle connection target.
// MaxConnLifetime and MaxConnIdleTime bound pooled connection reuse.
// Path holds the SQLite database file path.
type Options struct {
	// DSN holds the Postgres connection string.
	DSN string
	// MaxConns caps the connection pool size.
	MaxConns int
	// MinConns is the minimum pool size for Postgres and the idle
	// connection target for SQLite.
	MinConns int
	// MaxConnLifetime bounds how long a pooled connection may be reused.
	MaxConnLifetime time.Duration
	// MaxConnIdleTime bounds how long a pooled connection may stay idle.
	MaxConnIdleTime time.Duration
	// Path holds the SQLite database file path.
	Path string
}

// Validate checks options for consistency, joining all violations.
// It performs only adapter-independent range checks; adapter factories own
// required-field checks such as DSN or Path presence.
func (o Options) Validate() error {
	var errs []error

	if o.MaxConns < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max conns must not be negative"})
	}

	if o.MinConns < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "min conns must not be negative"})
	}

	if o.MaxConnLifetime < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max conn lifetime must not be negative"})
	}

	if o.MaxConnIdleTime < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max conn idle time must not be negative"})
	}

	return errors.Join(errs...)
}
