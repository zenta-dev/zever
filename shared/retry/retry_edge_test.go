package retry

import (
	"math"
	"testing"
	"time"
)

// TestPolicyNextDelay_overflowCapped verifies huge attempt counts saturate at
// the maximum representable duration instead of overflowing.
func TestPolicyNextDelay_overflowCapped(t *testing.T) {
	t.Parallel()

	exponential := Policy{BaseDelay: time.Second, Multiplier: 2}
	if got := exponential.NextDelay(200); got != maxDuration {
		t.Errorf("exponential NextDelay(200) = %v, want %v", got, maxDuration)
	}

	linear := Policy{BaseDelay: time.Hour, Linear: true}
	if got := linear.NextDelay(math.MaxInt); got != maxDuration {
		t.Errorf("linear NextDelay(MaxInt) = %v, want %v", got, maxDuration)
	}
}

// TestPolicyNextDelay_negativeBaseIsZero verifies negative base delays clamp to zero.
func TestPolicyNextDelay_negativeBaseIsZero(t *testing.T) {
	t.Parallel()

	p := Policy{BaseDelay: -time.Second, Multiplier: 2}
	if got := p.NextDelay(1); got != 0 {
		t.Errorf("NextDelay(1) = %v, want 0", got)
	}
}

// TestPolicyNextDelay_jitterClamped verifies a jitter fraction above 1 is
// clamped to 1, keeping the result within [0, 2*delay].
func TestPolicyNextDelay_jitterClamped(t *testing.T) {
	t.Parallel()

	p := Policy{BaseDelay: time.Second, Multiplier: 1, Jitter: 5}

	for range 200 {
		got := p.NextDelay(1)
		if got < 0 || got > 2*time.Second {
			t.Fatalf("NextDelay with Jitter=5 = %v, want within [0, %v]", got, 2*time.Second)
		}
	}
}
