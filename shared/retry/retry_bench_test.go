package retry

import (
	"context"
	"testing"
	"time"
)

// BenchmarkNextDelayExponential measures exponential backoff computation.
func BenchmarkNextDelayExponential(b *testing.B) {
	p := Policy{BaseDelay: time.Millisecond, Multiplier: 2, MaxDelay: time.Minute}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		_ = p.NextDelay(i%30 + 1)
		i++
	}
}

// BenchmarkNextDelayLinear measures linear backoff computation.
func BenchmarkNextDelayLinear(b *testing.B) {
	p := Policy{BaseDelay: time.Millisecond, Linear: true, MaxDelay: time.Minute}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		_ = p.NextDelay(i%30 + 1)
		i++
	}
}

// BenchmarkNextDelayJitter measures backoff with symmetric jitter.
func BenchmarkNextDelayJitter(b *testing.B) {
	p := Policy{BaseDelay: time.Millisecond, Multiplier: 2, MaxDelay: time.Minute, Jitter: 0.2}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		_ = p.NextDelay(i%30 + 1)
		i++
	}
}

// BenchmarkNextDelayJitterParallel measures jitter under concurrent callers.
func BenchmarkNextDelayJitterParallel(b *testing.B) {
	p := Policy{BaseDelay: time.Millisecond, Multiplier: 2, MaxDelay: time.Minute, Jitter: 0.2}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.NextDelay(3)
		}
	})
}

// BenchmarkParseRetryAfterSeconds measures parsing the delay-seconds form.
func BenchmarkParseRetryAfterSeconds(b *testing.B) {
	now := time.Unix(0, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = ParseRetryAfter("120", now)
	}
}

// BenchmarkParseRetryAfterHTTPDate measures parsing the HTTP-date form.
func BenchmarkParseRetryAfterHTTPDate(b *testing.B) {
	now := time.Date(2026, time.October, 21, 7, 26, 0, 0, time.UTC)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = ParseRetryAfter("Wed, 21 Oct 2026 07:28:00 GMT", now)
	}
}

// BenchmarkParseRetryAfterMs measures parsing the millisecond form.
func BenchmarkParseRetryAfterMs(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = ParseRetryAfterMs("120000")
	}
}

// BenchmarkDoImmediateSuccess measures the retry loop with a first-try success.
func BenchmarkDoImmediateSuccess(b *testing.B) {
	ctx := b.Context()
	p := Policy{MaxAttempts: 3, BaseDelay: time.Millisecond}
	fn := func(context.Context) error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := Do(ctx, p, fn); err != nil {
			b.Fatalf("Do() error = %v", err)
		}
	}
}
