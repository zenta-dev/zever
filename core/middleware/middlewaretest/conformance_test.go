package middlewaretest_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/middleware/middlewaretest"
)

// TestConformance proves the chaining contract holds over the in-kit stubs.
func TestConformance(t *testing.T) {
	t.Parallel()

	middlewaretest.Conformance(t)
}
