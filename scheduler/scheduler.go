package scheduler

import (
	"context"
	"fmt"
	"sync"
)

// EntryID identifies a registered schedule. Zero is invalid.
type EntryID uint64

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

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[adapter]; dup {
		return &DuplicateError{Adapter: adapter}
	}

	factories[adapter] = factory

	return nil
}

// Open creates a Scheduler for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Scheduler, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("scheduler: open %s: %w", adapter, err)
	}

	return s, nil
}
