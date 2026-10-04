package posthog

import (
	"testing"

	"github.com/zenta-dev/zever/core/analytics"
)

// benchAdapter returns an adapter wired to a fake client that records
// enqueued messages without touching the network. The fake client is
// mutex-guarded, so it is safe for the parallel benchmark.
func benchAdapter() *adapter {
	f := &fakeClient{}

	return &adapter{
		client:             f,
		anonymousID:        "anonymous",
		groupType:          "organization",
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

// BenchmarkTrack measures event mapping and enqueue.
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

// BenchmarkIdentify measures trait mapping and enqueue.
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

// BenchmarkGroup measures group-identify mapping and enqueue.
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

// BenchmarkTrackParallel measures Track throughput under concurrent load.
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
