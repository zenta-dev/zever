package i18n

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
	for i := 0; i < b.N; i++ {
		if err := Register(benchFreshAdapter(), func(Options) (I18n, error) { return &stubI18n{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := benchFreshAdapter()
	if err := Register(a, func(Options) (I18n, error) { return &stubI18n{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := Open(a, Options{})
		if err != nil {
			b.Fatalf("Open err = %v", err)
		}
		if err := n.Close(); err != nil {
			b.Fatalf("Close err = %v", err)
		}
	}
}

func BenchmarkAdapter_String(b *testing.B) {
	a := Adapter("bench-adapter")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.String()
	}
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseAdapter("bench-adapter")
	}
}

func BenchmarkOptions_Validate(b *testing.B) {
	o := Options{Remote: RemoteOptions{Endpoint: "https://example.com/x", Timeout: 5e9, MaxInFlight: 100}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := o.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}
