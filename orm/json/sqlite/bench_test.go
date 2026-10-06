package sqlite

import (
	"context"
	"testing"

	orm "github.com/zenta-dev/zever/orm"
)

func BenchmarkExtractTextRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).Where(ExtractText(docData, "name").Eq("alice")).All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKeyExistsRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).Where(KeyExists(docData, "meta")).All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkArrayContainsRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).Where(ArrayContains(docData, "go")).All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkChainedExtractRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).
			Where(JSONPath(docData).Extract("tags").ExtractIndexText(0).Eq("go")).
			All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}
