package google

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// TestEdgeDistance_coordBoundaries proves exact coordinate limits pass
// validation while out-of-range values fail closed.
func TestEdgeDistance_coordBoundaries(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"OK","rows":[{"elements":[{"status":"OK","distance":{"value":1}}]}]}`, 200)
	g := newGeoWithServer(t, srv)
	ctx := t.Context()

	if _, err := g.Distance(ctx, geo.Point{Lat: 90, Lng: 180}, geo.Point{Lat: -90, Lng: -180}); err != nil {
		t.Fatalf("Distance(max valid) err = %v, want nil", err)
	}

	for _, p := range []geo.Point{{Lat: 90.1}, {Lat: 0, Lng: -180.1}} {
		if _, err := g.Distance(ctx, p, geo.Point{}); !errors.Is(err, geo.ErrInvalidCoordinate) {
			t.Fatalf("Distance(%+v) err = %v, want ErrInvalidCoordinate", p, err)
		}
	}
}

// TestEdgeReverseGeocode_coordBoundaries proves exact coordinate limits are
// accepted and out-of-range values rejected before any request.
func TestEdgeReverseGeocode_coordBoundaries(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"OK","results":[]}`, 200)
	g := newGeoWithServer(t, srv)

	if _, err := g.ReverseGeocode(t.Context(), 90, 180); !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("ReverseGeocode(max valid) err = %v, want ErrNotFound", err)
	}

	if _, err := g.ReverseGeocode(t.Context(), 91, 0); !errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("ReverseGeocode(91) err = %v, want ErrInvalidCoordinate", err)
	}
}

// TestEdgeClose_idempotent proves Close is safe to call repeatedly.
func TestEdgeClose_idempotent(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{}`, 200)
	g := newGeoWithServer(t, srv)

	if err := g.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
}
