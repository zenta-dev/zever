package postgres

import (
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/core/workflow/workflowtest"
)

// TestConformancePostgres proves the DB-backed engine passes the workflow
// kit against a file-backed sqlite database.
func TestConformancePostgres(t *testing.T) {
	t.Parallel()

	workflowtest.Conformance(t, func(t *testing.T) workflow.Workflow {
		t.Helper()

		w, err := New(Options{Path: filepath.Join(t.TempDir(), "workflow-kit.db")})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		d, ok := w.(*driver)
		if !ok {
			t.Fatalf("New() returned %T, want *driver", w)
		}

		t.Cleanup(func() { _ = w.Close() })

		return d
	})
}
