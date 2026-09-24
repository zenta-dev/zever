package static

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zenta-dev/zever/geo"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "cities.json")
}

func mustOpenStatic(t *testing.T) geo.Geo {
	t.Helper()
	g, err := New(geo.Options{Path: fixturePath(t)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_InvalidPath(t *testing.T) {
	t.Parallel()
	cases := []string{".", "./", "a/..", "a/../b.json", "../cities.json", "..", "a/../../b.json"}
	for _, p := range cases {
		p := p
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			_, err := New(geo.Options{Path: p})
			if err == nil {
				t.Fatalf("want error for path %q", p)
			}
			if !errors.Is(err, geo.ErrInvalidOptions) {
				t.Fatalf("want ErrInvalidOptions, got %v", err)
			}
		})
	}
}

func TestNew_ReadError(t *testing.T) {
	t.Parallel()
	_, err := New(geo.Options{Path: "/nonexistent/cities.json"})
	if err == nil {
		t.Fatal("want error for missing file")
	}
	// read error should not be ErrInvalidOptions / ErrNotFound
	if errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatal("read error must not be ErrInvalidOptions")
	}
}

func TestNew_ParseError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{ invalid"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := New(geo.Options{Path: bad})
	if err == nil {
		t.Fatal("want parse error")
	}
}

func TestNew_Happy(t *testing.T) {
	t.Parallel()
	g, err := New(geo.Options{Path: fixturePath(t)})
	if err != nil {
		t.Fatalf("New happy: %v", err)
	}
	if g == nil {
		t.Fatal("want non-nil geo")
	}
	_ = g.Close()
}

func TestNew_DefaultPath(t *testing.T) {
	// not parallel: uses Chdir which is process-global
	t.Run("read_error", func(t *testing.T) {
		_, err := New(geo.Options{Path: ""})
		if err == nil {
			t.Fatal("want error for default missing file")
		}
	})
	t.Run("happy", func(t *testing.T) {
		dir := t.TempDir()
		// copy fixture to dir/cities.json
		data2, err2 := os.ReadFile(fixturePath(t))
		if err2 != nil {
			t.Fatalf("read fixture: %v", err2)
		}
		if writeErr := os.WriteFile(filepath.Join(dir, "cities.json"), data2, 0o600); writeErr != nil { //nolint:gosec // test temp path
			t.Fatalf("write: %v", writeErr)
		}
		t.Chdir(dir)
		g, err3 := New(geo.Options{})
		if err3 != nil {
			t.Fatalf("New default happy: %v", err3)
		}
		if g == nil {
			t.Fatal("nil geo")
		}
		_ = g.Close()
	})
}

// ---------------------------------------------------------------------------
// Geocode
// ---------------------------------------------------------------------------

func TestGeocode_Empty(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	t.Cleanup(func() { _ = g.Close() })
	for _, q := range []string{"", "   ", "\t\n "} {
		q := q
		t.Run(q, func(t *testing.T) {
			t.Parallel()
			_, err := g.Geocode(t.Context(), q)
			if err == nil {
				t.Fatalf("want error for empty query %q", q)
			}
			if !errors.Is(err, geo.ErrNotFound) {
				t.Fatalf("want ErrNotFound, got %v", err)
			}
		})
	}
}

