package geo

import (
	"context"
	"fmt"
	"sync"
)

// Geo is the interface that geo adapters must implement.
type Geo interface {
	// Geocode resolves address to locations.
	Geocode(ctx context.Context, address string) ([]Location, error)
	// ReverseGeocode resolves coordinates to addresses.
	ReverseGeocode(ctx context.Context, lat, lng float64) ([]Address, error)
	// Distance returns driving or great-circle distance in meters.
	Distance(ctx context.Context, from, to Point) (float64, error)
	// Close releases resources held by the adapter.
	Close() error
}

// Factory creates a Geo from typed options.
type Factory func(opts Options) (Geo, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register makes an adapter available.
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

// Open opens a Geo using the named, already-registered adapter.
func Open(adapter Adapter, opts Options) (Geo, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("geo: unknown adapter %q (forgotten import?): %w", adapter, &UnknownAdapterError{Adapter: adapter})
	}
	c, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("geo: open %s: %w", adapter, err)
	}
	return c, nil
}
