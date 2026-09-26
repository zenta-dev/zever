package geo

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
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

// Factory creates a Geo from the given Options.
type Factory func(opts Options) (Geo, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateAdapterError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}
	return factories.Register(adapter, factory)
}

// Open creates a Geo for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Geo, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}
	c, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("geo: open %s: %w", adapter, err)
	}
	return c, nil
}