func TestGeocode_NoMatch(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	_, err := g.Geocode(t.Context(), "Atlantis")
	if err == nil {
		t.Fatal("want error for no match")
	}
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGeocode_Exact(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	locs, err := g.Geocode(t.Context(), "London")
	if err != nil {
		t.Fatalf("geocode: %v", err)
	}
	if len(locs) != 1 || locs[0].Formatted != "London" {
		t.Fatalf("want London exact, got %+v", locs)
	}
}

func TestGeocode_CaseInsensitive(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	t.Cleanup(func() { _ = g.Close() })
	cases := []string{"london", "LONDON", "LoNdOn"}
	for _, q := range cases {
		q := q
		t.Run(q, func(t *testing.T) {
			t.Parallel()
			locs, err := g.Geocode(t.Context(), q)
			if err != nil {
				t.Fatalf("geocode %q: %v", q, err)
			}
			if len(locs) != 1 || locs[0].Formatted != "London" {
				t.Fatalf("want London for %q, got %+v", q, locs)
			}
		})
	}
}

func TestGeocode_Trim(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	locs, err := g.Geocode(t.Context(), "  London ")
	if err != nil {
		t.Fatalf("geocode: %v", err)
	}
	if len(locs) != 1 || locs[0].Formatted != "London" {
		t.Fatalf("want trimmed exact match London, got %+v", locs)
	}
}

func TestGeocode_NoSubstringMiddle(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	_, err := g.Geocode(t.Context(), "york")
	if err == nil {
		t.Fatal("want error for mid-word query")
	}
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGeocode_Prefix(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	locs, err := g.Geocode(t.Context(), "New")
	if err != nil {
		t.Fatalf("geocode: %v", err)
	}
	if len(locs) != 1 || locs[0].Formatted != "New York" {
		t.Fatalf("want New York via prefix, got %+v", locs)
	}
}

func TestGeocode_PrefixMultiple(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	// "P" matches Phoenix and Paris (both start with P)
	locs, err := g.Geocode(t.Context(), "P")
	if err != nil {
		t.Fatalf("geocode P: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("want 2 results for prefix P, got %d: %+v", len(locs), locs)
	}
	names := map[string]bool{}
	for _, l := range locs {
		names[l.Formatted] = true
	}
	if !names["Phoenix"] || !names["Paris"] {
		t.Fatalf("want Phoenix+Paris, got %+v", locs)
	}
}

func TestGeocode_ExactPrioritizedOverPrefix(t *testing.T) {
	t.Parallel()
	// custom cities where "New York" exact coexists with "Newark" prefix.
	dir := t.TempDir()
	path := filepath.Join(dir, "cities.json")
	data := `[{"name":"New York","lat":40.7,"lng":-74},{"name":"Newark","lat":40.7,"lng":-74.1}]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	g, err := New(geo.Options{Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer g.Close()

	// exact should return only New York
	locs, err := g.Geocode(t.Context(), "New York")
	if err != nil {
		t.Fatalf("geocode: %v", err)
	}
	if len(locs) != 1 || locs[0].Formatted != "New York" {
		t.Fatalf("want exact prioritized, got %+v", locs)
	}
	// prefix "New" should return both
	locs, err = g.Geocode(t.Context(), "New")
	if err != nil {
		t.Fatalf("geocode New: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("want 2 for prefix New, got %+v", locs)
	}
}

func TestGeocode_PrefixFallback_Table(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	t.Cleanup(func() { _ = g.Close() })
	cases := []struct {
		name string
		q    string
		want string
	}{
		{"ber prefix", "Ber", "Berlin"},
		{"tok prefix", "Tok", "Tokyo"},
		{"syd prefix", "Syd", "Sydney"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			locs, err := g.Geocode(t.Context(), tc.q)
			if err != nil {
				t.Fatalf("geocode %q: %v", tc.q, err)
			}
			if len(locs) != 1 || locs[0].Formatted != tc.want {
				t.Fatalf("want %s, got %+v", tc.want, locs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ReverseGeocode
// ---------------------------------------------------------------------------

func TestReverseGeocode_ValidWithin(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	// exactly at London
	addrs, err := g.ReverseGeocode(t.Context(), 51.5074, -0.1278)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if len(addrs) != 1 || addrs[0].Formatted != "London" {
		t.Fatalf("want London, got %+v", addrs)
	}
	if addrs[0].Components["city"] != "London" {
		t.Fatalf("want city component London, got %+v", addrs[0].Components)
	}
	// ~1km offset still within 1.5km
	addrs, err = g.ReverseGeocode(t.Context(), 51.515, -0.1278)
	if err != nil {
		t.Fatalf("reverse near: %v", err)
	}
	if len(addrs) == 0 {
		t.Fatal("want near match within 1.5km")
	}
}

func TestReverseGeocode_OutsideNotFound(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	_, err := g.ReverseGeocode(t.Context(), 0, 0)
	if err == nil {
		t.Fatal("want not found for 0,0")
	}
	if !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestReverseGeocode_InvalidCoords(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	t.Cleanup(func() { _ = g.Close() })
	cases := []struct {
		name string
		lat  float64
		lng  float64
	}{
		{"nan lat", math.NaN(), 0},
		{"nan lng", 0, math.NaN()},
		{"inf lat", math.Inf(1), 0},
		{"inf lng", 0, math.Inf(-1)},
		{"lat over 90", 91, 0},
		{"lat under -90", -91, 0},
		{"lng over 180", 0, 181},
		{"lng under -180", 0, -181},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := g.ReverseGeocode(t.Context(), tc.lat, tc.lng)
			if err == nil {
				t.Fatal("want error for invalid coords")
			}
			if !errors.Is(err, geo.ErrInvalidCoordinate) {
				t.Fatalf("want ErrInvalidCoordinate, got %v", err)
			}
		})
	}
}

func TestReverseGeocode_BoundaryValid(t *testing.T) {
	t.Parallel()
	// ensure ValidCoord boundary true: create custom file with point at boundary
	dir := t.TempDir()
	path := filepath.Join(dir, "cities.json")
	data := `[{"name":"NorthPole","lat":90,"lng":180},{"name":"SouthPole","lat":-90,"lng":-180}]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	g, err := New(geo.Options{Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer g.Close()

	if !geo.ValidCoord(90, 180) {
		t.Fatal("90,180 should be valid")
	}
	if !geo.ValidCoord(-90, -180) {
		t.Fatal("-90,-180 should be valid")
	}
	addrs, err := g.ReverseGeocode(t.Context(), 90, 180)
	if err != nil {
		t.Fatalf("reverse boundary: %v", err)
	}
	if addrs[0].Formatted != "NorthPole" {
		t.Fatalf("want NorthPole, got %+v", addrs)
	}
}

// ---------------------------------------------------------------------------
// Distance
// ---------------------------------------------------------------------------

func TestDistance_Valid(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	// NYC-London ~5570km → 5.5M meters
	dist, err := g.Distance(t.Context(), geo.Point{Lat: 40.7128, Lng: -74.0060}, geo.Point{Lat: 51.5074, Lng: -0.1278})
	if err != nil {
		t.Fatalf("distance: %v", err)
	}
	if dist < 5_400_000 || dist > 5_700_000 {
		t.Fatalf("NYC-London want 5.4M-5.7M, got %.0f", dist)
	}
	// London-Paris ~340km
	dist, err = g.Distance(t.Context(), geo.Point{Lat: 51.5074, Lng: -0.1278}, geo.Point{Lat: 48.8566, Lng: 2.3522})
	if err != nil {
		t.Fatalf("distance: %v", err)
	}
	if dist < 300_000 || dist > 400_000 {
		t.Fatalf("London-Paris want 300k-400k, got %.0f", dist)
	}
	// 0.01 deg latitude ~1112m
	dist, err = g.Distance(t.Context(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0.01, Lng: 0})
	if err != nil {
		t.Fatalf("distance: %v", err)
	}
	if dist < 1100 || dist > 1125 {
		t.Fatalf("0.01 deg lat want 1100-1125, got %.2f", dist)
	}
	// zero distance
	dist, err = g.Distance(t.Context(), geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: 0})
	if err != nil {
		t.Fatalf("distance zero: %v", err)
	}
	if dist != 0 {
		t.Fatalf("zero distance want 0, got %v", dist)
	}
}

func TestDistance_InvalidCoords(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	t.Cleanup(func() { _ = g.Close() })
	cases := []struct {
		name string
		from geo.Point
		to   geo.Point
	}{
		{"lat over 90", geo.Point{Lat: 91, Lng: 0}, geo.Point{Lat: 0, Lng: 0}},
		{"lat under -90", geo.Point{Lat: -91, Lng: 0}, geo.Point{Lat: 0, Lng: 0}},
		{"lng over 180", geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: 181}},
		{"lng under -180", geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: -181}},
		{"nan from", geo.Point{Lat: math.NaN(), Lng: 0}, geo.Point{Lat: 0, Lng: 0}},
		{"inf to", geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: math.Inf(1), Lng: 0}},
		{"inf lng", geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: math.Inf(-1)}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := g.Distance(t.Context(), tc.from, tc.to)
			if err == nil {
				t.Fatal("want error for invalid coords")
			}
			if !errors.Is(err, geo.ErrInvalidCoordinate) {
				t.Fatalf("want ErrInvalidCoordinate, got %v", err)
			}
		})
	}
}

