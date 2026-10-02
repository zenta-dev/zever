package routertest_test

import (
	"testing"

	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/router/routertest"
)

// TestConformanceStdhttp proves the kit passes against the stdhttp adapter.
func TestConformanceStdhttp(t *testing.T) {
	t.Parallel()

	routertest.Conformance(t, func(t *testing.T) router.Router {
		t.Helper()

		r, err := routerstdhttp.New(router.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return r
	})
}
