package memory

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
)

// TestEdgeTopicAtMaxLen proves a topic of exactly MaxTopicLen is accepted;
// only lengths above the bound are rejected.
func TestEdgeTopicAtMaxLen(t *testing.T) {
	t.Parallel()

	b := coverMustBus(t, testOpts())
	defer b.Close()

	topic := strings.Repeat("a", eventbus.MaxTopicLen)

	if err := b.Publish(t.Context(), topic, eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish at MaxTopicLen: %v", err)
	}
}

// TestEdgePublishEmptyPayload proves a zero-length payload and nil headers
// are valid boundary inputs.
func TestEdgePublishEmptyPayload(t *testing.T) {
	t.Parallel()

	b := coverMustBus(t, testOpts())
	defer b.Close()

	if err := b.Publish(t.Context(), "t", nil, nil); err != nil {
		t.Fatalf("Publish nil payload: %v", err)
	}

	if err := b.Publish(t.Context(), "t", eventbus.NewPayload(nil), eventbus.NewHeaders(nil)); err != nil {
		t.Fatalf("Publish empty payload: %v", err)
	}
}
