package scheduler

import (
	"fmt"
	"sync/atomic"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/job"
)

var benchSeq atomic.Int64

func benchAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", 3000+int(benchSeq.Add(1))))
}

func benchOptions() Options {
	return Options{Dispatcher: &job.Dispatcher{Q: &stubQueue{}}}
}

func BenchmarkOpen(b *testing.B) {
	a := benchAdapter()
	if err := Register(a, func(Options) (Scheduler, error) { return fakeScheduler{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	opts := benchOptions()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, opts); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenShared(b *testing.B) {
	a := benchAdapter()
	if err := RegisterShared(a, func(_ coredb.DB, _ Options) (Scheduler, error) { return fakeScheduler{}, nil }); err != nil {
		b.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	opts := benchOptions()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := OpenShared(a, nil, opts); err != nil {
			b.Fatalf("OpenShared(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := benchOptions()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
