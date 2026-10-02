package jobtest_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/job/jobtest"
)

// TestConformance proves the dispatcher-over-Queue contract holds on the
// in-memory wiring. Not parallel: job definitions are process-global and
// the kit resets the registry per subtest.
func TestConformance(t *testing.T) {
	jobtest.Conformance(t)
}
