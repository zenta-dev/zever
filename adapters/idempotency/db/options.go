package db

import (
	"errors"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

// DefaultTable is the idempotency table created when Options.Table is
// empty.
const DefaultTable = "idempotency"

// DefaultConnectTimeout bounds pool connect during construction.
const DefaultConnectTimeout = 5 * time.Second

// DefaultMinPreserveTTL floors preserved expiries against clock skew.
const DefaultMinPreserveTTL = time.Second

// defaultPrefix namespaces idempotency keys when no prefix is set.
const defaultPrefix = "idem:"

// Options holds typed configuration for the DB-backed idempotency store.
// An empty DSN selects sqlite (Path, default ":memory:"); a set DSN opens
// postgres. Table defaults to DefaultTable; Prefix namespaces keys and
// defaults to "idem:"; TTL is the default reservation lifetime (zero means
// idempotency.DefaultTTL).
//
// Pool knobs (DSN/Path/MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime)
// are embedded from coredb.Options so pool tuning stays in one place.
type Options struct {
	// Options holds the shared DB pool configuration.
	coredb.Options
	// Table is the idempotency table name. Default DefaultTable.
	Table string `json:"table" toml:"table" yaml:"table"`
	// Prefix is the key prefix for idempotency data. Default "idem:".
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
	// TTL is the default reservation lifetime. Zero means
	// idempotency.DefaultTTL.
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
