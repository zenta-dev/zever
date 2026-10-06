package outbox

import (
	"errors"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/shared/retry"
)

// ScopeName is the tracer and meter scope for outbox relay telemetry.
const ScopeName = "outbox"

const (
	// DefaultTable is the outbox table name when Options.Table is empty.
	DefaultTable = "outbox"
	// DefaultInboxTable is the inbox table name when Options.InboxTable is empty.
	DefaultInboxTable = "inbox"
	// DefaultPollInterval is the relay poll cadence when Options.PollInterval
	// is unset.
	DefaultPollInterval = time.Second
	// DefaultBatchSize caps how many messages one relay poll claims.
	DefaultBatchSize = 100
	// DefaultMaxAttempts is the publish attempt cap before a message is
	// marked failed.
	DefaultMaxAttempts = 5
	// DefaultRetention is how long processed messages are kept before cleanup.
	DefaultRetention = 168 * time.Hour
	// DefaultLockSeconds is the claim lease granted while publishing.
	DefaultLockSeconds = 30
)

// Publisher names accepted by Options.Publisher. The value is informational:
// it records which transport the application wired to the relay's Publisher.
const (
	// PublisherEventBus selects the eventbus bridge.
	PublisherEventBus = "eventbus"
	// PublisherQueue selects the queue bridge.
	PublisherQueue = "queue"
)

// Options configures outbox construction. Fields form a union of all adapter
// options; each adapter consumes only the fields it needs.
type Options struct {
	// DSN is the postgres connection string or sqlite path for the db
	// adapter. Empty selects a private in-memory database.
	DSN string `json:"dsn" toml:"dsn" yaml:"dsn"`
	// Table is the outbox table name. Empty means DefaultTable.
	Table string `json:"table" toml:"table" yaml:"table"`
	// InboxTable is the inbox table name. Empty means DefaultInboxTable.
	InboxTable string `json:"inbox_table" toml:"inbox_table" yaml:"inbox_table"`
	// Publisher records which transport the relay publishes through:
	// "eventbus", "queue", or "" (unset). Informational only.
	Publisher string `json:"publisher" toml:"publisher" yaml:"publisher"`
	// PollInterval is the relay poll cadence. Zero means DefaultPollInterval.
	PollInterval time.Duration `json:"poll_interval" toml:"poll_interval" yaml:"poll_interval"`
	// BatchSize caps how many messages one poll claims. Zero means
	// DefaultBatchSize.
	BatchSize int `json:"batch_size" toml:"batch_size" yaml:"batch_size"`
	// MaxAttempts is the publish attempt cap. Zero means DefaultMaxAttempts.
	MaxAttempts int `json:"max_attempts" toml:"max_attempts" yaml:"max_attempts"`
	// Retry is the per-attempt backoff policy applied after a publish failure.
	Retry retry.Policy `json:"retry" toml:"retry" yaml:"retry"`
	// Retention is how long processed messages are kept. Zero means
	// DefaultRetention.
	Retention time.Duration `json:"retention" toml:"retention" yaml:"retention"`
	// LockSeconds is the claim lease in seconds. Zero means DefaultLockSeconds.
	LockSeconds int `json:"lock_seconds" toml:"lock_seconds" yaml:"lock_seconds"`
	// NotifyChannel is the postgres LISTEN/NOTIFY channel (cdc adapter).
	NotifyChannel string `json:"notify_channel" toml:"notify_channel" yaml:"notify_channel"`
	// Prefix is the logical-replication message prefix (cdc adapter).
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
	// Slot is the logical-replication slot name (cdc adapter).
	Slot string `json:"slot" toml:"slot" yaml:"slot"`
	// Publication is the logical-replication publication name (cdc adapter).
	Publication string `json:"publication" toml:"publication" yaml:"publication"`
	// DedicatedPool opts out of container-level pool sharing. Default false
	// shares one pool per exact DSN; true opens a private pool.
	DedicatedPool bool `json:"dedicated_pool" toml:"dedicated_pool" yaml:"dedicated_pool"`
	// StallReadiness opts the relay into the container readiness aggregate: while
	// Status().Stalled is true, container.Ready reports the process as not ready
	// so orchestrators stop routing traffic to an instance whose messages are
	// backing up. Default false keeps readiness purely dependency-based; the
	// container reads this flag, adapters ignore it.
	StallReadiness bool `json:"stall_readiness" toml:"stall_readiness" yaml:"stall_readiness"`
	// Provider emits relay metrics and spans. Nil disables telemetry. It is
	// Go-API-only: never decoded from configuration files.
	Provider observability.Provider `json:"-" toml:"-" yaml:"-"`
}

// Validate checks options for consistency, joining all violations.
// Zero durations and counts mean "apply the adapter default" and are valid;
// only negative values fail.
func (o Options) Validate() error {
	var errs []error

	if o.Table != "" && !validIdentifier(o.Table) {
		errs = append(errs, InvalidOptionsError{Reason: fmt.Sprintf("invalid table name %q", o.Table)})
	}

	if o.InboxTable != "" && !validIdentifier(o.InboxTable) {
		errs = append(errs, InvalidOptionsError{Reason: fmt.Sprintf("invalid inbox_table name %q", o.InboxTable)})
	}

	if o.PollInterval < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "poll_interval must not be negative"})
	}

	if o.BatchSize < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "batch_size must not be negative"})
	}

	if o.MaxAttempts < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "max_attempts must not be negative"})
	}

	if o.Retention < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "retention must not be negative"})
	}

	if o.LockSeconds < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "lock_seconds must not be negative"})
	}

	switch o.Publisher {
	case "", PublisherEventBus, PublisherQueue:
	default:
		errs = append(errs, InvalidOptionsError{
			Reason: fmt.Sprintf("publisher must be %q, %q, or empty, got %q", PublisherEventBus, PublisherQueue, o.Publisher),
		})
	}

	return errors.Join(errs...)
}

// validIdentifier reports whether s matches [A-Za-z_][A-Za-z0-9_]*. Table
// names are interpolated into DDL, so core rejects unsafe names before any
// adapter sees them; adapters re-validate via shared/dbconn.
func validIdentifier(s string) bool {
	if s == "" {
		return false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')

		if !ok {
			return false
		}
	}

	return true
}
