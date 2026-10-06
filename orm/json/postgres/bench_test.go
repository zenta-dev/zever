package postgres

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

func BenchmarkContainsRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).Where(Contains(docData, `{"role":"admin"}`)).All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPathExtractRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).
			Where(JSONPath(docData).PathExtract("a", "b").Eq(`{}`)).
			All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKeyExistsAnyRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).Where(KeyExistsAny(docData, "a", "b", "c")).All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}
