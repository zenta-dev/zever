package ai

import (
	"context"
	"fmt"
	"sync"
)

// AI defines the LLM facade contract for generation, streaming and embeddings.
type AI interface {
	Generate(ctx context.Context, model string, messages []Message, opts GenerateOptions) (Generation, error)
	Stream(ctx context.Context, model string, messages []Message, opts GenerateOptions) (<-chan StreamChunk, error)
	Embed(ctx context.Context, model string, inputs []string, opts EmbedOptions) ([][]float32, error)
	Close() error
}

// Factory creates an AI from the given Options.
type Factory func(opts Options) (AI, error)

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
		return &DuplicateAdapterError{Adapter: adapter}
	}

	factories[adapter] = factory

	return nil
}

// Open creates an AI for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (AI, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	a, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("ai: open %s: %w", adapter, err)
	}

	return a, nil
}
