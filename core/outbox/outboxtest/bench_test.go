package outboxtest_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/outbox/outboxtest"
)

func BenchmarkNewRecorder(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = outboxtest.NewRecorder()
	}
}

func BenchmarkRecorderPublish(b *testing.B) {
	r := outboxtest.NewRecorder()
	msg := outbox.Message{ID: "m-1", Topic: "orders", Payload: []byte("payload")}
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		_ = r.Publish(ctx, msg)
	}
}

func BenchmarkRecorderMessages(b *testing.B) {
	r := outboxtest.NewRecorder()

	if err := r.Publish(b.Context(), outbox.Message{ID: "m-1", Topic: "orders"}); err != nil {
		b.Fatalf("Publish() error = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = r.Messages()
	}
}
