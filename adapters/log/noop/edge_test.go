package noop

import (
	"testing"

	"github.com/zenta-dev/zever/core/log"
)

// TestEdgeEnabled_outOfRangeLevels proves Enabled is false for unknown and
// out-of-range levels.
func TestEdgeEnabled_outOfRangeLevels(t *testing.T) {
	t.Parallel()

	l := New()

	for _, lv := range []log.Level{log.Level(0), log.Level(99), log.Level(255)} {
		if l.Enabled(lv) {
			t.Fatalf("Enabled(%d) = true, want false", lv)
		}
	}
}

// TestEdgeWithContext_returnsSameLogger proves WithContext never allocates a
// distinct logger.
func TestEdgeWithContext_returnsSameLogger(t *testing.T) {
	t.Parallel()

	l := New()

	if got := l.WithContext(t.Context()); got != l {
		t.Fatalf("WithContext() = %v, want same logger", got)
	}
}

// TestEdgeEvent_chainingReturnsSameEvent proves the chainers return the
// shared noop event.
func TestEdgeEvent_chainingReturnsSameEvent(t *testing.T) {
	t.Parallel()

	l := New()
	ev := l.Info()

	if got := ev.Str("k", "v").Int("i", 1).Bool("b", true); got == nil {
		t.Fatal("chained event = nil, want non-nil")
	}
}
