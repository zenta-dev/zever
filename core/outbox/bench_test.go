package outbox_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func BenchmarkMessageValidate(b *testing.B) {
	msg := outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("{}")}

	for b.Loop() {
		_ = msg.Validate()
	}
}

func BenchmarkMessageClone(b *testing.B) {
	msg := outbox.Message{
		ID:      "evt-1",
		Topic:   "orders",
		Payload: []byte("payload"),
		Headers: map[string]string{"k": "v"},
	}

	for b.Loop() {
		_ = msg.Clone()
	}
}
