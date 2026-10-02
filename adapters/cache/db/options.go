package db

import (
	"errors"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

// DefaultTable is the cache table created when Options.Table is empty.
const DefaultTable = "cache"

// DefaultConnectTimeout bounds pool connect during construction.
const DefaultConnectTimeout = 5 * time.Second

// defaultPrefix namespaces cache keys when no prefix is set.
const defaultPrefix = "cache"

// Options holds typed configuration for the DB-backed cache. An empty DSN
// selects sqlite (Path, default ":memory:"); a set DSN opens postgres.
// Table defaults to DefaultTable; Prefix namespaces keys and defaults to
// "cache"; TTL reserves a store-level default entry lifetime for shape
// parity with the session/db options. Entry writes always use the per-call
// ttl (ttl <= 0 persists), so a set TTL is validated but does not alter
// writes.
//
// Pool knobs (DSN/Path/MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime)
// are embedded from coredb.Options so pool tuning stays in one place.
type Options struct {
	// Options holds the shared DB pool configuration.
	coredb.Options
	// Table is the cache table name. Default DefaultTable.
	Table string `json:"table" toml:"table" yaml:"table"`
	// Prefix is the key prefix for cache data. Default "cache".
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
	// TTL is the default entry lifetime. Zero means per-call ttl governs
	// (ttl <= 0 persists).
	TTL time.Duration `json:"ttl" toml:"ttl" yaml:"ttl"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if err := o.Options.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("db: %w", err))
	}

	if o.TTL < 0 {
		errs = append(errs, errors.New("db: ttl must be >= 0"))
	}

	return errors.Join(errs...)
}
