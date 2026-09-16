package google

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	gmaps "googlemaps.github.io/maps"

	"github.com/zenta-dev/zever/geo"
)

func TestNew_RequiresAPIKey(t *testing.T) {
	_, err := New(geo.Options{})
	if !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestNew_NoBaseURL(t *testing.T) {
	g, err := New(geo.Options{APIKey: "fake"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil geo")
	}
	_ = g.Close()
}

func TestNew_InvalidBaseURL(t *testing.T) {
	cases := []struct {
		name  string
		url   string
		allow bool
	}{
		{"missing_scheme", "maps.googleapis.com/maps/api", false},
		{"missing_host", "https://", false},
		{"http_blocked", "http://example.com", false},
		{"ftp_scheme", "ftp://example.com", false},
		{"parse_error", "http://[invalid", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(geo.Options{APIKey: "fake", BaseURL: tc.url, AllowInsecure: tc.allow})
			if !errors.Is(err, geo.ErrInvalidOptions) {
				t.Fatalf("expected ErrInvalidOptions for %q, got %v", tc.url, err)
			}
		})
	}
}

func TestNew_WithBaseURL_AllowInsecure(t *testing.T) {
	g, err := New(geo.Options{APIKey: "fake", BaseURL: "http://example.com", AllowInsecure: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil")
	}
	_ = g.Close()

	g2, err := New(geo.Options{APIKey: "fake", BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = g2.Close()
}

func TestValidateBaseURL_Table(t *testing.T) {
	tests := []struct {
		raw   string
		allow bool
		ok    bool
	}{
		{"https://maps.googleapis.com", false, true},
		{"https://maps.googleapis.com/foo", false, true},
		{"http://example.com", true, true},
		{"http://example.com", false, false},
		{"", false, false}, // url.Parse("") gives empty scheme/host -> fail (but we don't call with empty in New)
		{"://bad", false, false},
		{"https://", false, false},
		{"ftp://example.com", false, false},
		{"ftp://example.com", true, false},
	}
	for _, tc := range tests {
		err := validateBaseURL(tc.raw, tc.allow)
		if tc.ok && err != nil {
			t.Fatalf("validateBaseURL(%q, %v) unexpected error: %v", tc.raw, tc.allow, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("validateBaseURL(%q, %v) expected error, got nil", tc.raw, tc.allow)
		}
		if !tc.ok && !errors.Is(err, geo.ErrInvalidOptions) {
			t.Fatalf("validateBaseURL(%q) expected ErrInvalidOptions, got %v", tc.raw, err)
		}
	}
	// also test parse error explicitly
	err := validateBaseURL("http://[::1", false)
	if !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions for parse error, got %v", err)
	}
}

func TestValidateBaseURL_ParseError(t *testing.T) {
	// url.Parse error case: invalid URL with control character
	err := validateBaseURL("http://%zz", false)
	// This may or may not be parse error, but ensure it returns ErrInvalidOptions if error
	if err != nil && !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
	// Force parse error via invalid URL that url.Parse fails on
	// Use non-UTF8? Instead use "http://[invalid"
	err = validateBaseURL("http://[invalid", false)
	if !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

//nolint:unparam // code always 200 in current tests but keeps helper generic
func newTestServer(t *testing.T, body string, code int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newGeoWithServer(t *testing.T, srv *httptest.Server) geo.Geo {
	t.Helper()
	g, err := New(geo.Options{APIKey: "fake-key", BaseURL: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func TestGeocode_ZeroResults(t *testing.T) {
	srv := newTestServer(t, `{"results":[],"status":"ZERO_RESULTS"}`, 200)
	g := newGeoWithServer(t, srv)
	_, err := g.Geocode(context.Background(), "nowhere 12345")
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGeocode_OK(t *testing.T) {
	resp := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"formatted_address": "1600 Amphitheatre Parkway, Mountain View, CA 94043, USA",
				"geometry": map[string]interface{}{
					"location": map[string]interface{}{"lat": 37.4224764, "lng": -122.0842499},
				},
			},
			{
				"formatted_address": "Second result",
				"geometry": map[string]interface{}{
					"location": map[string]interface{}{"lat": 37.5, "lng": -122.0},
				},
			},
		},
		"status": "OK",
	}
	b, _ := json.Marshal(resp)
	srv := newTestServer(t, string(b), 200)
	g := newGeoWithServer(t, srv)

	locs, err := g.Geocode(context.Background(), "1600 Amphitheatre Parkway")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("expected 2 results, got %d", len(locs))
	}
	if locs[0].Lat != 37.4224764 || locs[0].Lng != -122.0842499 {
		t.Fatalf("unexpected loc0: %+v", locs[0])
	}
	if locs[0].Formatted != "1600 Amphitheatre Parkway, Mountain View, CA 94043, USA" {
		t.Fatalf("unexpected formatted: %q", locs[0].Formatted)
	}
	if locs[1].Formatted != "Second result" {
		t.Fatalf("unexpected second formatted")
	}
}

func TestGeocode_ClientError(t *testing.T) {
	srv := newTestServer(t, `{"status":"INVALID_REQUEST","error_message":"bad"}`, 200)
	g := newGeoWithServer(t, srv)
	_, err := g.Geocode(context.Background(), "addr")
	if err == nil {
		t.Fatal("expected error")
	}
	// should not be ErrNotFound, just generic wrapped error
	if errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("should not be ErrNotFound, got %v", err)
	}
}

func TestReverseGeocode_InvalidCoord(t *testing.T) {
	srv := newTestServer(t, `{"results":[],"status":"OK"}`, 200)
	g := newGeoWithServer(t, srv)

	cases := []struct {
		lat, lng float64
	}{
		{91, 0},
		{-91, 0},
		{0, 181},
		{0, -181},
		{math.NaN(), 0},
		{0, math.NaN()},
		{math.Inf(1), 0},
		{0, math.Inf(-1)},
	}
	for _, tc := range cases {
		_, err := g.ReverseGeocode(context.Background(), tc.lat, tc.lng)
		if !errors.Is(err, geo.ErrInvalidCoordinate) {
			t.Fatalf("lat=%v lng=%v expected ErrInvalidCoordinate, got %v", tc.lat, tc.lng, err)
		}
	}
}

func TestReverseGeocode_ZeroResults(t *testing.T) {
	srv := newTestServer(t, `{"results":[],"status":"ZERO_RESULTS"}`, 200)
	g := newGeoWithServer(t, srv)
	_, err := g.ReverseGeocode(context.Background(), 37.4, -122.0)
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReverseGeocode_OK(t *testing.T) {
	resp := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"formatted_address": "1600 Amphitheatre Parkway, Mountain View, CA",
				"address_components": []map[string]interface{}{
					{"long_name": "1600", "short_name": "1600", "types": []string{"street_number"}},
					{"long_name": "Mountain View", "short_name": "Mountain View", "types": []string{"locality", "political"}},
				},
			},
		},
		"status": "OK",
	}
	b, _ := json.Marshal(resp)
	srv := newTestServer(t, string(b), 200)
	g := newGeoWithServer(t, srv)

	addrs, err := g.ReverseGeocode(context.Background(), 37.422, -122.084)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addrs) != 1 {
		t.Fatalf("expected 1, got %d", len(addrs))
	}
	if addrs[0].Formatted != "1600 Amphitheatre Parkway, Mountain View, CA" {
		t.Fatalf("bad formatted: %q", addrs[0].Formatted)
	}
	if addrs[0].Components["street_number"] != "1600" {
		t.Fatalf("bad component: %+v", addrs[0].Components)
	}
	if addrs[0].Components["locality"] != "Mountain View" {
		t.Fatalf("bad locality")
	}
}

func TestReverseGeocode_ClientError(t *testing.T) {
	srv := newTestServer(t, `{"status":"REQUEST_DENIED","error_message":"denied"}`, 200)
	g := newGeoWithServer(t, srv)
	_, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, geo.ErrNotFound) || errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("should not be notfound/invalidcoord")
	}
}

