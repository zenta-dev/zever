package redis

import (
	"testing"

	"github.com/zenta-dev/zever/eventbus"
)

// TestPull_crossInstance verifies the pull API across two bus instances
// sharing the same miniredis broker: b1 pulls, b2 publishes.
func TestPull_crossInstance(t *testing.T) {
	t.Parallel()

	server := testServer(t)
	b1 := freshAdapterOn(t, server, nil)
	b2 := freshAdapterOn(t, server, nil)
	ctx := t.Context()
	topic := freshTopic()

	ch, err := b1.SubscribeChan(ctx, topic, 16)
	if err != nil {
		t.Fatalf("SubscribeChan err = %v, want nil", err)
	}

	headers := eventbus.NewHeaders(map[string]string{"k": "v"})
	if err := b2.Publish(ctx, topic, eventbus.NewPayload([]byte("x-instance")), headers); err != nil {
		t.Fatalf("Publish err = %v, want nil", err)
	}

	m := waitMsg(t, ch)

	if m.Topic != topic {
		t.Errorf("Topic = %q, want %q", m.Topic, topic)
	}

	if string(m.Payload) != "x-instance" {
		t.Errorf("Payload = %q, want %q", m.Payload, "x-instance")
	}

	if m.Headers["k"] != "v" {
		t.Errorf("Headers = %v, want k=v", m.Headers)
	}

	if m.ReceivedAt.IsZero() {
		t.Error("ReceivedAt is zero, want decode-time stamp")
	}

	if err := b1.Unsubscribe(topic, ch); err != nil {
		t.Fatalf("Unsubscribe err = %v, want nil", err)
	}
}
