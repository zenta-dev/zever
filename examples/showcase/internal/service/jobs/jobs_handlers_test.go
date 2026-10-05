package jobs_test

import (
	"testing"

	"github.com/zenta-dev/zever/examples/showcase/internal/service/jobs"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/seed"
)

// TestHandleSendConfirmation proves the handler decodes the order_id payload
// and delegates to RunConfirmation.
func TestHandleSendConfirmation(t *testing.T) {
	ctx, deps := testDeps(t)
	handle := jobs.HandleSendConfirmation(deps)

	if err := handle(ctx, jobs.SendConfirmationArgs{OrderID: seed.OrderID}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if err := handle(ctx, jobs.SendConfirmationArgs{}); err == nil {
		t.Fatal("handler empty order id: want error")
	}
}

// TestHandleProcessOrder proves the handler delegates to RunProcessOrder.
func TestHandleProcessOrder(t *testing.T) {
	ctx, deps := testDeps(t)
	handle := jobs.HandleProcessOrder(deps)

	if err := handle(ctx, jobs.ProcessOrderArgs{OrderID: seed.OrderID}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if err := handle(ctx, jobs.ProcessOrderArgs{OrderID: "missing"}); err == nil {
		t.Fatal("handler missing order: want error")
	}
}

// TestHandleScheduledJobs covers the two no-argument scheduled handlers.
func TestHandleScheduledJobs(t *testing.T) {
	ctx, deps := testDeps(t)

	if err := jobs.HandleReindexSearch(deps)(ctx, struct{}{}); err != nil {
		t.Fatalf("HandleReindexSearch: %v", err)
	}

	if err := jobs.HandleGenerateDailyReport(deps)(ctx, struct{}{}); err != nil {
		t.Fatalf("HandleGenerateDailyReport: %v", err)
	}
}
