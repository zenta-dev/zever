package queue

import (
	"testing"
	"time"
)

// newBenchAdapter opens a queue-backed webhook over an in-process stub queue
// whose Pop always reports empty, so the consumer pool stays idle and the
// benchmark measures the enqueue path only.
func newBenchAdapter(b *testing.B) *adapter {
	b.Helper()

	a := newTestAdapter(newStubQueue())
	b.Cleanup(func() { _ = a.Close() })

	if err := a.Register(b.Context(), "evt", "https://example.com/hook", "bench-secret"); err != nil {
		b.Fatalf("Register(): %v", err)
	}

	return a
}

// BenchmarkRegister measures target syntax validation plus the registration
// map write under the adapter mutex.
func BenchmarkRegister(b *testing.B) {
	a := newTestAdapter(newStubQueue())
	b.Cleanup(func() { _ = a.Close() })

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := a.Register(b.Context(), "evt", "http://127.0.0.1:1/hook", "bench-secret"); err != nil {
			b.Fatalf("Register(): %v", err)
		}
	}
}

// BenchmarkDeliver measures enqueuing one delivery: signature build, header
// map construction, and a queue push per registered target.
func BenchmarkDeliver(b *testing.B) {
	a := newBenchAdapter(b)
	ctx := b.Context()
	payload := []byte(`{"event":"bench"}`)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := a.Deliver(ctx, "evt", payload); err != nil {
			b.Fatalf("Deliver(): %v", err)
		}
	}
}

// BenchmarkBuildSignatureHeader measures building the "t=...,v1=..." HMAC
// envelope for a delivery.
func BenchmarkBuildSignatureHeader(b *testing.B) {
	payload := []byte(`{"event":"bench"}`)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if h := buildSignatureHeader("bench-secret", payload, 1700000000); h == "" {
			b.Fatal("buildSignatureHeader() returned empty header")
		}
	}
}

// BenchmarkVerifySignatureHeader measures parsing and constant-time HMAC
// verification of a valid envelope.
func BenchmarkVerifySignatureHeader(b *testing.B) {
	payload := []byte(`{"event":"bench"}`)
	header := buildSignatureHeader("bench-secret", payload, 1700000000)
	now := time.Unix(1700000000, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := verifySignatureHeader("bench-secret", payload, header, defaultReplayTolerance, now); err != nil {
			b.Fatalf("verifySignatureHeader(): %v", err)
		}
	}
}
