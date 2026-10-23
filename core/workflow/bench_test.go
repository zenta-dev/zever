package workflow_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

func BenchmarkOptionsValidate(b *testing.B) {
	opts := workflow.Options{HostPort: "localhost:7233", Namespace: "default"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
