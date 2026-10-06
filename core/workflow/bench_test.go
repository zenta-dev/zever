package workflow_test

import (
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
)

// benchWorkflowAdapter registers a stub workflow once and returns its adapter
// so Open can be measured without the one-shot registration cost.
func benchWorkflowAdapter(b *testing.B) workflow.Adapter {
	b.Helper()

	a := freshWorkflowAdapter()
	if err := workflow.Register(a, func(workflow.Options) (workflow.Workflow, error) { return stubWorkflow{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchWorkflowAdapter(b)
	opts := workflow.Options{HostPort: "localhost:7233", Namespace: "default"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := workflow.Open(a, opts); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenShared(b *testing.B) {
	a := freshWorkflowAdapter()
	if err := workflow.RegisterShared(a, func(coredb.DB, workflow.Options) (workflow.Workflow, error) {
		return stubWorkflow{}, nil
	}); err != nil {
		b.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	opts := workflow.Options{HostPort: "localhost:7233"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := workflow.OpenShared(a, nil, opts); err != nil {
			b.Fatalf("OpenShared(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchWorkflowAdapter(b)
	opts := workflow.Options{HostPort: "localhost:7233"}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := workflow.Open(a, opts); err != nil {
				b.Errorf("Open(%v) error = %v", a, err)
				return
			}
		}
	})
}

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
