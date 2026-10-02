// Package geotest provides the conformance kit third-party geo adapters run to prove backend parity.
package geotest

import (
	"errors"
	"math"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// Conformance verifies factory-built backends implement the geo.Geo
// contract: open/register round-trip, empty-query and invalid-
// coordinate sentinels, same-point distance, and Close. Each subtest
// takes a fresh instance from factory so cases stay isolated. Tests
// never call time.Sleep and never touch the network.
//
// Documented live-creds exemption: the google adapter needs an API
// key plus network, so its conformance test skips with a reason and
// the kit runs against the static adapter in its own package. The
// osm adapter runs hermetic against a loopback stub (empty-query
// probes map empty results to ErrNotFound; coordinate and distance
// checks never reach the wire). The kit never asserts successful
// lookups: result sets are dataset-dependent, so known-place
// coverage stays adapter-owned.
func Conformance(t *testing.T, factory func(t *testing.T) geo.Geo) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("EmptyQuery", func(t *testing.T) { conformanceEmptyQuery(t, factory) })
	t.Run("InvalidCoordinate", func(t *testing.T) { conformanceInvalidCoordinate(t, factory) })
	t.Run("SamePointDistance", func(t *testing.T) { conformanceSamePointDistance(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := geo.Open(geo.Adapter("conformance-missing-adapter"), geo.Options{}); !errors.Is(err, geo.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := geo.Adapter("conformance-probe-geo")

	if err := geo.Register(probe, nil); !errors.Is(err, geo.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(geo.Options) (geo.Geo, error) {
		return nil, errors.New("geotest: probe factory must not run")
	}

	_ = geo.Register(probe, stub)

	if err := geo.Register(probe, stub); !errors.Is(err, geo.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceEmptyQuery(t *testing.T, factory func(t *testing.T) geo.Geo) {
	t.Helper()

	ctx := t.Context()
	g := factory(t)

	if _, err := g.Geocode(ctx, ""); !errors.Is(err, geo.ErrNotFound) {
		t.Errorf("Geocode(empty) err = %v, want ErrNotFound", err)
	}

	if _, err := g.Geocode(ctx, "   "); !errors.Is(err, geo.ErrNotFound) {
		t.Errorf("Geocode(blank) err = %v, want ErrNotFound", err)
	}
}

func conformanceInvalidCoordinate(t *testing.T, factory func(t *testing.T) geo.Geo) {
	t.Helper()

	ctx := t.Context()
	g := factory(t)

	for _, tc := range []struct {
		name string
		lat  float64
		lng  float64
	}{
		{"nan", math.NaN(), 0},
		{"out-of-range-lat", 91, 0},
		{"out-of-range-lng", 0, 181},
	} {
		if _, err := g.ReverseGeocode(ctx, tc.lat, tc.lng); !errors.Is(err, geo.ErrInvalidCoordinate) {
			t.Errorf("ReverseGeocode(%s) err = %v, want ErrInvalidCoordinate", tc.name, err)
		}

		if _, err := g.Distance(ctx, geo.Point{Lat: tc.lat, Lng: tc.lng}, geo.Point{}); !errors.Is(err, geo.ErrInvalidCoordinate) {
			t.Errorf("Distance(%s) err = %v, want ErrInvalidCoordinate", tc.name, err)
		}
	}
}

func conformanceSamePointDistance(t *testing.T, factory func(t *testing.T) geo.Geo) {
	t.Helper()

	ctx := t.Context()
	g := factory(t)

	got, err := g.Distance(ctx, geo.Point{Lat: 48.8566, Lng: 2.3522}, geo.Point{Lat: 48.8566, Lng: 2.3522})
	if err != nil {
		t.Fatalf("Distance(same) error = %v", err)
	}

	if got < 0 || got > 1e-6 {
		t.Errorf("Distance(same) = %v, want ~0 meters", got)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) geo.Geo) {
	t.Helper()

	g := factory(t)

	if err := g.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
