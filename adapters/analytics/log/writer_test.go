package log

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/zenta-dev/zever/core/analytics"
)

// RED: NewWithWriter injects writer seam; Track/Identify/Group emit JSON lines.
func TestNewWithWriter_track_emitsJSONLine(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	a, err := NewWithWriter(analytics.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter: %v", err)
	}

	if err := a.Track(t.Context(), "page_view", map[string]any{"page": "/home"}); err != nil {
		t.Fatalf("track: %v", err)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if entry["event"] != "page_view" {
		t.Errorf("want event=page_view, got %v", entry["event"])
	}
}

func TestNewWithWriter_identifyGroup_emitJSONLines(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	a, err := NewWithWriter(analytics.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter: %v", err)
	}

	if err := a.Identify(t.Context(), "user-1", map[string]any{"email": "a@b.com"}); err != nil {
		t.Fatalf("identify: %v", err)
	}

	if err := a.Group(t.Context(), "user-1", "org-42", map[string]any{"plan": "pro"}); err != nil {
		t.Fatalf("group: %v", err)
	}

	lines := bytes.Count(buf.Bytes(), []byte("\n"))
	if lines != 2 {
		t.Errorf("want 2 log lines, got %d", lines)
	}
}

func TestNewWithWriter_nilWriter_fallsBackToStdout(t *testing.T) {
	t.Parallel()

	a, err := NewWithWriter(analytics.Options{}, nil)
	if err != nil {
		t.Fatalf("NewWithWriter nil writer: %v", err)
	}

	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
}

func TestNewWithWriter_invalidOptions_returnsError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	if _, err := NewWithWriter(analytics.Options{MaxProperties: -1}, &buf); err == nil {
		t.Fatal("expected error for invalid options, got nil")
	}
}
