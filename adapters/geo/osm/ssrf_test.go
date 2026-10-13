package osm

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/shared/httpclient"
)

// stubServer serves an empty search result and counts hits.
func stubServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestNew_BlocksPrivateByDefault verifies a loopback endpoint is refused
// unless allow_private opts in, without contacting it.
func TestNew_BlocksPrivateByDefault(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubServer(t, &hits)

	g, err := New(geo.Options{Endpoint: srv.URL, UserAgent: "ssrf/1.0", AllowInsecure: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = g.Close() }()

	if _, err := g.Geocode(t.Context(), "x"); !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("Geocode() error = %v, want ErrPrivateAddress", err)
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("server hits = %d, want 0", got)
	}
}

// TestNew_AllowPrivateOptsIn verifies the allow_private opt-in reaches a
// loopback endpoint.
func TestNew_AllowPrivateOptsIn(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubServer(t, &hits)

	g, err := New(geo.Options{Endpoint: srv.URL, UserAgent: "ssrf/1.0", AllowInsecure: true, AllowPrivate: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = g.Close() }()

	if _, err := g.Geocode(t.Context(), "x"); !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("Geocode() error = %v, want ErrNotFound (empty result reached)", err)
	}

	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}
