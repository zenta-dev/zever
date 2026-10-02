package osm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/geo/geotest"
)

// TestOSMConformance proves the osm adapter honors the geo.Geo
// contract via the shared conformance kit. Nominatim is faked over
// loopback httptest (no external network): /search answers an empty
// result set so empty-query probes map to ErrNotFound, while invalid
// coordinates and same-point distance never reach the wire.
func TestOSMConformance(t *testing.T) {
	t.Parallel()

	geotest.Conformance(t, func(t *testing.T) geo.Geo {
		t.Helper()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(srv.Close)

		g, err := New(geo.Options{
			UserAgent:     "kit-conformance",
			Endpoint:      srv.URL,
			AllowInsecure: true,
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = g.Close() })

		return g
	})
}
