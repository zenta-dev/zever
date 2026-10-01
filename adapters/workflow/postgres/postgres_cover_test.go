package postgres

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

// TestSignal_Cancel_RaceCompletionFailClosed ensures Signal/Cancel against a
// completed or vanished run never report silent success.
func TestSignal_Cancel_RaceCompletionFailClosed(t *testing.T) {
	t.Parallel()
	d := mustNew(t, Options{Owner: "owner-race"})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", "hi", "")
	if err != nil {
		t.Fatalf("Start err = %v", err)
	}
	// Start completes synchronously, so Signal must now fail closed.
	if err := d.Signal(ctx, id, "advance", "late"); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Signal completed = %v, want ErrRunCompleted", err)
	}
	if err := d.Cancel(ctx, id); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Cancel completed = %v, want ErrRunCompleted", err)
	}
	// Unknown run must not report success.
	if err := d.Signal(ctx, "run-does-not-exist", "advance", "x"); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Signal unknown = %v, want ErrUnknownRun", err)
	}
	if err := d.Cancel(ctx, "run-does-not-exist"); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Cancel unknown = %v, want ErrUnknownRun", err)
	}
}
