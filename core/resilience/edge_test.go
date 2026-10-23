package resilience_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

func TestState_constants(t *testing.T) {
	t.Parallel()

	if resilience.StateClosed != "closed" || resilience.StateOpen != "open" || resilience.StateHalfOpen != "half-open" {
		t.Fatalf("state constants = %q, %q, %q", resilience.StateClosed, resilience.StateOpen, resilience.StateHalfOpen)
	}
}
