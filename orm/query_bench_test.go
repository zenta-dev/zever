package orm

import (
	"context"
	"testing"
)

func benchCapture() *edgeCaptureDB { return &edgeCaptureDB{dialect: "sqlite"} }

func BenchmarkQuerySelectRender(b *testing.B) {
	ctx := context.Background()
	capture := benchCapture()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := From(widgets).
			Where(widgetName.Eq("alpha")).
			OrderBy(widgetID.Asc()).
			Limit(10).
			All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuerySelectJoinRender(b *testing.B) {
	ctx := context.Background()
	capture := benchCapture()
	rel := NewRelation[widget, widget]("id", "id", widgets)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := JoinOn(From(widgets), rel, InnerJoin).
			Where(widgetName.Eq("alpha")).
			All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQueryInsertRender(b *testing.B) {
	ctx := context.Background()
	capture := benchCapture()

	b.ReportAllocs()
	for b.Loop() {
		if err := InsertInto(widgets).
			Values(Set(widgetID, "w9"), Set(widgetName, "Delta"), Set(widgetQty, 40)).
			Exec(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQueryUpdateRender(b *testing.B) {
	ctx := context.Background()
	capture := benchCapture()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := UpdateTable(widgets).
			Set(Set(widgetName, "Delta")).
			Where(widgetID.Eq("w1")).
			Exec(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQueryDeleteRender(b *testing.B) {
	ctx := context.Background()
	capture := benchCapture()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := DeleteFrom(widgets).
			Where(widgetID.Eq("w1")).
			Exec(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQueryCursorNextPageRender(b *testing.B) {
	ctx := context.Background()
	capture := benchCapture()

	b.ReportAllocs()
	for b.Loop() {
		q := From(widgets).OrderBy(widgetID.Asc()).Limit(10)
		if _, err := NextPage(q, NewCursorKey(widgetID, "w1")).All(ctx, capture); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQueryPreloadRender(b *testing.B) {
	ctx := context.Background()
	capture := benchCapture()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Preload(ctx, capture, From(widgets), widgetID, From(widgets), func(*widget) string { return "" }, func(*widget) string { return "" }); err != nil {
			b.Fatal(err)
		}
	}
}
