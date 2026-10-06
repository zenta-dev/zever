package memory_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/adapters/outbox/memory"
	"github.com/zenta-dev/zever/core/outbox"
)

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		s, err := memory.New(memory.Options{})
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		_ = s.Close()
	}
}

func BenchmarkRecord(b *testing.B) {
	s, err := memory.New(memory.Options{Publisher: outbox.PublisherFunc(func(context.Context, outbox.Message) error { return nil })})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	msg := outbox.Message{ID: "1", Topic: "t", Payload: []byte("payload")}

	b.ReportAllocs()

	for b.Loop() {
		_ = s.Record(context.Background(), nil, msg)
	}
}

func BenchmarkRecordInvalid(b *testing.B) {
	s, err := memory.New(memory.Options{Publisher: outbox.PublisherFunc(func(context.Context, outbox.Message) error { return nil })})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	b.ReportAllocs()

	for b.Loop() {
		_ = s.Record(context.Background(), nil, outbox.Message{})
	}
}

func BenchmarkStatus(b *testing.B) {
	s, err := memory.New(memory.Options{})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	b.ReportAllocs()

	for b.Loop() {
		_ = s.Status()
	}
}

func BenchmarkName(b *testing.B) {
	s, err := memory.New(memory.Options{})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	b.ReportAllocs()

	for b.Loop() {
		_ = s.Name()
	}
}
