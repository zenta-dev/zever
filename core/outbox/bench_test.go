package outbox_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func BenchmarkMessageValidate(b *testing.B) {
	msg := outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("{}")}

	b.ReportAllocs()

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

	b.ReportAllocs()

	for b.Loop() {
		_ = msg.Clone()
	}
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_, _ = outbox.ParseAdapter("memory")
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := outbox.Default()

	b.ReportAllocs()

	for b.Loop() {
		_ = opts.Validate()
	}
}

func BenchmarkRegisterOpen(b *testing.B) {
	adapter := outbox.Adapter("bench-stub")

	if err := outbox.Register(adapter, func(outbox.Options) (outbox.Store, error) {
		return &stubStore{name: "bench"}, nil
	}); err != nil {
		b.Fatalf("Register() error = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		s, err := outbox.Open(adapter, outbox.Options{})
		if err != nil {
			b.Fatalf("Open() error = %v", err)
		}

		_ = s.Close()
	}
}

func BenchmarkPublisherFuncPublish(b *testing.B) {
	var p outbox.Publisher = outbox.PublisherFunc(func(context.Context, outbox.Message) error { return nil })
	msg := outbox.Message{ID: "1", Topic: "t"}

	b.ReportAllocs()

	for b.Loop() {
		_ = p.Publish(context.Background(), msg)
	}
}
