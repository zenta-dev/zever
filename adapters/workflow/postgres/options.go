package postgres

import (
	"errors"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

// DefaultTable is the workflow-runs table created when Options.Table is empty.
const DefaultTable = "workflow_runs"

// DefaultLeaseTTL is the claim lease granted to a run owner when
// Options.LeaseTTL is unset.
const DefaultLeaseTTL = 30 * time.Second

// DefaultConnectTimeout bounds pool connect during construction.
const DefaultConnectTimeout = 5 * time.Second

// DefaultOperationTimeout bounds a single DB round trip.
const DefaultOperationTimeout = 5 * time.Second

// Options holds typed configuration for the postgres workflow adapter.
// An empty DSN selects sqlite (Path, default ":memory:"); a set DSN opens
// postgres. Table defaults to DefaultTable; Owner identifies this replica's
// lease holder and must differ per replica that may reclaim work.
//
// Pool knobs (DSN/Path/MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime)
// are embedded from coredb.Options so pool tuning stays in one place;
// core/db owns required-field checks for DSN/Path.
type Options struct {
	// Options holds the shared DB pool configuration.
	coredb.Options
	// Table is the workflow-runs table name. Default DefaultTable.
	Table string `json:"table" toml:"table" yaml:"table"`
	// Owner identifies this replica as a lease holder.
	Owner string `json:"owner" toml:"owner" yaml:"owner"`
	// LeaseTTL is the claim lease granted to a run owner.
	LeaseTTL time.Duration `json:"lease_ttl" toml:"lease_ttl" yaml:"lease_ttl"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if err := o.Options.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("postgres: %w", err))
	}

	if o.LeaseTTL < 0 {
		errs = append(errs, errors.New("postgres: lease_ttl must not be negative"))
	}

	if o.Table != "" {
		if err := validateTableName(o.Table); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// validateTableName rejects table names outside [A-Za-z_][A-Za-z0-9_]*
// because the name is interpolated into DDL (orm has no DDL builder).
func validateTableName(name string) error {
	if name == "" {
		return errors.New("postgres: table must not be empty")
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')

		if !ok {
			return fmt.Errorf("postgres: invalid table name %q", name)
		}
	}

	return nil
}
