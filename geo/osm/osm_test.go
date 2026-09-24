package osm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/geo"
)

func TestNew_RequiresUserAgent(t *testing.T) {
	t.Parallel()
	_, err := New(geo.Options{Endpoint: defaultEndpoint})
	if !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
	if !strings.Contains(err.Error(), "user_agent") {
		t.Fatalf("error missing user_agent: %v", err)
	}
}

func TestNew_InvalidEndpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		ep    string
		allow bool
	}{
		{"missing scheme", "://bad", false},
		{"no host", "https://", false},
		{"http blocked", "http://example.com", false},
		{"unknown scheme", "ftp://example.com", false},
		{"empty host http", "http://", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(geo.Options{Endpoint: tc.ep, UserAgent: "test-app/1.0", AllowInsecure: tc.allow})
			if !errors.Is(err, geo.ErrInvalidOptions) {
				t.Fatalf("expected ErrInvalidOptions for %q, got %v", tc.ep, err)
			}
		})
	}
}

func TestNew_Defaults(t *testing.T) {
	t.Parallel()

	// empty endpoint → default
	g, err := New(geo.Options{UserAgent: "test-app/1.0"})
	if err != nil {
		t.Fatalf("New default: %v", err)
	}
	m, ok := g.(*osmGeo)
	if !ok {
		t.Fatalf("expected *osmGeo, got %T", g)
	}
	if m.endpoint != defaultEndpoint {
		t.Fatalf("expected default endpoint %q, got %q", defaultEndpoint, m.endpoint)
	}
	if m.maxBody != defaultMaxBody {
		t.Fatalf("expected default maxBody %d, got %d", defaultMaxBody, m.maxBody)
	}
	if m.client.Timeout != defaultTimeout {
		t.Fatalf("expected default timeout %v, got %v", defaultTimeout, m.client.Timeout)
	}
	if m.baseURL.String() != defaultEndpoint {
		t.Fatalf("baseURL mismatch: %v", m.baseURL)
	}

	// http with AllowInsecure
	g2, err := New(geo.Options{Endpoint: "http://example.com", UserAgent: "test-app/1.0", AllowInsecure: true})
	if err != nil {
		t.Fatalf("http AllowInsecure: %v", err)
	}
	m2, ok := g2.(*osmGeo)
	if !ok {
		t.Fatalf("expected *osmGeo, got %T", g2)
	}
	if m2.endpoint != "http://example.com" {
		t.Fatalf("got %q", m2.endpoint)
	}

	// TrimSuffix
	g3, err := New(geo.Options{Endpoint: "https://example.com/", UserAgent: "test-app/1.0"})
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	m3, ok := g3.(*osmGeo)
	if !ok {
		t.Fatalf("expected *osmGeo, got %T", g3)
	}
	if strings.HasSuffix(m3.endpoint, "/") {
		t.Fatalf("endpoint not trimmed: %q", m3.endpoint)
	}

	// custom BaseURL fallback when Endpoint empty
	g4, err := New(geo.Options{BaseURL: "https://example.com/base", UserAgent: "test-app/1.0"})
	if err != nil {
		t.Fatalf("BaseURL fallback: %v", err)
	}
	m4, ok := g4.(*osmGeo)
	if !ok {
		t.Fatalf("expected *osmGeo, got %T", g4)
	}
	if m4.endpoint != "https://example.com/base" {
		t.Fatalf("BaseURL not used: %q", m4.endpoint)
	}

	// custom Timeout
	g5, err := New(geo.Options{Endpoint: defaultEndpoint, UserAgent: "test-app/1.0", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("custom timeout: %v", err)
	}
	g5m, ok := g5.(*osmGeo)
	if !ok {
		t.Fatalf("expected *osmGeo, got %T", g5)
	}
	if g5m.client.Timeout != 5*time.Second {
		t.Fatalf("custom timeout not set")
	}

	// custom MaxResponseBody
	g6, err := New(geo.Options{Endpoint: defaultEndpoint, UserAgent: "test-app/1.0", MaxResponseBody: 12345})
	if err != nil {
		t.Fatalf("custom maxBody: %v", err)
	}
	g6m, ok := g6.(*osmGeo)
	if !ok {
		t.Fatalf("expected *osmGeo, got %T", g6)
	}
	if g6m.maxBody != 12345 {
		t.Fatalf("custom maxBody not set")
	}

	// endpoint valid with path
	g7, err := New(geo.Options{Endpoint: "https://example.com/api", UserAgent: "test-app/1.0"})
	if err != nil {
		t.Fatalf("endpoint with path: %v", err)
	}
	g7m, ok := g7.(*osmGeo)
	if !ok {
		t.Fatalf("expected *osmGeo, got %T", g7)
	}
	if g7m.endpoint != "https://example.com/api" {
		t.Fatalf("got %q", g7m.endpoint)
	}
}

func TestValidateEndpoint(t *testing.T) {
	t.Parallel()
	if err := validateEndpoint("https://example.com", false); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := validateEndpoint("http://example.com", true); err != nil {
		t.Fatalf("http allowed: %v", err)
	}
	if err := validateEndpoint("http://example.com", false); !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
	if err := validateEndpoint("ftp://example.com", false); !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions for ftp, got %v", err)
	}
	if err := validateEndpoint("://bad", false); !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions for bad url, got %v", err)
	}
	if err := validateEndpoint("", false); !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions for empty, got %v", err)
	}
}

