package jobs_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/examples/demoapp/internal/service/jobs"
)

// TestRunWelcomeEmailBoundaries pins empty-email error and empty-name fallback.
func TestRunWelcomeEmailBoundaries(t *testing.T) {
	ctx, deps, mailBuf, notifyBuf := newDeps(t)
	_ = notifyBuf

	if err := jobs.RunWelcomeEmail(ctx, deps, "", "Ann"); err == nil {
		t.Fatal("empty email: want error")
	}
	// Empty name falls back to the email address and still sends.
	mailBuf.Reset()
	if err := jobs.RunWelcomeEmail(ctx, deps, "ann@example.com", ""); err != nil {
		t.Fatalf("empty name fallback: %v", err)
	}
	if mailBuf.Len() == 0 {
		t.Fatal("mail buffer empty, want welcome mail")
	}
}

// TestRunProcessOrderUnknown pins unknown-order error path.
func TestRunProcessOrderUnknown(t *testing.T) {
	ctx, deps, mailBuf, notifyBuf := newDeps(t)
	_ = mailBuf
	_ = notifyBuf

	if err := jobs.RunProcessOrder(ctx, deps, ""); err == nil {
		t.Fatal("empty order id: want error")
	}
	if err := jobs.RunProcessOrder(ctx, deps, "does-not-exist"); !errors.Is(err, jobs.ErrOrderNotFound) {
		t.Fatalf("unknown order = %v, want ErrOrderNotFound", err)
	}
}

// TestRunReindexEmpty pins reindex over an empty store succeeds.
func TestRunReindexEmpty(t *testing.T) {
	ctx, deps, mailBuf, notifyBuf := newDeps(t)
	_ = mailBuf
	_ = notifyBuf

	if err := jobs.RunReindex(ctx, deps); err != nil {
		t.Fatalf("empty reindex: %v", err)
	}
}

// TestHandleUnknownJobArgs pins handler shape errors for missing ids.
func TestHandleUnknownJobArgs(t *testing.T) {
	ctx, deps, mailBuf, notifyBuf := newDeps(t)
	_ = mailBuf
	_ = notifyBuf

	if err := jobs.HandleProcessOrder(deps)(ctx, jobs.ProcessOrderArgs{OrderID: "missing"}); err == nil {
		t.Fatal("handler missing order: want error")
	}
	if err := jobs.HandleSendWelcomeEmail(deps)(ctx, jobs.SendWelcomeEmailArgs{Email: "", Name: ""}); err == nil {
		t.Fatal("handler empty welcome: want error")
	}
}
