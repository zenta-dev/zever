package google

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// benchServer serves fixed Geocoding and Distance Matrix responses on loopback.
func benchServer(b *testing.B) *httptest.Server {
	b.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/maps/api/distancematrix/json":
			_, _ = w.Write([]byte(`{"status":"OK","rows":[{"elements":[{"status":"OK",` +
				`"distance":{"value":343000,"text":"343 km"},"duration":{"value":12345,"text":"3 hours"}}]}]}`))
		default:
			_, _ = w.Write([]byte(`{"status":"OK","results":[{"formatted_address":"Paris, France",` +
				`"geometry":{"location":{"lat":48.8566,"lng":2.3522}}}]}`))
		}
	}))

	b.Cleanup(srv.Close)

	return srv
}

// benchGeo builds a Google adapter pointed at an in-process stub server.
func benchGeo(b *testing.B) geo.Geo {
	b.Helper()

	srv := benchServer(b)

	g, err := New(geo.Options{APIKey: "bench-key", BaseURL: srv.URL, AllowInsecure: true})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = g.Close() })

	return g
}

// BenchmarkGeocode measures the request, decode, and mapping path.
func BenchmarkGeocode(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := g.Geocode(ctx, "Paris"); err != nil {
			b.Fatalf("Geocode: %v", err)
		}
	}
}

// BenchmarkDistance measures the Distance Matrix request and decode path.
func BenchmarkDistance(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()
	from := geo.Point{Lat: 51.5074, Lng: -0.1278}
	to := geo.Point{Lat: 48.8566, Lng: 2.3522}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := g.Distance(ctx, from, to); err != nil {
			b.Fatalf("Distance: %v", err)
		}
	}
}