func TestGeocode_OK(t *testing.T) {
	t.Parallel()
	var capturedUA string
	var capturedPath string
	var capturedQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"place_id":1,"lat":"40.7128","lon":"-74.0060","display_name":"New York, NY","address":{"city":"New York","country":"USA"}}]`)
	}))
	defer srv.Close()

	g, err := New(geo.Options{Endpoint: srv.URL, UserAgent: "myapp/1.0", AllowInsecure: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	locs, err := g.Geocode(context.Background(), "New York")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1, got %d", len(locs))
	}
	if locs[0].Lat != 40.7128 || locs[0].Lng != -74.0060 {
		t.Fatalf("loc mismatch: %+v", locs[0])
	}
	if locs[0].Formatted != "New York, NY" {
		t.Fatalf("formatted mismatch: %q", locs[0].Formatted)
	}
	if capturedUA != "myapp/1.0" {
		t.Fatalf("User-Agent not sent: %q", capturedUA)
	}
	if capturedPath != "/search" {
		t.Fatalf("path %q", capturedPath)
	}
	if capturedQuery.Get("format") != "jsonv2" {
		t.Fatalf("format query missing: %v", capturedQuery)
	}
	if capturedQuery.Get("addressdetails") != "1" {
		t.Fatalf("addressdetails missing")
	}
	if capturedQuery.Get("limit") != "10" {
		t.Fatalf("limit missing")
	}
	if capturedQuery.Get("q") != "New York" {
		t.Fatalf("q missing: %q", capturedQuery.Get("q"))
	}
}

func TestGeocode_EmptyResults(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.Geocode(context.Background(), "nowhere")
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGeocode_InvalidLatString(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"place_id":1,"lat":"bad","lon":"-74","display_name":"X","address":{}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.Geocode(context.Background(), "bad")
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for invalid lat, got %v", err)
	}
}

func TestGeocode_InvalidLonString(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"place_id":1,"lat":"40.7","lon":"bad","display_name":"X","address":{}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.Geocode(context.Background(), "bad")
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for invalid lon, got %v", err)
	}
}

func TestGeocode_InvalidCoordinate(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// lat 200 out of range → ValidCoord false → ErrInvalidCoordinate
		fmt.Fprint(w, `[{"place_id":1,"lat":"200","lon":"0","display_name":"X","address":{}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.Geocode(context.Background(), "bad")
	if !errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("expected ErrInvalidCoordinate, got %v", err)
	}
}

