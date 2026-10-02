package stdhttp

import (
	"testing"

	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/router/routertest"
)

// TestStdhttpConformance proves the stdhttp adapter honors the
// router.Router contract via the shared conformance kit. Routes serve
// over httptest (no external network).
func TestStdhttpConformance(t *testing.T) {
	t.Parallel()

	routertest.Conformance(t, func(t *testing.T) router.Router {
		t.Helper()

		r, err := New(router.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return r
	})
}
