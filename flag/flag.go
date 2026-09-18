package flag

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
)

// Flag defines the core operations for evaluating feature flags.
//
// Semantics: a missing key returns (fallback, nil). A type mismatch
// returns (fallback, non-nil error). Errors report infrastructure or
// type failures only; absence of a key is not an error.
// Type-mismatch errors are adapter-local fmt.Errorf values with key
// context, e.g. `flag: static: key %q is not a bool`.
type Flag interface {
	// Bool evaluates key as a bool.
	Bool(ctx context.Context, key string, fallback bool) (bool, error)
	// String evaluates key as a string.
	String(ctx context.Context, key string, fallback string) (string, error)
	// Int evaluates key as an int.
	Int(ctx context.Context, key string, fallback int) (int, error)
	// JSON decodes key into out, falling back to fallback on missing key.
	JSON(ctx context.Context, key string, out any, fallback any) error
	// Close shuts down the flag client and releases associated resources.
	Close() error
}

// EvalContext carries per-evaluation data such as targeting signals.
type EvalContext struct {
	// RandomizationID is the stable ID used for percentage rollouts.
	RandomizationID string
	// Signals carries arbitrary targeting attributes.
	Signals map[string]any
}

type evalKey struct{}

// WithEvalContext attaches ec to ctx.
func WithEvalContext(ctx context.Context, ec EvalContext) context.Context {
	return context.WithValue(ctx, evalKey{}, ec)
}

// EvalContextFrom extracts the EvalContext from ctx.
// It returns false when no evaluation context is attached.
func EvalContextFrom(ctx context.Context) (EvalContext, bool) {
	ec, ok := ctx.Value(evalKey{}).(EvalContext)
	return ec, ok
}

// Factory creates a Flag from the given Options.
type Factory func(opts Options) (Flag, error)

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

// Open creates a Flag for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Flag, error) {
	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	f, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("flag: open %s: %w", adapter, err)
	}

	return f, nil
}