func TestGeocode_DecodeError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not json`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.Geocode(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestGeocode_StatusNon200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.Geocode(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("expected status 500, got %v", err)
	}
	if !strings.Contains(err.Error(), "internal error") {
		t.Fatalf("expected body in error, got %v", err)
	}
}

func TestGeocode_StatusTruncate(t *testing.T) {
	t.Parallel()
	large := strings.Repeat("a", 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, large)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.Geocode(context.Background(), "x")
	if err == nil {
		t.Fatalf("expected error")
	}
	// ensure truncated to 512
	if len(err.Error()) > 600 {
		// status prefix + 512 + overhead
		t.Fatalf("error not truncated: len %d", len(err.Error()))
	}
	// check that message contains 512 a's not 1000
	if strings.Count(err.Error(), "a") > 520 {
		t.Fatalf("not truncated: %v", err)
	}
}

func TestGeocode_BodyTooLarge(t *testing.T) {
	t.Parallel()
	large := strings.Repeat("x", 2<<20) // 2MB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, large)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true, MaxResponseBody: 1 << 20})
	_, err := g.Geocode(context.Background(), "x")
	if !errors.Is(err, geo.ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	// also test helper largeErr
	if !errors.Is(largeErr(g, "x"), geo.ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge via helper, got %v", largeErr(g, "x"))
	}
}

func largeErr(g geo.Geo, q string) error {
	_, err := g.Geocode(context.Background(), q)
	return err
}

func TestGeocode_ContextCancellation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := g.Geocode(ctx, "x")
	if err == nil {
		t.Fatalf("expected context error")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") && !strings.Contains(strings.ToLower(err.Error()), "context") {
		// allow url.Error wrapping context.Canceled
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestGeocode_BodyTooLarge_CustomLimit(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 100 bytes body, limit 10 should trigger
		fmt.Fprint(w, strings.Repeat("b", 100))
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true, MaxResponseBody: 10})
	_, err := g.Geocode(context.Background(), "x")
	if !errors.Is(err, geo.ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestReverse_OK_SingleObject(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/reverse" {
			t.Errorf("path %q", r.URL.Path)
		}
		if r.Header.Get("User-Agent") != "app/1.0" {
			t.Errorf("UA %q", r.Header.Get("User-Agent"))
		}
		q := r.URL.Query()
		if q.Get("lat") == "" || q.Get("lon") == "" {
			t.Errorf("missing lat/lon: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"place_id":1,"lat":"40.7","lon":"-74","display_name":"New York","address":{"city":"NYC","country":"USA"}}`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	addrs, err := g.ReverseGeocode(context.Background(), 40.7, -74)
	if err != nil {
		t.Fatalf("ReverseGeocode: %v", err)
	}
	if len(addrs) != 1 {
		t.Fatalf("expected 1, got %d", len(addrs))
	}
	if addrs[0].Formatted != "New York" {
		t.Fatalf("formatted %q", addrs[0].Formatted)
	}
	if addrs[0].Components["city"] != "NYC" {
		t.Fatalf("components %v", addrs[0].Components)
	}
}

func TestReverse_OK_Array(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"place_id":1,"lat":"40.7","lon":"-74","display_name":"New York","address":{"city":"NYC"}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	addrs, err := g.ReverseGeocode(context.Background(), 40.7, -74)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if len(addrs) != 1 || addrs[0].Formatted != "New York" {
		t.Fatalf("unexpected: %v", addrs)
	}
}

func TestReverse_ArrayMultiple(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"place_id":1,"display_name":"A","address":{"city":"A"}},{"place_id":2,"display_name":"B","address":{"city":"B"}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	addrs, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2, got %d", len(addrs))
	}
}

func TestReverse_Empty_Slice(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReverse_Empty_ObjectNoDisplayName(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"place_id":1,"lat":"0","lon":"0","display_name":"","address":{}}`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReverse_ArrayFilteredEmpty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"place_id":1,"display_name":"","address":{}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for filtered empty, got %v", err)
	}
}

func TestReverse_InvalidCoord(t *testing.T) {
	t.Parallel()
	g, _ := New(geo.Options{Endpoint: defaultEndpoint, UserAgent: "app/1.0"})
	tests := []struct{ lat, lng float64 }{
		{100, 0}, {0, 200}, {90.1, 0},
	}
	for _, tc := range tests {
		_, err := g.ReverseGeocode(context.Background(), tc.lat, tc.lng)
		if !errors.Is(err, geo.ErrInvalidCoordinate) {
			t.Fatalf("expected ErrInvalidCoordinate for %v, got %v", tc, err)
		}
	}
}

func TestReverse_DecodeError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not json at all`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestReverse_EmptyBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ` `)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty body, got %v", err)
	}
}

func TestReverse_BracesEmpty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for {}, got %v", err)
	}
}

func TestReverse_StatusNon200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err == nil || !strings.Contains(err.Error(), "status 404") {
		t.Fatalf("expected status 404, got %v", err)
	}
}

