package pretty

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/log"
)

// TestEdgeLongMessageAndValue proves a megabyte-scale message and field value
// are rendered without truncation or panic.
func TestEdgeLongMessageAndValue(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	l := NewWithWriter(log.Options{MinLevel: log.LevelInfo}, &buf)

	long := strings.Repeat("x", 1<<20)
	l.Info().Str("k", long).Msg(long)

	if got := buf.Len(); got < 2<<20 {
		t.Fatalf("output %d bytes, want >= %d", got, 2<<20)
	}
}

// TestEdgeEmptyMessageAndNoFields proves an empty message with no fields
// still emits a level line.
func TestEdgeEmptyMessageAndNoFields(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	l := NewWithWriter(log.Options{MinLevel: log.LevelInfo}, &buf)

	l.Info().Send()

	if !strings.Contains(buf.String(), "INFO") {
		t.Fatalf("output %q missing INFO", buf.String())
	}
}
