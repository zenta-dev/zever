package observability

import (
	"fmt"
	"sync/atomic"
	"testing"
)

var benchAdapterSeq atomic.Int64

func benchFreshAdapter() Adapter {
	return Adapter(fmt.Sprintf("bench-%d", 1000+benchAdapterSeq.Add(1)))
}

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := Register(benchFreshAdapter(), func(Options) (Provider, error) { return stubProvider{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := benchFreshAdapter()
	if err := Register(a, func(Options) (Provider, error) { return stubProvider{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := Open(a, Options{ServiceName: "svc"}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchFreshAdapter()
	if err := Register(a, func(Options) (Provider, error) { return stubProvider{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{ServiceName: "svc"}); err != nil {
				b.Fatalf("Open err = %v", err)
			}
		}
	})
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = ParseAdapter("otel")
	}
}

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
