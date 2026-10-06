package sqlite

import (
	"context"
	"testing"

	orm "github.com/zenta-dev/zever/orm"
)

func BenchmarkMatchRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).Where(Match(docBody, "distributed systems")).All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRankOrderRender(b *testing.B) {
	ctx := context.Background()
	capture := &captureExec{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := orm.From(docs).
			Where(Match(docBody, "go")).
			OrderBy(Rank(docBody)).
			All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}
