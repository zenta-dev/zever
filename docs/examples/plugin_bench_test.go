package examples_test

import (
	"testing"

	"github.com/zenta-dev/zever/container"
)

// BenchmarkPluginResolve measures the container plugin fast path after the
// greeter plugin is registered and warmed.
func BenchmarkPluginResolve(b *testing.B) {
	_ = registerGreetPlugin() // duplicate registration on a warm registry is fine

	c := container.New(nil)
	if _, err := container.Resolve[greeter](c, greetPluginName); err != nil {
		b.Fatalf("warmup Resolve: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := container.Resolve[greeter](c, greetPluginName); err != nil {
			b.Fatalf("Resolve: %v", err)
		}
	}
}
