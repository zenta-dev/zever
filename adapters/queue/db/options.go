package db

import (
	"errors"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// DefaultTable is the queue-messages table created when Options.Table is empty.
const DefaultTable = "queue_messages"

// DefaultVisibilityTimeout is the claim lease granted to a pop when
// Options.VisibilityTimeout is unset.
const DefaultVisibilityTimeout = 30 * time.Second

// DefaultPollTimeout is the Pop wait for a message when Options.PollTimeout
// is unset.
const DefaultPollTimeout = 5 * time.Second

// DefaultPollInterval is the Pop retry cadence when Options.PollInterval is
// unset. It stays well under the conformance kit's short poll timeout so
// empty-queue cases stay fast.
const DefaultPollInterval = 10 * time.Millisecond

// DefaultReclaimBatch caps how many stale claims one reclaim sweep
// releases when Options.ReclaimBatch is unset.
const DefaultReclaimBatch = 100

// DefaultConnectTimeout bounds pool connect during construction.
const DefaultConnectTimeout = 5 * time.Second

// Options holds typed configuration for the DB-backed queue adapter. An
// empty DSN selects sqlite (Path, default ":memory:"); a set DSN opens
// postgres. Table defaults to DefaultTable; Owner identifies this
// instance's claims and defaults to a random value unique per driver.
// VisibilityTimeout is the claim lease, PollTimeout bounds Pop,
// PollInterval is the Pop retry cadence, ReclaimBatch caps one reclaim
// sweep, and Buffer caps buffered ready messages per topic (<= 0 means
// unbounded, matching the redis adapter).
//
// Pool knobs (DSN/Path/MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime)
// are embedded from coredb.Options so pool tuning stays in one place;
// core/db owns required-field checks for DSN/Path.
type Options struct {
	// Options holds the shared DB pool configuration.
	coredb.Options
	// Table is the queue-messages table name. Default DefaultTable.
	Table string `json:"table" toml:"table" yaml:"table"`
	// Owner identifies this instance as a claim holder. Default random.
	Owner string `json:"owner" toml:"owner" yaml:"owner"`
	// VisibilityTimeout is the duration a popped message stays invisible.
	VisibilityTimeout time.Duration `json:"visibility_timeout" toml:"visibility_timeout" yaml:"visibility_timeout"`
	// PollTimeout is the duration Pop waits for a message before empty.
	PollTimeout time.Duration `json:"poll_timeout" toml:"poll_timeout" yaml:"poll_timeout"`
	// PollInterval is the cadence Pop retries claim while waiting.
	PollInterval time.Duration `json:"poll_interval" toml:"poll_interval" yaml:"poll_interval"`
	// ReclaimBatch caps how many stale claims one sweep releases.
	ReclaimBatch int `json:"reclaim_batch" toml:"reclaim_batch" yaml:"reclaim_batch"`
	// Buffer caps buffered ready messages per topic. <= 0 is unbounded.
	Buffer int `json:"buffer" toml:"buffer" yaml:"buffer"`
	// Provider emits the producer span opened for every push. Nil disables
	// telemetry. It is Go-API-only: never decoded from configuration files.
	Provider observability.Provider `json:"-" toml:"-" yaml:"-"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if err := o.Options.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("db: %w", err))
	}

	if o.VisibilityTimeout < 0 {
		errs = append(errs, errors.New("db: visibility_timeout must not be negative"))
	}

	if o.PollTimeout < 0 {
		errs = append(errs, errors.New("db: poll_timeout must not be negative"))
	}

	if o.PollInterval < 0 {
		errs = append(errs, errors.New("db: poll_interval must not be negative"))
	}

	if o.ReclaimBatch < 0 {
		errs = append(errs, errors.New("db: reclaim_batch must not be negative"))
	}

	if o.Buffer < 0 {
		errs = append(errs, errors.New("db: buffer must not be negative"))
	}

	if o.Table != "" {
		if err := dbconn.ValidateTableName(o.Table); err != nil {
			errs = append(errs, fmt.Errorf("db: %w", err))
		}
	}

	return errors.Join(errs...)
}