func TestReverseGeocode_EmptyTypes(t *testing.T) {
	// Ensure branch where ac.Types empty does not panic and is skipped
	resp := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"formatted_address": "Somewhere",
				"address_components": []map[string]interface{}{
					{"long_name": "Foo", "short_name": "Foo", "types": []string{}},
					{"long_name": "Bar", "short_name": "Bar", "types": []string{"country"}},
				},
			},
		},
		"status": "OK",
	}
	b, _ := json.Marshal(resp)
	srv := newTestServer(t, string(b), 200)
	g := newGeoWithServer(t, srv)
	addrs, err := g.ReverseGeocode(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := addrs[0].Components["country"]; !ok {
		t.Fatalf("expected country component")
	}
	if len(addrs[0].Components) != 1 {
		t.Fatalf("expected 1 component, got %+v", addrs[0].Components)
	}
}

func TestDistance_InvalidCoord(t *testing.T) {
	srv := newTestServer(t, `{"status":"OK","rows":[]}`, 200)
	g := newGeoWithServer(t, srv)

	_, err := g.Distance(context.Background(), geo.Point{Lat: 91, Lng: 0}, geo.Point{Lat: 0, Lng: 0})
	if !errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("expected ErrInvalidCoordinate for from, got %v", err)
	}
	_, err = g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: 200})
	if !errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("expected ErrInvalidCoordinate for to, got %v", err)
	}
	_, err = g.Distance(context.Background(), geo.Point{Lat: math.NaN(), Lng: 0}, geo.Point{Lat: 0, Lng: 0})
	if !errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("expected ErrInvalidCoordinate for NaN, got %v", err)
	}
}

