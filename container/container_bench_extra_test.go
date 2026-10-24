package container

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/shared/grpcclient"
)

// BenchmarkContainer_NewNilConfig measures the lazy root constructor: no
// service opens, only the config fallback and pool registry allocation.
func BenchmarkContainer_NewNilConfig(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if New(nil) == nil {
			b.Fatal("nil container")
		}
	}
}

// BenchmarkContainer_ResolvePlugin measures the plugin resolution path
// (registry lookup, per-container slot creation, typed assertion) from a
// cold container.
func BenchmarkContainer_ResolvePlugin(b *testing.B) {
	name := mustUniquePluginName(b, "bench-widget")
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		return &fakePluginWidget{label: "bench"}, nil
	}); err != nil {
		b.Fatalf("RegisterPlugin: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New(config.Default())
		if _, err := Resolve[*fakePluginWidget](c, name); err != nil {
			b.Fatalf("Resolve: %v", err)
		}
	}
}

// BenchmarkContainer_ResolvePluginParallel measures the plugin fast path
// under concurrent callers; Resolve takes pluginsMu on every call, so this
// captures the steady-state contention.
func BenchmarkContainer_ResolvePluginParallel(b *testing.B) {
	name := mustUniquePluginName(b, "bench-widget-par")
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		return &fakePluginWidget{label: "bench"}, nil
	}); err != nil {
		b.Fatalf("RegisterPlugin: %v", err)
	}
	c := New(config.Default())
	if _, err := Resolve[*fakePluginWidget](c, name); err != nil {
		b.Fatalf("warmup Resolve: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Resolve[*fakePluginWidget](c, name); err != nil {
				b.Fatalf("Resolve: %v", err)
			}
		}
	})
}

// BenchmarkContainer_JobResolve measures resolving the job dispatcher, which
// resolves and shares the queue as a side effect.
func BenchmarkContainer_JobResolve(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New(nil, Provide(queue.DefaultRef(), newFakeQueue))
		if _, err := c.Job(); err != nil {
			b.Fatalf("Job: %v", err)
		}
	}
}

// BenchmarkContainer_CloseEmpty measures the shutdown cost of a container
// that never resolved anything.
func BenchmarkContainer_CloseEmpty(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New(nil)
		if err := c.Close(b.Context()); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}

// BenchmarkGRPCClient_CachedParallel measures the steady-state GRPCClient
// fast path (one shared cache, one map hit) under concurrent callers.
func BenchmarkGRPCClient_CachedParallel(b *testing.B) {
	c := New(nil)
	conn, err := c.GRPCClient("passthrough:///bench-cached", grpcclient.WithInsecure())
	if err != nil {
		b.Fatalf("warmup GRPCClient: %v", err)
	}
	if conn == nil {
		b.Fatal("warmup returned nil conn")
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.Close(ctx); err != nil {
			b.Fatalf("Close: %v", err)
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			got, err := c.GRPCClient("passthrough:///bench-cached", grpcclient.WithInsecure())
			if err != nil {
				b.Fatalf("GRPCClient: %v", err)
			}
			if got != conn {
				b.Fatal("GRPCClient returned a new conn for a cached target")
			}
		}
	})
}
