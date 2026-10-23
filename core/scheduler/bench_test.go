package scheduler

import (
	"testing"

	"github.com/zenta-dev/zever/core/job"
)

func benchOptions() Options {
	return Options{Dispatcher: &job.Dispatcher{Q: &stubQueue{}}}
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
