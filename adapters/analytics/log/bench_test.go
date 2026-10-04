package log

import (
	"io"
	"log/slog"
	"testing"

	"github.com/zenta-dev/zever/core/analytics"
)

// benchAdapter returns an adapter whose JSON handler discards output, so the
// benchmark measures event mapping without unbounded buffer growth.
func benchAdapter() *adapter {
	return &adapter{
		logger:             slog.New(slog.NewJSONHandler(io.Discard, nil)),
		anonymousID:        analytics.DefaultAnonymousID,
		maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
	}
}

func benchProperties() map[string]any {
	return map[string]any{
		"plan":   "pro",
		"amount": 42,
		"tags":   []string{"a", "b"},
	}
}

// BenchmarkTrack measures a Track event render.
func BenchmarkTrack(b *testing.B) {
	a := benchAdapter()
	ctx := b.Context()
	props := benchProperties()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := a.Track(ctx, "signup", props); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIdentify measures an Identify event render.
func BenchmarkIdentify(b *testing.B) {
	a := benchAdapter()
	ctx := b.Context()
	traits := benchProperties()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := a.Identify(ctx, "user-1", traits); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGroup measures a Group event render.
func BenchmarkGroup(b *testing.B) {
	a := benchAdapter()
	ctx := b.Context()
	traits := benchProperties()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := a.Group(ctx, "user-1", "group-1", traits); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTrackParallel measures Track throughput under concurrent load. The
// slog JSON handler and io.Discard are safe for concurrent use.
func BenchmarkTrackParallel(b *testing.B) {
	a := benchAdapter()
	ctx := b.Context()
	props := benchProperties()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := a.Track(ctx, "signup", props); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
