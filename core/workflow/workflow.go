package workflow

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
)

// RunID uniquely identifies a workflow run.
type RunID string

// Workflow defines the core operations for interacting with a workflow engine.
// It handles run lifecycle, signaling, querying, and shutdown.
type Workflow interface {
	// Start begins a new run of the named workflow with input and workflowID.
	//
	// Start input must be JSON-marshalable. The value is stored as-is without
	// marshaling, so a non-marshalable input does not fail at Start time; it
	// fails lazily at Query time when the stored state is marshaled with
	// json.Marshal.
	Start(ctx context.Context, name string, input any, workflowID string) (RunID, error)

	// Signal delivers a signal with value to the named channel of runID.
	//
	// Signal value must be JSON-marshalable. The value is stored as-is without
	// marshaling, so a non-marshalable value does not fail at Signal time; it
	// fails lazily at Query time when the stored state is marshaled with
	// json.Marshal.
	Signal(ctx context.Context, runID RunID, name string, value any) error

	// Query evaluates the named query against runID and decodes into out.
	//
	// Query state and out must be JSON-marshalable. Query marshals the stored
	// state with json.Marshal, so a non-marshalable input, signal value, or
	// step result stored earlier fails here, not at Start or Signal time.
	Query(ctx context.Context, runID RunID, name string, out any) error

	// Cancel requests cancellation of runID.
	Cancel(ctx context.Context, runID RunID) error

	// Close shuts down the workflow client and releases associated resources.
	Close() error
}

// Factory creates a Workflow from the given Options.
type Factory func(opts Options) (Workflow, error)

// StepFunc is a named workflow step: input in, output or error out.
//
// StepFunc input and output must be JSON-marshalable. The output becomes
// queryable state, so a non-marshalable result does not fail when the step
// returns; it fails lazily at Query time when the state is marshaled with
// json.Marshal.
// Engines that support host-registered steps expose registration through
// StepRegistrar rather than a concrete adapter type.
type StepFunc func(ctx context.Context, input any) (any, error)

// StepRegistrar is implemented by workflow engines that allow hosts to
// register step functions at runtime. It is a separate interface, not part
// of Workflow, so engines without the concept are unaffected and no
// existing implementer breaks.
type StepRegistrar interface {
	// RegisterStep registers fn under name, replacing any prior step.
	RegisterStep(name string, fn StepFunc)
}

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(a Adapter) error { return &DuplicateError{Adapter: a} },
	func(a Adapter) error { return &UnknownAdapterError{Adapter: a} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Workflow for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Workflow, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	w, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("workflow: open %s: %w", adapter, err)
	}

	return w, nil
}
