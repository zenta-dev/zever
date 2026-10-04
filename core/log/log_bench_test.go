package log

import (
	"testing"
	"time"
)

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := Register(freshAdapter(), func(Options) (Logger, error) { return stubLogger{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Logger, error) { return stubLogger{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Logger, error) { return stubLogger{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Fatalf("Open err = %v", err)
			}
		}
	})
}

func BenchmarkParseLevel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := ParseLevel("warn"); err != nil {
			b.Fatalf("ParseLevel err = %v", err)
		}
	}
}

func BenchmarkLevelString(b *testing.B) {
	l := LevelWarn
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = l.String()
	}
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = ParseAdapter("slog")
	}
}

func BenchmarkFieldConstructors(b *testing.B) {
	now := time.Unix(0, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = String("k", "v")
		_ = Int("n", 1)
		_ = Int64("n64", 2)
		_ = Float64("f", 1.5)
		_ = Bool("b", true)
		_ = Duration("d", time.Second)
		_ = Time("t", now)
		_ = Err(nil)
		_ = Any("a", struct{}{})
	}
}

func BenchmarkFromContext(b *testing.B) {
	ctx := ContextWithLogger(b.Context(), stubLogger{})
	fallback := stubLogger{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = FromContext(ctx, fallback)
	}
}
