package observability

import (
	"testing"
)

func BenchmarkOptionsValidate(b *testing.B) {
	o := Options{ServiceName: "svc", Endpoint: "localhost:4317", Insecure: true, SampleRatio: 0.5}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := o.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

func BenchmarkNormalizeAttrs(b *testing.B) {
	attrs := []Attr{
		String(HTTPRequestMethod, "GET"),
		Int(HTTPResponseStatusCode, 200),
		String("user.password", "hunter2"),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = normalizeAttrs(attrs, MaxValueLen)
	}
}

func BenchmarkRedact(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = redact("authorization")
	}
}

func BenchmarkRequestIDFromContext(b *testing.B) {
	ctx := WithRequestID(b.Context(), "req-1")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = RequestIDFromContext(ctx)
	}
}

func BenchmarkMapCarrierSetGet(b *testing.B) {
	c := MapCarrier{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.Set("traceparent", "00-abc-def-01")
		_ = c.Get("traceparent")
	}
}
