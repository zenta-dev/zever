package log

import (
	"testing"
	"time"
)

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
