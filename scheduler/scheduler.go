package scheduler

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
	"github.com/zenta-dev/zever/job"
)

// EntryID identifies a registered schedule. Zero is invalid.
// It aliases job.EntryID: the scheduler package is a thin facade and
// shares the same cron entry ID space as job.Scheduler.
type EntryID = job.EntryID

// Scheduler registers jobs on cron specs and fires them through a dispatcher.
type Scheduler interface {
	// Schedule registers jobName with args on cron spec and returns its entry ID.
	Schedule(ctx context.Context, spec, jobName string, args any) (EntryID, error)
	// Remove unregisters the schedule with the given ID.
	// An unknown ID is a no-op returning nil.
	Remove(id EntryID) error
	// Entries returns a snapshot of the live entry IDs.
	Entries() []EntryID
	// Start begins cron ticks. It is idempotent and non-blocking.
	Start() error
	// Stop ends cron ticks, waiting for running ticks up to CloseTimeout.
	Stop() error
	// Name returns the adapter name for the scheduler.
	Name() string
}

// Factory creates a Scheduler from the given Options.
type Factory func(opts Options) (Scheduler, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Scheduler for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Scheduler, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("scheduler: open %s: %w", adapter, err)
	}

	return s, nil
}
