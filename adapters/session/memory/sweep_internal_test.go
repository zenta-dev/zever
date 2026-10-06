package memory

import (
	"testing"
	"time"
)

// TestSweepIntervalBoundaries proves the derived sweep period is clamped to
// [DefaultMinSweepInterval, DefaultMaxSweepInterval] on both ends and passes
// through unclamped in the middle.
func TestSweepIntervalBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ttl  time.Duration
		want time.Duration
	}{
		{"below floor clamped up", 100 * time.Millisecond, DefaultMinSweepInterval},
		{"at floor", 2 * DefaultMinSweepInterval, DefaultMinSweepInterval},
		{"middle passes through", 4 * time.Second, 2 * time.Second},
		{"above ceiling clamped down", time.Hour, DefaultMaxSweepInterval},
		{"at ceiling", 2 * DefaultMaxSweepInterval, DefaultMaxSweepInterval},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := sweepInterval(tc.ttl); got != tc.want {
				t.Fatalf("sweepInterval(%v) = %v, want %v", tc.ttl, got, tc.want)
			}
		})
	}
}
