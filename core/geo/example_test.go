package geo_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zenta-dev/zever/adapters/geo/static"
	"github.com/zenta-dev/zever/core/geo"
)

// ExampleOpen opens the static adapter over a small cities file and geocodes.
func ExampleOpen() {
	if err := geo.Register(geo.Static, static.New); err != nil {
		return
	}

	dir := filepath.Join(os.TempDir(), "zever-geo-example")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "cities.json")
	data := []byte(`[{"name":"Paris","lat":48.8566,"lng":2.3522}]`)

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return
	}

	g, err := geo.Open(geo.Static, geo.Options{Path: path})
	if err != nil {
		return
	}

	defer func() { _ = g.Close() }()

	locs, err := g.Geocode(context.Background(), "paris")
	if err != nil {
		return
	}

	fmt.Println(locs[0].Formatted)
	// Output: Paris
}
