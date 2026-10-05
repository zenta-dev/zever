package static

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// TestEdgeEmptyCatalog checks behavior over an empty cities catalog.
func TestEdgeEmptyCatalog(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cities.json")
	if err := os.WriteFile(path, []byte(`[]`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	g, err := New(geo.Options{Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = g.Close() })

	ctx := t.Context()

	if _, err := g.Geocode(ctx, "London"); !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("Geocode(empty catalog) = %v, want ErrNotFound", err)
	}

	if _, err := g.ReverseGeocode(ctx, 51.5, -0.1); !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("ReverseGeocode(empty catalog) = %v, want ErrNotFound", err)
	}

	if _, err := g.Distance(ctx, geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 1, Lng: 1}); err != nil {
		t.Fatalf("Distance(empty catalog) = %v, want nil", err)
	}
}

// TestEdgeDistanceSamePoint checks the zero-distance boundary.
func TestEdgeDistanceSamePoint(t *testing.T) {
	t.Parallel()

	g := mustOpenStatic(t)
	t.Cleanup(func() { _ = g.Close() })

	got, err := g.Distance(t.Context(), geo.Point{Lat: 1, Lng: 2}, geo.Point{Lat: 1, Lng: 2})
	if err != nil {
		t.Fatalf("Distance() error = %v", err)
	}

	if got != 0 {
		t.Fatalf("Distance(same point) = %v, want 0", got)
	}
}

// TestEdgeHaversineIdentical checks the zero kernel.
func TestEdgeHaversineIdentical(t *testing.T) {
	t.Parallel()

	if got := haversine(12.34, 56.78, 12.34, 56.78); got != 0 {
		t.Fatalf("haversine(same) = %v, want 0", got)
	}
}

// TestEdgeConcurrentReads exercises the immutable catalog concurrently.
func TestEdgeConcurrentReads(t *testing.T) {
	t.Parallel()

	g := mustOpenStatic(t)
	t.Cleanup(func() { _ = g.Close() })

	ctx := t.Context()

	var wg sync.WaitGroup

	for range 16 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 50 {
				if _, err := g.Geocode(ctx, "London"); err != nil {
					t.Errorf("Geocode() error = %v", err)
					return
				}

				if _, err := g.ReverseGeocode(ctx, 51.5074, -0.1278); err != nil {
					t.Errorf("ReverseGeocode() error = %v", err)
					return
				}

				if _, err := g.Distance(ctx, geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 1, Lng: 1}); err != nil {
					t.Errorf("Distance() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