func TestDistance_BoundaryValid(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	defer g.Close()
	if !geo.ValidCoord(90, 180) || !geo.ValidCoord(-90, -180) {
		t.Fatal("boundary should be valid")
	}
	dist, err := g.Distance(t.Context(), geo.Point{Lat: 90, Lng: 180}, geo.Point{Lat: -90, Lng: -180})
	if err != nil {
		t.Fatalf("distance boundary: %v", err)
	}
	if math.IsNaN(dist) || math.IsInf(dist, 0) {
		t.Fatalf("want finite distance, got %v", dist)
	}
}

func TestDistance_Antipodal(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	t.Cleanup(func() { _ = g.Close() })
	cases := []struct {
		name string
		from geo.Point
		to   geo.Point
	}{
		{"equator", geo.Point{Lat: 0, Lng: 0}, geo.Point{Lat: 0, Lng: 180}},
		{"high latitudes", geo.Point{Lat: 60, Lng: 0}, geo.Point{Lat: -60, Lng: 180}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dist, err := g.Distance(t.Context(), tc.from, tc.to)
			if err != nil {
				t.Fatalf("distance: %v", err)
			}
			if math.IsNaN(dist) || math.IsInf(dist, 0) {
				t.Fatalf("want finite distance, got %v", dist)
			}
			want := 1000 * math.Pi * 6371
			if dist != want {
				t.Fatalf("antipodal want %.1f, got %.1f", want, dist)
			}
		})
	}
}

