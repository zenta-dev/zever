package db

import (
	"errors"
	"fmt"
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
