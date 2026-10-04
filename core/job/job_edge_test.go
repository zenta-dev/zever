package job

import (
	"context"
	"testing"
	"time"
)

func TestDispatch_nonPositiveDelay_usesImmediatePush(t *testing.T) {
	Reset()

	if err := Register("edge-immediate", func(context.Context, benchArgs) error { return nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	sq := &stubQueue{}
	d := &Dispatcher{Q: sq}

	if err := d.Dispatch(t.Context(), "edge-immediate", benchArgs{ID: 1}, In(-time.Second)); err != nil {
		t.Fatalf("Dispatch err = %v", err)
	}

	if sq.pushes != 1 || sq.delayedPushes != 0 {
		t.Fatalf("pushes=%d delayedPushes=%d, want 1/0", sq.pushes, sq.delayedPushes)
	}
}

func TestDispatch_delayOption_overridesImmediate(t *testing.T) {
	Reset()

	if err := Register("edge-delayed", func(context.Context, benchArgs) error { return nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	sq := &stubQueue{}
	d := &Dispatcher{Q: sq}

	if err := d.Dispatch(t.Context(), "edge-delayed", benchArgs{ID: 1}, In(time.Minute)); err != nil {
		t.Fatalf("Dispatch err = %v", err)
	}

	if sq.pushes != 0 || sq.delayedPushes != 1 {
		t.Fatalf("pushes=%d delayedPushes=%d, want 0/1", sq.pushes, sq.delayedPushes)
	}
}
