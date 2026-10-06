package db

import (
	"context"
	"os"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

// TestPostgresLive_Saga exercises saga execution end to end against a live
// postgres. Set POSTGRES_DSN to run; skipped otherwise.
func TestPostgresLive_Saga(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres live tests")
	}

	d := mustNew(t, Options{DSN: dsn, Owner: "owner-saga-live"})
	ctx := t.Context()

	t.Cleanup(func() {
		_, _ = d.conn.Exec(ctx, `DROP TABLE IF EXISTS "`+DefaultSagaTable+`"`)
		_, _ = d.conn.Exec(ctx, `DROP TABLE IF EXISTS "`+DefaultSagaCompensationsTable+`"`)
	})

	d.RegisterSaga("live-saga", []workflow.SagaStep{
		{Name: "a", Execute: func(_ context.Context, input any) (any, error) {
			return input, nil
		}},
		{Name: "b", Execute: func(_ context.Context, input any) (any, error) {
			return input, nil
		}},
	})

	id, err := d.RunSaga(ctx, "live-saga", "hello", "wf-live-saga")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st, err := d.SagaStatus(ctx, id)
	if err != nil {
		t.Fatalf("SagaStatus failed: %v", err)
	}

	if st.Status != workflow.SagaCompleted {
		t.Fatalf("status = %q, want %q", st.Status, workflow.SagaCompleted)
	}

	if st.CurrentStep != 2 {
		t.Errorf("currentStep = %d, want 2", st.CurrentStep)
	}
}
