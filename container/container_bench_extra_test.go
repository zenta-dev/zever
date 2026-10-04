package container

import (
	"testing"

	"github.com/zenta-dev/zever/config"
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
	registerTestAdapters()
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
	registerTestAdapters()
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
	registerTestAdapters()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New(config.Default())
		if _, err := c.Job(); err != nil {
			b.Fatalf("Job: %v", err)
		}
	}
}

// BenchmarkContainer_CloseEmpty measures the shutdown cost of a container
// that never resolved anything.
func BenchmarkContainer_CloseEmpty(b *testing.B) {
	registerTestAdapters()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New(config.Default())
		if err := c.Close(b.Context()); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}
