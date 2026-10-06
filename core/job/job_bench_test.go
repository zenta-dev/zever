package job

import (
	"context"
	"strconv"
	"testing"
	"time"
)

type benchArgs struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func BenchmarkRegister(b *testing.B) {
	Reset()
	noop := func(context.Context, benchArgs) error { return nil }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if err := Register("bench-"+strconv.Itoa(i), noop); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkLookup(b *testing.B) {
	Reset()
	if err := Register("bench-lookup", func(context.Context, benchArgs) error { return nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, ok := Lookup("bench-lookup"); !ok {
			b.Fatal("Lookup miss")
		}
	}
}

func BenchmarkLookupParallel(b *testing.B) {
	Reset()
	if err := Register("bench-lookup-par", func(context.Context, benchArgs) error { return nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, ok := Lookup("bench-lookup-par"); !ok {
				b.Fatal("Lookup miss")
			}
		}
	})
}

func BenchmarkPriorityString(b *testing.B) {
	var p Priority = PriorityHigh
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = p.String()
	}
}

func BenchmarkRetryPolicyBackoff(b *testing.B) {
	p := DefaultRetryPolicy()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = p.Backoff(10)
	}
}

func BenchmarkDispatch(b *testing.B) {
	Reset()
	if err := Register("bench-dispatch", func(context.Context, benchArgs) error { return nil }, WithPriority(PriorityHigh)); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	d := &Dispatcher{Q: &stubQueue{}}
	args := benchArgs{ID: 1, Name: "x"}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := d.Dispatch(ctx, "bench-dispatch", args); err != nil {
			b.Fatalf("Dispatch err = %v", err)
		}
	}
}

func BenchmarkUse(b *testing.B) {
	Reset()
	mw := func(next Handler) Handler { return next }
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := Use(mw); err != nil {
			b.Fatalf("Use err = %v", err)
		}
	}
}

func BenchmarkUniqueID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = uniqueID("bench-job", "unique-key")
	}
}

func BenchmarkDispatchDelayed(b *testing.B) {
	Reset()
	if err := Register("bench-dispatch-delayed", func(context.Context, benchArgs) error { return nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	d := &Dispatcher{Q: &stubQueue{}}
	args := benchArgs{ID: 1}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := d.Dispatch(ctx, "bench-dispatch-delayed", args, In(time.Hour)); err != nil {
			b.Fatalf("Dispatch err = %v", err)
		}
	}
}
