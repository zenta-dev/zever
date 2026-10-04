package jobs_test

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/examples/todo/internal/service/jobs"
)

// TestCountOverdueEmpty pins the empty-store boundary: counting overdue notes
// on a migrated but unseeded database returns zero.
func TestCountOverdueEmpty(t *testing.T) {
	c, ctx := newTestDB(t)

	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}

	n, err := jobs.CountOverdue(ctx, database, time.Now().UTC())
	if err != nil {
		t.Fatalf("CountOverdue: %v", err)
	}
	if n != 0 {
		t.Fatalf("CountOverdue = %d, want 0", n)
	}
}
