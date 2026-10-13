package render

import (
	"strconv"
	"testing"

	sqlited "github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// BenchmarkShapeCacheHit measures the shape-cache hit path: one warm shape
// rendered per iteration, so every iteration is a cache hit plus arg
// collection and no text build.
func BenchmarkShapeCacheHit(b *testing.B) {
	d := sqlited.New()
	cols := []string{"id", "name", "quantity"}
	order := []OrderTerm{{Column: "id"}}

	if _, _, err := Select(d, "widgets", cols, Node{}, order, 0, 0); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, _, err := Select(d, "widgets", cols, Node{}, order, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkShapeCacheHitParallel measures the hit path under concurrent
// load across many distinct warm shapes, so lock behavior on the cache
// dominates rather than a single shared entry.
func BenchmarkShapeCacheHitParallel(b *testing.B) {
	d := sqlited.New()

	const shapes = 64

	for i := 0; i < shapes; i++ {
		cols := []string{"id" + strconv.Itoa(i)}

		if _, _, err := Select(d, "widgets", cols, Node{}, nil, 0, 0); err != nil {
			b.Fatal(err)
		}
	}

	// b.RunParallel carries the benchmark's implicit context; the render
	// path takes no context, so no explicit b.Context() is needed here.
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			cols := []string{"id" + strconv.Itoa(i%shapes)}

			if _, _, err := Select(d, "widgets", cols, Node{}, nil, 0, 0); err != nil {
				b.Error(err)

				return
			}

			i++
		}
	})
}