func TestReverse_BodyTooLarge(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, strings.Repeat("z", 2<<20))
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if !errors.Is(err, geo.ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestReverse_NilAddressMap(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// omit address field
		fmt.Fprint(w, `{"place_id":1,"lat":"0","lon":"0","display_name":"Some Place"}`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	addrs, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if addrs[0].Components == nil {
		t.Fatalf("expected non-nil components")
	}
}

func TestDistance_Valid(t *testing.T) {
	t.Parallel()
	g, _ := New(geo.Options{Endpoint: defaultEndpoint, UserAgent: "app/1.0"})
	d, err := g.Distance(context.Background(), geo.Point{Lat: 40.7128, Lng: -74.0060}, geo.Point{Lat: 34.0522, Lng: -118.2437})
	if err != nil {
		t.Fatalf("Distance: %v", err)
	}
	// NYC to LA ~3935 km -> 3.9e6 meters
	if d < 3_800_000 || d > 4_100_000 {
		t.Fatalf("distance out of range: %f", d)
	}
}

func TestDistance_Zero(t *testing.T) {
	t.Parallel()
	g, _ := New(geo.Options{Endpoint: defaultEndpoint, UserAgent: "app/1.0"})
	d, err := g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: 0})
	if err != nil {
		t.Fatalf("Distance: %v", err)
	}
	if d != 0 {
		t.Fatalf("expected 0, got %f", d)
	}
}

func TestDistance_InvalidCoord(t *testing.T) {
	t.Parallel()
	g, _ := New(geo.Options{Endpoint: defaultEndpoint, UserAgent: "app/1.0"})
	_, err := g.Distance(context.Background(), geo.Point{Lat: 100, Lng: 0}, geo.Point{Lat: 0, Lng: 0})
	if !errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("expected ErrInvalidCoordinate, got %v", err)
	}
	_, err = g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: 200})
	if !errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("expected ErrInvalidCoordinate for to, got %v", err)
	}
	_, err = g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 91, Lng: 0})
	if !errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("expected ErrInvalidCoordinate, got %v", err)
	}
}

func TestDistance_HaversineRange(t *testing.T) {
	t.Parallel()
	g, _ := New(geo.Options{Endpoint: defaultEndpoint, UserAgent: "app/1.0"})
	// antipodal: 0,0 to 0,180 ~20015 km
	d, err := g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: 180})
	if err != nil {
		t.Fatalf("Distance: %v", err)
	}
	if d < 19_000_000 || d > 21_000_000 {
		t.Fatalf("antipodal distance out of range: %f", d)
	}
}

func TestHaversineClamp(t *testing.T) {
	t.Parallel()
	// direct haversine test to cover clamp branches
	// Use values that would cause a slightly >1 due to floating error
	// The function clamps 0..1, we test it doesn't panic
	if v := haversine(0, 0, 0, 0); v != 0 {
		t.Fatalf("expected 0, got %f", v)
	}
	// Test clamping at 1 via antipodal
	if v := haversine(90, 0, -90, 0); v < 19000 || v > 21000 {
		// 90 to -90 is 180 degrees ~ 20015 km, haversine returns km
		t.Fatalf("haversine antipodal pole: %f", v)
	}
}

func TestHaversine_ExtremeClamp(t *testing.T) {
	t.Parallel()
	// Force a >1 case by checking math boundary: haversine uses sin^2, max 1
	// Verify function doesn't return NaN for extreme inputs
	for _, tc := range []struct{ lat1, lng1, lat2, lng2 float64 }{
		{0, 0, 0, 180},
		{90, 180, -90, -180},
		{-90, -180, 90, 180},
	} {
		v := haversine(tc.lat1, tc.lng1, tc.lat2, tc.lng2)
		if v != v { // NaN
			t.Fatalf("haversine NaN for %v", tc)
		}
		if v < 0 {
			t.Fatalf("haversine negative for %v: %f", tc, v)
		}
	}
}

func TestDo_RedactURLError(t *testing.T) {
	t.Parallel()
	// url.Error with access_token should be redacted
	orig := &url.Error{Op: "Get", URL: "https://example.com/search?access_token=secret123&format=json", Err: errors.New("fail")}
	red := redactURLError(orig)
	var ue *url.Error
	if !errors.As(red, &ue) {
		t.Fatalf("expected *url.Error, got %T", red)
	}
	if strings.Contains(ue.URL, "secret123") {
		t.Fatalf("token not redacted: %q", ue.URL)
	}
	if !strings.Contains(ue.URL, "REDACTED") {
		t.Fatalf("REDACTED missing: %q", ue.URL)
	}
	// without token, should remain same (no redaction but still url.Error)
	orig2 := &url.Error{Op: "Get", URL: "https://example.com/search?q=test", Err: errors.New("fail")}
	red2 := redactURLError(orig2)
	var ue2 *url.Error
	if !errors.As(red2, &ue2) {
		t.Fatalf("expected *url.Error")
	}
	if ue2.URL != orig2.URL {
		t.Fatalf("URL changed unexpectedly: %q vs %q", ue2.URL, orig2.URL)
	}
	// non-url.Error should pass through
	plain := errors.New("plain")
	if got := redactURLError(plain); !errors.Is(got, plain) {
		t.Fatalf("plain error should pass through")
	}
	// url.Error with unparsable URL -> still returns url.Error without panic
	orig3 := &url.Error{Op: "Get", URL: "://bad url", Err: errors.New("fail")}
	red3 := redactURLError(orig3)
	var ue3 *url.Error
	if !errors.As(red3, &ue3) {
		t.Fatalf("expected *url.Error for bad url")
	}
}

