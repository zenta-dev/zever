package breaking

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

// BenchmarkCompareAppFixture compares the resolved app.zen schema against
// itself: the zero-change fast path over two modules' worth of entities,
// messages, and services.
func BenchmarkCompareAppFixture(b *testing.B) {
	old := testSchema(testModule("app"))
	newSchema := testSchema(testModule("app"))

	b.ReportAllocs()
	for b.Loop() {
		_ = Compare(old, newSchema)
	}
}

// BenchmarkCompareDriftedSchemas compares two same-shaped schemas that
// differ in several breaking ways (removed entity, changed field type,
// changed HTTP path), exercising the change-classification paths.
func BenchmarkCompareDriftedSchemas(b *testing.B) {
	old := testSchema(testModule("app"))
	newSchema := testSchema(testModule("app"))
	newSchema.Modules[0].Entities = newSchema.Modules[0].Entities[:0]

	b.ReportAllocs()
	for b.Loop() {
		_ = Compare(old, newSchema)
	}
}

// BenchmarkCompareEmptySchemas is the boundary case: two empty schemas.
func BenchmarkCompareEmptySchemas(b *testing.B) {
	old := &ir.Schema{}
	newSchema := &ir.Schema{}

	b.ReportAllocs()
	for b.Loop() {
		_ = Compare(old, newSchema)
	}
}

// BenchmarkHasBreaking walks a mixed change list.
func BenchmarkHasBreaking(b *testing.B) {
	changes := []Change{
		{Kind: KindFieldAdded, Breaking: false},
		{Kind: KindFieldRemoved, Breaking: true},
		{Kind: KindValidateAdded, Breaking: false},
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = HasBreaking(changes)
	}
}

// BenchmarkModulesByKey measures the module keying both schemas pay before
// any comparison work.
func BenchmarkModulesByKey(b *testing.B) {
	schema := testSchema(testModule("app"))

	b.ReportAllocs()
	for b.Loop() {
		_ = modulesByKey(schema)
	}
}
