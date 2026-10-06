package cdc

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/outbox"
)

func BenchmarkNew(b *testing.B) {
	opts := Options{Options: outbox.Options{DSN: "postgres://localhost/db"}}

	b.ReportAllocs()

	for b.Loop() {
		s, err := New(opts)
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		_ = s.Close()
	}
}

func BenchmarkRecord(b *testing.B) {
	s := &store{prefix: DefaultPrefix}
	tx := &fakeTx{}
	msg := outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("payload")}

	b.ReportAllocs()

	for b.Loop() {
		_ = s.Record(context.Background(), tx, msg)
	}
}

func BenchmarkEncodeMessage(b *testing.B) {
	msg := outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("payload")}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = encodeMessage(msg)
	}
}

func BenchmarkDecodeMessage(b *testing.B) {
	payload, err := encodeMessage(outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("payload")})
	if err != nil {
		b.Fatalf("encodeMessage() error = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = decodeMessage(payload)
	}
}

func BenchmarkHandleMessage(b *testing.B) {
	s := &store{prefix: DefaultPrefix, maxAttempts: 3, sleep: recordingSleep(new([]time.Duration))}
	pub := &stubPublisher{}

	payload, err := encodeMessage(outbox.Message{ID: "evt-1", Topic: "orders"})
	if err != nil {
		b.Fatalf("encodeMessage() error = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = s.handleMessage(b.Context(), DefaultPrefix, payload, pub.Publish)
	}
}

func BenchmarkQuoteLiteral(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = quoteLiteral("zever_outbox")
	}
}

func BenchmarkSleepCtx(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = sleepCtx(b.Context(), 0)
	}
}

func BenchmarkStatus(b *testing.B) {
	s := &store{}

	b.ReportAllocs()

	for b.Loop() {
		_ = s.Status()
	}
}
