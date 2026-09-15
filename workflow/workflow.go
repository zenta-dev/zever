package workflow

import (
	"context"
	"fmt"
	"sync"
)

// RunID uniquely identifies a workflow run.
type RunID string

// Workflow defines the core operations for interacting with a workflow engine.
// It handles run lifecycle, signaling, querying, and shutdown.
type Workflow interface {
	// Start begins a new run of the named workflow with input and workflowID.
	Start(ctx context.Context, name string, input any, workflowID string) (RunID, error)

	// Signal delivers a signal with value to the named channel of runID.
	Signal(ctx context.Context, runID RunID, name string, value any) error

	// Query evaluates the named query against runID and decodes into out.
	Query(ctx context.Context, runID RunID, name string, out any) error

	// Cancel requests cancellation of runID.
	Cancel(ctx context.Context, runID RunID) error

	// Close shuts down the workflow client and releases associated resources.
	Close() error
}

// Factory creates a Workflow from the given Options.
type Factory func(opts Options) (Workflow, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(a Adapter, f Factory) error {
	if f == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, a)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[a]; dup {
		return &DuplicateError{Adapter: a}
	}

	factories[a] = f

	return nil
}

// Open creates a Workflow for adapter using the registered Factory and opts.
func Open(a Adapter, opts Options) (Workflow, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[a]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: a}
	}

	w, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("workflow: open %s: %w", a, err)
	}

	return w, nil
}
