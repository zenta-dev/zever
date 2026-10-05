package ai

import (
	"testing"
	"time"
)

// benchAdapter registers a stub AI once and returns its adapter so Open can
// be measured without paying the (one-shot) registration cost.
func benchAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (AI, error) { return &stubAI{}, nil }); err != nil {
		b.Fatalf("Register(%v) err = %v", a, err)
	}

	return a
}

// BenchmarkRegister measures the registry registration hot path.
func BenchmarkRegister(b *testing.B) {
	factory := func(Options) (AI, error) { return &stubAI{}, nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		a := freshAdapter()
		if err := Register(a, factory); err != nil {
			b.Fatalf("Register(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpen measures the registry lookup plus factory invocation hot path.
func BenchmarkOpen(b *testing.B) {
	a := benchAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpenParallel measures concurrent Open calls on one adapter.
func BenchmarkOpenParallel(b *testing.B) {
	a := benchAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Errorf("Open(%v) err = %v", a, err)
				return
			}
		}
	})
}

// BenchmarkOptionsValidate measures options validation with a BaseURL present.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{BaseURL: "https://api.openai.com/v1", Timeout: 30 * time.Second}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() err = %v", err)
		}
	}
}

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ParseAdapter("anthropic"); err != nil {
			b.Fatalf("ParseAdapter() err = %v", err)
		}
	}
}

// BenchmarkAdapterString measures the adapter name rendering hot path.
func BenchmarkAdapterString(b *testing.B) {
	a := Anthropic

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = a.String()
	}
}
