package cdc

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zenta-dev/zever/core/outbox"
)

const (
	// DefaultPrefix is the logical-replication message prefix used when
	// Options.Prefix is empty.
	DefaultPrefix = "zever_outbox"
	// DefaultSlot is the replication slot created when Options.Slot is empty.
	DefaultSlot = "zever_outbox_slot"
	// DefaultPublication is the publication named in the pgoutput plugin args
	// when Options.Publication is empty.
	DefaultPublication = "zever_outbox_pub"
	// DefaultConnectTimeout bounds the replication connection during Start.
	DefaultConnectTimeout = 10 * time.Second
	// DefaultAckInterval is the minimum gap between StandbyStatusUpdate acks
	// when no keepalive requested one.
	DefaultAckInterval = 5 * time.Second
)

// Options holds typed configuration for the CDC outbox adapter. The embedded
// outbox.Options carries the shared fields (DSN, Prefix, Slot, Publication,
// Retry, MaxAttempts); Publisher is the transport the consumer publishes
// through and is required to Start. Record does not need it.
type Options struct {
	// Options holds the shared outbox configuration.
	outbox.Options
	// Publisher receives replicated messages. Required to Start the consumer.
	Publisher outbox.Publisher `json:"-" toml:"-" yaml:"-"`
}

// Validate checks options for consistency, joining all violations. The DSN,
// Slot, Publication, and Prefix must be non-empty. Publisher is required only
// when Start is called, not here.
func (o Options) Validate() error {
	var errs []error

	if err := o.Options.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("cdc: %w", err))
	}

	if strings.TrimSpace(o.DSN) == "" {
		errs = append(errs, errors.New("cdc: dsn must not be empty"))
	}

	if strings.TrimSpace(o.Slot) == "" {
		errs = append(errs, errors.New("cdc: slot must not be empty"))
	}

	if strings.TrimSpace(o.Publication) == "" {
		errs = append(errs, errors.New("cdc: publication must not be empty"))
	}

	if strings.TrimSpace(o.Prefix) == "" {
		errs = append(errs, errors.New("cdc: prefix must not be empty"))
	}

	return errors.Join(errs...)
}
