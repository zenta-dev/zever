package redis

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
)

// TestEdgeTopicAtMaxLen proves a topic of exactly MaxTopicLen is accepted
// by the redis adapter; only lengths above the bound are rejected.
func TestEdgeTopicAtMaxLen(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, nil)
	topic := strings.Repeat("a", eventbus.MaxTopicLen)

	if err := a.Publish(t.Context(), topic, eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish at MaxTopicLen: %v", err)
	}
}

// TestEdgePublishEmptyPayload proves a zero-length payload and nil headers
// are valid boundary inputs.
func TestEdgePublishEmptyPayload(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, func(o *eventbus.Options) {
		o.BufferSize = eventbus.DefaultBufferSize
	})

	if err := a.Publish(t.Context(), "t", nil, nil); err != nil {
		t.Fatalf("Publish nil payload: %v", err)
	}
}
