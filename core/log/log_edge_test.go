package log

import (
	"testing"
)

func TestLevelString_maxValue_unknown(t *testing.T) {
	t.Parallel()

	if got := Level(255).String(); got != "unknown" {
		t.Fatalf("Level(255).String() = %q, want %q", got, "unknown")
	}
}

