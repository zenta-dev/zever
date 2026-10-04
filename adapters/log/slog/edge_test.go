package slog

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/log"
)

// TestEdgeLongMessage proves a megabyte message is encoded without panic.
func TestEdgeLongMessage(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	l := NewWithWriter(log.Options{MinLevel: log.LevelInfo}, &buf)

	l.Info().Str("k", "v").Msg(strings.Repeat("x", 1<<20))

	if buf.Len() < 1<<20 {
		t.Fatalf("output %d bytes, want >= %d", buf.Len(), 1<<20)
	}
}

// TestEdgeEmptyMessageAndNoFields proves an empty event still encodes a line.
func TestEdgeEmptyMessageAndNoFields(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	l := NewWithWriter(log.Options{MinLevel: log.LevelInfo}, &buf)

	l.Info().Send()

	if buf.Len() == 0 {
		t.Fatal("output empty, want JSON line")
	}
}
