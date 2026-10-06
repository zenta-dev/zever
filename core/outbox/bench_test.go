package outbox_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/zenta-dev/zever/core/db"
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

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		a := outbox.Adapter(fmt.Sprintf("bench-%d", i))
		if err := outbox.Register(a, func(outbox.Options) (outbox.Store, error) { return &stubStore{}, nil }); err != nil {
			b.Fatalf("Register(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := outbox.Adapter("bench-open")
	if err := outbox.Register(a, func(outbox.Options) (outbox.Store, error) { return &stubStore{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := outbox.Open(a, outbox.Options{}); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenShared(b *testing.B) {
	a := outbox.Adapter("bench-open-shared")
	if err := outbox.RegisterShared(a, func(db.DB, outbox.Options) (outbox.Store, error) {
		return &stubStore{}, nil
	}); err != nil {
		b.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := outbox.OpenShared(a, nil, outbox.Options{}); err != nil {
			b.Fatalf("OpenShared(%v) error = %v", a, err)
		}
	}
}