func TestDistance_NoResults_EmptyRows(t *testing.T) {
	srv := newTestServer(t, `{"status":"OK","rows":[]}`, 200)
	g := newGeoWithServer(t, srv)
	_, err := g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 1, Lng: 1})
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound empty rows, got %v", err)
	}
}

func TestDistance_NoResults_EmptyElements(t *testing.T) {
	srv := newTestServer(t, `{"status":"OK","rows":[{"elements":[]}]}`, 200)
	g := newGeoWithServer(t, srv)
	_, err := g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 1, Lng: 1})
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound empty elements, got %v", err)
	}
}

func TestDistance_StatusNotOK(t *testing.T) {
	resp := map[string]interface{}{
		"status": "OK",
		"rows": []map[string]interface{}{
			{"elements": []map[string]interface{}{
				{"status": "ZERO_RESULTS"},
			}},
		},
	}
	b, _ := json.Marshal(resp)
	srv := newTestServer(t, string(b), 200)
	g := newGeoWithServer(t, srv)
	_, err := g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 1, Lng: 1})
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for status not OK, got %v", err)
	}
}

func TestDistance_OK(t *testing.T) {
	resp := map[string]interface{}{
		"status":                "OK",
		"destination_addresses": []string{"3150 Commonwealth Ave W, Singapore 129580"},
		"origin_addresses":      []string{"105 Cecil St, Singapore 069534"},
		"rows": []map[string]interface{}{
			{"elements": []map[string]interface{}{
				{
					"status":   "OK",
					"distance": map[string]interface{}{"text": "12.5 km", "value": 12535},
					"duration": map[string]interface{}{"text": "18 mins", "value": 1083},
				},
			}},
		},
	}
	b, _ := json.Marshal(resp)
	srv := newTestServer(t, string(b), 200)
	g := newGeoWithServer(t, srv)
	d, err := g.Distance(context.Background(), geo.Point{Lat: 1.315125, Lng: 103.764713}, geo.Point{Lat: 1.280776, Lng: 103.8487})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 12535 {
		t.Fatalf("expected 12535, got %v", d)
	}
}

func TestDistance_ClientError(t *testing.T) {
	srv := newTestServer(t, `{"status":"OVER_QUERY_LIMIT","error_message":"limit"}`, 200)
	g := newGeoWithServer(t, srv)
	_, err := g.Distance(context.Background(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 1, Lng: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, geo.ErrNotFound) || errors.Is(err, geo.ErrInvalidCoordinate) {
		t.Fatalf("should not be notfound")
	}
}

func TestClose(t *testing.T) {
	g, err := New(geo.Options{APIKey: "fake"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestGeocode_HTTPError(t *testing.T) {
	// server returns 500 with non-JSON; client will try to decode but still error? Check behavior.
	// Maps client does json.Decode; if 500 with error body, it will still try decode. Let's test with 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	defer srv.Close()
	g, err := New(geo.Options{APIKey: "fake", BaseURL: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer g.Close()
	_, err = g.Geocode(context.Background(), "addr")
	if err == nil {
		t.Fatal("expected error on http 500")
	}
}

func TestNew_ClientError(t *testing.T) {
	orig := newClient
	newClient = func(_ ...gmaps.ClientOption) (*gmaps.Client, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newClient = orig })
	_, err := New(geo.Options{APIKey: "fake"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errors.New("boom")) {
		// just check wrapped
		if err.Error() == "" {
			t.Fatalf("empty error")
		}
	}
}
