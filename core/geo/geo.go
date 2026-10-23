package geo

import (
	"context"
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