func TestReadLimitedBody_Errors(t *testing.T) {
	t.Parallel()
	// successful read
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer srv.Close()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, err := readLimitedBody(resp, 100)
	if err != nil {
		t.Fatalf("readLimitedBody: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body %q", string(body))
	}

	// too large
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, strings.Repeat("a", 20))
	}))
	defer srv2.Close()
	req2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv2.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp2.Body.Close()
	_, err = readLimitedBody(resp2, 10)
	if !errors.Is(err, geo.ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}

	// read error via broken body
	resp3 := &http.Response{Body: io.NopCloser(brokenReader{})}
	_, err = readLimitedBody(resp3, 100)
	if err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("expected read error, got %v", err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("broken") }

func TestCheckStatus(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: http.StatusOK}
	if err := checkStatus(resp, []byte("ok")); err != nil {
		t.Fatalf("200 should not error: %v", err)
	}
	resp404 := &http.Response{StatusCode: http.StatusNotFound}
	err := checkStatus(resp404, []byte("not found"))
	if err == nil || !strings.Contains(err.Error(), "status 404") {
		t.Fatalf("expected status 404, got %v", err)
	}
	resp500 := &http.Response{StatusCode: http.StatusInternalServerError}
	large := strings.Repeat("x", 1000)
	err = checkStatus(resp500, []byte(large))
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("expected status 500, got %v", err)
	}
	if strings.Count(err.Error(), "x") > 520 {
		t.Fatalf("not truncated: %d x", strings.Count(err.Error(), "x"))
	}
}

