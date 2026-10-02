package postgres

import (
	"errors"
	"fmt"
	"strings"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

// DefaultConnectTimeout bounds pool connect during construction.
const DefaultConnectTimeout = 5 * time.Second

// Options holds typed configuration for the DB-backed search adapter. An
// empty DSN selects sqlite (Path, default ":memory:"); a postgres URL DSN
// opens postgres; any other non-empty DSN is treated as a sqlite path.
//
// Pool knobs (DSN/Path/MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime)
// are embedded from coredb.Options so pool tuning stays in one place;
// core/db owns required-field checks for DSN/Path.
type Options struct {
	// Options holds the shared DB pool configuration.
	coredb.Options
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if err := o.Options.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("postgres: %w", err))
	}

	return errors.Join(errs...)
}

// isPostgresDSN reports whether dsn selects the postgres backend: a URL
// with a postgres scheme. Anything else (including ":memory:") is a sqlite
// path. Matching is case-insensitive with surrounding whitespace ignored.
func isPostgresDSN(dsn string) bool {
	s := strings.ToLower(strings.TrimSpace(dsn))

	return strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://")
}

// dbOptions maps a core search DSN onto shared pool options: a postgres
// URL stays a DSN, anything else (including empty) becomes a sqlite Path.
// Empty selects ":memory:".
func dbOptions(dsn string) coredb.Options {
	if isPostgresDSN(dsn) {
		return coredb.Options{DSN: dsn}
	}

	if strings.TrimSpace(dsn) == "" {
		return coredb.Options{Path: ":memory:"}
	}

	return coredb.Options{Path: dsn}
}
