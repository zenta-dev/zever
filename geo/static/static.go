package static

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/zenta-dev/zever/geo"
)

type city struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

type staticGeo struct {
	cities []city
}

// New creates a static geo adapter from typed options.
func New(opts geo.Options) (geo.Geo, error) {
	path := opts.Path
	if path == "" {
		path = "cities.json"
	}

	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == "" {
		return nil, fmt.Errorf("geo: static: invalid path %q: %w", path, geo.ErrInvalidOptions)
	}

	data, err := os.ReadFile(cleaned) //nolint:gosec // G304: path is configuration value, not user input
	if err != nil {
		return nil, fmt.Errorf("geo: static: read cities: %w", err)
	}

	var cities []city
	if err := json.Unmarshal(data, &cities); err != nil {
		return nil, fmt.Errorf("geo: static: parse cities: %w", err)
	}

	return &staticGeo{cities: cities}, nil
}

func (s *staticGeo) Geocode(_ context.Context, address string) ([]geo.Location, error) {
	query := strings.ToLower(strings.TrimSpace(address))
	if query == "" {
		return nil, fmt.Errorf("geo: static: empty query: %w", geo.ErrNotFound)
	}

	var exact, prefix []geo.Location

	for _, c := range s.cities {
		name := strings.ToLower(c.Name)

		switch {
		case name == query:
			exact = append(exact, geo.Location{Lat: c.Lat, Lng: c.Lng, Formatted: c.Name})
		case strings.HasPrefix(name, query):
			prefix = append(prefix, geo.Location{Lat: c.Lat, Lng: c.Lng, Formatted: c.Name})
		}
	}

	results := exact

	if len(results) == 0 {
		results = prefix
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("geo: static: no match for %q: %w", address, geo.ErrNotFound)
	}

	return results, nil
}

func (s *staticGeo) ReverseGeocode(_ context.Context, lat float64, lng float64) ([]geo.Address, error) {
	if !geo.ValidCoord(lat, lng) {
		return nil, fmt.Errorf("geo: static: invalid coordinates: %w", geo.ErrInvalidCoordinate)
	}

	var results []geo.Address

	for _, c := range s.cities {
		if haversine(c.Lat, c.Lng, lat, lng) < 1.5 {
			results = append(results, geo.Address{
				Formatted:  c.Name,
				Components: map[string]string{"city": c.Name},
			})
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("geo: static: no match for (%.4f, %.4f): %w", lat, lng, geo.ErrNotFound)
	}

	return results, nil
}

func (s *staticGeo) Distance(_ context.Context, from geo.Point, to geo.Point) (float64, error) {
	if !geo.ValidCoord(from.Lat, from.Lng) || !geo.ValidCoord(to.Lat, to.Lng) {
		return 0, fmt.Errorf("geo: static: invalid coordinates: %w", geo.ErrInvalidCoordinate)
	}

	return 1000 * haversine(from.Lat, from.Lng, to.Lat, to.Lng), nil
}

func (s *staticGeo) Close() error { return nil }

func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0

	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	// a is a sum of squares so it can never be negative; only the upper
	// clamp is reachable (float rounding can push antipodal pairs past 1).
	if a > 1 {
		a = 1
	}

	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
