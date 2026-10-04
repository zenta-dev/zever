package log

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/zenta-dev/zever/core/analytics"
)

// TestEdgeTrack_atPropertyLimit covers the inclusive boundary of the
// property-count cap: exactly maxProperties is accepted, one more is rejected.
func TestEdgeTrack_atPropertyLimit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	a := &adapter{
		logger:             slog.New(slog.NewJSONHandler(&buf, nil)),
		anonymousID:        analytics.DefaultAnonymousID,
		maxProperties:      2,
		maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
	}

	if err := a.Track(t.Context(), "e", map[string]any{"a": 1, "b": 2}); err != nil {
		t.Fatalf("Track at limit = %v, want nil", err)
	}

	if err := a.Track(t.Context(), "e", map[string]any{"a": 1, "b": 2, "c": 3}); err == nil {
		t.Fatal("Track over limit = nil, want CountLimitError")
	}
}

// TestEdgeTrack_emptyEvent covers the empty-string boundary: an empty event
// name is still a valid log line, not an error.
func TestEdgeTrack_emptyEvent(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	a := newTestAdapter(&buf)

	if err := a.Track(t.Context(), "", nil); err != nil {
		t.Fatalf("Track empty event = %v, want nil", err)
	}

	if entry := decodeEntry(t, &buf); entry["event"] != "" {
		t.Errorf("event = %v, want empty", entry["event"])
	}
}
