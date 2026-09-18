package slog

import (
	"bytes"
	"testing"

	"github.com/zenta-dev/zever/log"
)

// RED: NewWithWriter injects writer seam.
func TestNewWithWriter_writesToBuffer(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &buf)
	l.Info().Str("k", "v").Msg("hello")

	if buf.Len() == 0 {
		t.Fatal("expected log output in buffer, got empty")
	}
}

func TestNewWithWriter_nilWriter_fallsBackToStdout(t *testing.T) {
	t.Parallel()

	l := NewWithWriter(log.Options{}, nil)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}
