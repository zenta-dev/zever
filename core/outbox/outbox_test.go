package outbox_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestPublisherFunc(t *testing.T) {
	var got outbox.Message

	var p outbox.Publisher = outbox.PublisherFunc(func(_ context.Context, m outbox.Message) error {
		got = m

		return nil
	})

	if err := p.Publish(context.Background(), outbox.Message{ID: "1", Topic: "t"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if got.ID != "1" {
		t.Errorf("got ID = %q, want 1", got.ID)
	}
}
