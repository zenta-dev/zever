package authztest_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/authz/authztest"
)

// TestConformance runs the self-contained authorization kit (stub backends).
func TestConformance(t *testing.T) {
	t.Parallel()

	authztest.Conformance(t)
}