func TestHaversineClamp(t *testing.T) {
	t.Parallel()
	// direct haversine call to ensure clamp paths exercised
	// antipodal already covers a>1 clamp (a may be 1.000... due to rounding)
	// identical point covers a==0 path
	if d := haversine(0, 0, 0, 0); d != 0 {
		t.Fatalf("haversine zero want 0, got %v", d)
	}
	if d := haversine(0, 0, 0, 180); d != math.Pi*6371 {
		t.Fatalf("haversine antipodal want %v, got %v", math.Pi*6371, d)
	}
	// a > 1 clamp via floating rounding (pair found by brute force)
	if d := haversine(-80.06864237283763, 179.62420924214427, 80.06864237283763, -0.37579075785572513); d != math.Pi*6371 {
		t.Fatalf("haversine clamp >1 want %v, got %v", math.Pi*6371, d)
	}
	// also test small offset triggers no clamp
	if d := haversine(0, 0, 0.01, 0); d < 1 || d > 2 {
		t.Fatalf("small haversine want 1-2 km, got %v", d)
	}
}

// ---------------------------------------------------------------------------
// Close + Register integration
// ---------------------------------------------------------------------------

func TestClose(t *testing.T) {
	t.Parallel()
	g := mustOpenStatic(t)
	if err := g.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRegisterAndOpen(t *testing.T) {
	t.Parallel()
	// ensure New can be used via geo.Register/Open
	// use unique adapter name to avoid duplicate
	// but Static already may be registered elsewhere? Test via direct New.
	// Instead verify factory works via geo.Open after registering if not yet registered.
	// Try register static if not already.
	_ = geo.Register(geo.Static, New) // ignore duplicate error
	g, err := geo.Open(geo.Static, geo.Options{Path: fixturePath(t)})
	if err != nil {
		t.Fatalf("Open static: %v", err)
	}
	defer g.Close()
	locs, err := g.Geocode(t.Context(), "Berlin")
	if err != nil {
		t.Fatalf("geocode Berlin: %v", err)
	}
	if len(locs) != 1 || locs[0].Formatted != "Berlin" {
		t.Fatalf("want Berlin, got %+v", locs)
	}
}
