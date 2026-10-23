package geo

import (
	"context"
)

// stubGeo is a minimal Geo implementation for facade tests.
type stubGeo struct{}

func (s *stubGeo) Geocode(_ context.Context, _ string) ([]Location, error) {
	return []Location{{Lat: 1, Lng: 2, Formatted: "stub"}}, nil
}

func (s *stubGeo) ReverseGeocode(_ context.Context, _, _ float64) ([]Address, error) {
	return []Address{{Formatted: "stub"}}, nil
}

func (s *stubGeo) Distance(_ context.Context, _, _ Point) (float64, error) {
	return 42, nil
}

func (s *stubGeo) Close() error {
	return nil
}

// NOTE: no t.Parallel anywhere in this file: registry is a global map.
