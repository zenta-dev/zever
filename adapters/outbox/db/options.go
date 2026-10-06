package db

import (
	"errors"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/shared/dbconn"
	"github.com/zenta-dev/zever/shared/retry"
)

const (
	// DefaultTable is the outbox table created when Options.Table is empty.
	DefaultTable = outbox.DefaultTable
	// DefaultInboxTable is the inbox table created when Options.InboxTable
	// is empty.
	DefaultInboxTable = outbox.DefaultInboxTable
	// DefaultPollInterval is the relay poll cadence when Options.PollInterval
	// is unset.
	DefaultPollInterval = outbox.DefaultPollInterval
	// DefaultBatchSize caps how many messages one poll claims.
	DefaultBatchSize = outbox.DefaultBatchSize
	// DefaultMaxAttempts is the publish attempt cap before DLQ.
	DefaultMaxAttempts = outbox.DefaultMaxAttempts
	// DefaultRetention is how long processed messages are kept.
	DefaultRetention = outbox.DefaultRetention
	// DefaultLockSeconds is the claim lease granted while publishing.
	DefaultLockSeconds = outbox.DefaultLockSeconds
	// DefaultStallAfter is how long the oldest pending message may wait
	// before Status reports Stalled.
	DefaultStallAfter = 5 * time.Minute
	// DefaultConnectTimeout bounds pool connect during construction.
	DefaultConnectTimeout = 5 * time.Second
	// DefaultOperationTimeout bounds one Status query round trip.
	DefaultOperationTimeout = 5 * time.Second
)

// Options holds typed configuration for the DB-backed outbox adapter. An
// empty DSN selects sqlite (Path, default ":memory:"); a set DSN opens
// postgres. Table/InboxTable default to DefaultTable/DefaultInboxTable.
// Publisher is the transport the relay publishes through; core outbox.Options
// carries only the informational string selector, so wiring supplies the
// concrete Publisher here.
//
// Pool knobs (DSN/Path/MaxConns/MinConns/MaxConnLifetime/MaxConnIdleTime)
// are embedded from coredb.Options so pool tuning stays in one place;
// core/db owns required-field checks for DSN/Path.
type Options struct {
	// Options holds the shared DB pool configuration.
	coredb.Options
	// Table is the outbox table name. Default DefaultTable.
	Table string `json:"table" toml:"table" yaml:"table"`
	// InboxTable is the inbox table name. Default DefaultInboxTable.
	InboxTable string `json:"inbox_table" toml:"inbox_table" yaml:"inbox_table"`
	// Publisher receives recorded messages. Required to Start the relay.
	Publisher outbox.Publisher `json:"-" toml:"-" yaml:"-"`
	// PollInterval is the relay poll cadence. Default DefaultPollInterval.
	PollInterval time.Duration `json:"poll_interval" toml:"poll_interval" yaml:"poll_interval"`
	// BatchSize caps how many messages one poll claims. Default DefaultBatchSize.
	BatchSize int `json:"batch_size" toml:"batch_size" yaml:"batch_size"`
	// MaxAttempts is the publish attempt cap before DLQ. Default DefaultMaxAttempts.
	MaxAttempts int `json:"max_attempts" toml:"max_attempts" yaml:"max_attempts"`
	// Retry is the per-attempt backoff applied after a publish failure.
	Retry retry.Policy `json:"retry" toml:"retry" yaml:"retry"`
	// Retention is how long processed messages are kept. Default DefaultRetention.
	Retention time.Duration `json:"retention" toml:"retention" yaml:"retention"`
	// LockSeconds is the claim lease in seconds. Default DefaultLockSeconds.
	LockSeconds int `json:"lock_seconds" toml:"lock_seconds" yaml:"lock_seconds"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if err := o.Options.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("db: %w", err))
	}

	if o.PollInterval < 0 {
		errs = append(errs, errors.New("db: poll_interval must not be negative"))
	}

	if o.BatchSize < 0 {
		errs = append(errs, errors.New("db: batch_size must not be negative"))
	}

	if o.MaxAttempts < 0 {
		errs = append(errs, errors.New("db: max_attempts must not be negative"))
	}

	if o.Retention < 0 {
		errs = append(errs, errors.New("db: retention must not be negative"))
	}

	if o.LockSeconds < 0 {
		errs = append(errs, errors.New("db: lock_seconds must not be negative"))
	}

	if o.Table != "" {
		if err := dbconn.ValidateTableName(o.Table); err != nil {
			errs = append(errs, fmt.Errorf("db: %w", err))
		}
	}

	if o.InboxTable != "" {
		if err := dbconn.ValidateTableName(o.InboxTable); err != nil {
			errs = append(errs, fmt.Errorf("db: %w", err))
		}
	}

	return errors.Join(errs...)
}
