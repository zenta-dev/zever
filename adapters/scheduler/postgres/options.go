package postgres

import (
	"errors"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// DefaultTable is the scheduler-slots table created when Options.Table is empty.
const DefaultTable = "scheduler_slots"

// DefaultLeaseTTL is the slot lease granted to a scheduler owner when
// Options.LeaseTTL is unset.
const DefaultLeaseTTL = 30 * time.Second

// DefaultConnectTimeout bounds pool connect during construction.
const DefaultConnectTimeout = 5 * time.Second

// DefaultFireTimeout bounds one tick's load/claim/dispatch when
// Options.FireTimeout is unset, so a hung DB cannot block the tick goroutine
// past Stop.
const DefaultFireTimeout = 30 * time.Second

// Options holds typed configuration for the postgres scheduler adapter.
// An empty DSN selects sqlite (Path, default ":memory:"); a set DSN opens
// postgres. Table defaults to DefaultTable; Owner identifies this replica's
// lease holder and defaults to a random value unique per driver.
//
// Scheduler behavior (Dispatcher, Locker, Logger, CloseTimeout) embeds
// scheduler.Options so container and Open flow through unchanged; pool
// knobs (DSN/Path/MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime)
// embed coredb.Options so pool tuning stays in one place.
type Options struct {
	// SchedulerOptions carries dispatcher, locker, logger, and timeouts.
	scheduler.Options
	// PoolOptions holds the shared DB pool configuration.
	PoolOptions coredb.Options
	// Table is the scheduler-slots table name. Default DefaultTable.
	Table string `json:"table" toml:"table" yaml:"table"`
	// Owner identifies this replica as a lease holder. Default random.
	Owner string `json:"owner" toml:"owner" yaml:"owner"`
	// LeaseTTL is the slot lease granted to a claim holder.
	LeaseTTL time.Duration `json:"lease_ttl" toml:"lease_ttl" yaml:"lease_ttl"`
	// FireTimeout bounds each tick's load/claim/dispatch against a hung DB.
	// Zero means DefaultFireTimeout; negative fails validation.
	FireTimeout time.Duration `json:"fire_timeout" toml:"fire_timeout" yaml:"fire_timeout"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if err := o.Options.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("postgres: %w", err))
	}

	if err := o.PoolOptions.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("postgres: %w", err))
	}

	if o.LeaseTTL < 0 {
		errs = append(errs, errors.New("postgres: lease_ttl must not be negative"))
	}

	if o.FireTimeout < 0 {
		errs = append(errs, scheduler.InvalidOptionsError{Reason: "fire_timeout must be >= 0"})
	}

	if o.Table != "" {
		if err := dbconn.ValidateTableName(o.Table); err != nil {
			errs = append(errs, fmt.Errorf("postgres: %w", err))
		}
	}

	return errors.Join(errs...)
}