func TestBuildURL(t *testing.T) {
	t.Parallel()
	m := &osmGeo{endpoint: "https://example.com"}
	u := m.buildURL("/search", url.Values{"q": {"test"}, "format": {"jsonv2"}})
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Path != "/search" {
		t.Fatalf("path %q", parsed.Path)
	}
	if parsed.Query().Get("q") != "test" {
		t.Fatalf("q %q", parsed.Query().Get("q"))
	}
	// no params
	u2 := m.buildURL("/reverse", nil)
	if u2 != "https://example.com/reverse" {
		t.Fatalf("no params url %q", u2)
	}
	u3 := m.buildURL("/reverse", url.Values{})
	if u3 != "https://example.com/reverse" {
		t.Fatalf("empty params url %q", u3)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()
	g, _ := New(geo.Options{Endpoint: defaultEndpoint, UserAgent: "app/1.0"})
	if err := g.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestGeocode_UserAgentRequiredOnDo(t *testing.T) {
	t.Parallel()
	// Ensure Geocode still works after New, and do wraps url.Error
	// Test do with canceled context to trigger url.Error path
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := g.Geocode(ctx, "x")
	if err == nil {
		t.Fatalf("expected error from canceled context")
	}
	// error should be prefixed geo: osm:
	if !strings.Contains(err.Error(), "geo: osm:") {
		t.Fatalf("error missing prefix: %v", err)
	}
}

func TestGeocode_LimitQuery(t *testing.T) {
	t.Parallel()
	// Ensure buildURL properly escapes queries and limit param present
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "a & b" {
			t.Errorf("q not escaped correctly: %q", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"lat":"1","lon":"1","display_name":"A","address":{}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.Geocode(context.Background(), "a & b")
	if err != nil {
		t.Fatalf("Geocode special: %v", err)
	}
}

func TestReverse_NilClientError(t *testing.T) {
	t.Parallel()
	// Test do error wrapping with bad endpoint (no server)
	g, _ := New(geo.Options{Endpoint: "https://127.0.0.1:1", UserAgent: "app/1.0", AllowInsecure: true, Timeout: 100 * time.Millisecond})
	_, err := g.Geocode(context.Background(), "test")
	if err == nil {
		t.Fatalf("expected error for unreachable host")
	}
	if !strings.Contains(err.Error(), "geo: osm:") {
		t.Fatalf("prefix missing: %v", err)
	}
}

func TestGeocode_CreateRequestError(t *testing.T) {
	t.Parallel()
	// Bypass New validation to inject bad URL
	m := &osmGeo{
		endpoint:  "http://example.com\nbad",
		client:    &http.Client{},
		userAgent: "app/1.0",
		maxBody:   1 << 20,
	}
	_, err := m.Geocode(context.Background(), "test")
	if err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("expected create request error, got %v", err)
	}
	if !strings.Contains(err.Error(), "geo: osm:") {
		t.Fatalf("prefix missing: %v", err)
	}
}

func TestReverse_CreateRequestError(t *testing.T) {
	t.Parallel()
	m := &osmGeo{
		endpoint:  "http://example.com\nbad",
		client:    &http.Client{},
		userAgent: "app/1.0",
		maxBody:   1 << 20,
	}
	_, err := m.ReverseGeocode(context.Background(), 0, 0)
	if err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("expected create request error, got %v", err)
	}
}

func TestReverse_Array_NilAddress(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// address field omitted -> nil map, should be converted to empty map
		fmt.Fprint(w, `[{"place_id":1,"display_name":"A"},{"place_id":2,"display_name":"B","address":{"city":"B"}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	addrs, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2, got %d", len(addrs))
	}
	if addrs[0].Components == nil {
		t.Fatalf("expected non-nil components for nil address")
	}
}

func TestReverse_DoError(t *testing.T) {
	t.Parallel()
	g, _ := New(geo.Options{Endpoint: "https://127.0.0.1:1", UserAgent: "app/1.0", AllowInsecure: true, Timeout: 50 * time.Millisecond})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err == nil || !strings.Contains(err.Error(), "geo: osm:") {
		t.Fatalf("expected geo: osm: error, got %v", err)
	}
}

func TestReverse_Array_FilteredWithAddress(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// display_name empty but address non-empty should NOT be filtered -> counted as result? Actually code filters only when both empty.
		fmt.Fprint(w, `[{"place_id":1,"display_name":"","address":{"city":"X"}},{"place_id":2,"display_name":"Y","address":{}}]`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	addrs, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 (empty display but address counts), got %d %v", len(addrs), addrs)
	}
}

func TestReverse_NullArray(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `null`)
	}))
	defer srv.Close()
	g, _ := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for null, got %v", err)
	}
}

func TestPace_SpacesOutRequests(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var times []time.Time

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"place_id":1,"lat":"1","lon":"1","display_name":"x"}]`)
	}))
	defer srv.Close()

	g, err := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	m, ok := g.(*osmGeo)
	if !ok {
		t.Fatalf("New() type = %T, want *osmGeo", g)
	}

	const interval = 50 * time.Millisecond
	m.minInterval = interval

	for range 3 {
		if _, err := m.Geocode(context.Background(), "x"); err != nil {
			t.Fatalf("Geocode: %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if len(times) != 3 {
		t.Fatalf("got %d requests, want 3", len(times))
	}

	const tolerance = 5 * time.Millisecond

	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap < interval-tolerance {
			t.Errorf("request %d gap = %v, want >= ~%v", i, gap, interval)
		}
	}
}

func TestPace_ContextCanceledDuringWait(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"place_id":1,"lat":"1","lon":"1","display_name":"x"}]`)
	}))
	defer srv.Close()

	g, err := New(geo.Options{Endpoint: srv.URL, UserAgent: "app/1.0", AllowInsecure: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	m, ok := g.(*osmGeo)
	if !ok {
		t.Fatalf("New() type = %T, want *osmGeo", g)
	}

	m.minInterval = time.Hour
	if _, gerr := m.Geocode(context.Background(), "x"); gerr != nil {
		t.Fatalf("first Geocode: %v", gerr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = m.Geocode(ctx, "x")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Geocode during pace wait err = %v, want context.DeadlineExceeded", err)
	}
}

func TestPace_ZeroIntervalDisabled(t *testing.T) {
	t.Parallel()

	m := &osmGeo{endpoint: "https://example.com"}

	if err := m.pace(context.Background()); err != nil {
		t.Fatalf("pace() with zero minInterval err = %v, want nil", err)
	}
}
