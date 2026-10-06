package fcm

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"firebase.google.com/go/v4/messaging"

	"github.com/zenta-dev/zever/core/notification"
)

func TestNotify_contextCanceled_skipsSend(t *testing.T) {
	t.Parallel()
	calls := 0
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		calls++
		return "id", nil
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := n.Notify(ctx, validPush())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Notify(canceled) = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Errorf("send called %d times, want 0", calls)
	}
}

func TestNotify_cancelDuringRetry_abortsWithoutExhausting(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	calls := 0
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		calls++
		cancel()
		return "", errors.New("boom")
	}}

	err := n.Notify(ctx, validPush())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Notify() = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("send called %d times, want 1 (ctx cancel aborts before retry)", calls)
	}
}

func TestNotify_emptyDataTableStaysNil(t *testing.T) {
	t.Parallel()
	var got *messaging.Message
	n := &notifier{send: func(_ context.Context, m *messaging.Message) (string, error) {
		got = m
		return "id", nil
	}}
	in := validPush()
	in.Data = map[string]string{}
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.Data != nil {
		t.Errorf("Data = %v, want nil for empty map", got.Data)
	}
}

func TestNotify_largeTTL_setsApnsExpiration(t *testing.T) {
	t.Parallel()
	var got *messaging.Message
	n := &notifier{send: func(_ context.Context, m *messaging.Message) (string, error) {
		got = m
		return "id", nil
	}}
	in := validPush()
	in.TTL = 365 * 24 * time.Hour
	before := time.Now()
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.Android == nil || got.Android.TTL == nil || *got.Android.TTL != in.TTL {
		t.Errorf("Android.TTL = %v, want %v", got.Android, in.TTL)
	}
	exp, err := strconv.ParseInt(got.APNS.Headers["apns-expiration"], 10, 64)
	if err != nil {
		t.Fatalf("apns-expiration = %q, not a unix int: %v", got.APNS.Headers["apns-expiration"], err)
	}
	want := before.Add(in.TTL).Unix()
	if d := exp - want; d < -120 || d > 120 {
		t.Errorf("apns-expiration = %d, want ~%d", exp, want)
	}
}

func TestNotify_lowPriority_mapsNormal(t *testing.T) {
	t.Parallel()
	var got *messaging.Message
	n := &notifier{send: func(_ context.Context, m *messaging.Message) (string, error) {
		got = m
		return "id", nil
	}}
	in := validPush()
	in.Priority = notification.PriorityLow
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.Android == nil || got.Android.Priority != "normal" {
		t.Errorf("Android = %+v, want priority normal", got.Android)
	}
	if got.APNS == nil || got.APNS.Headers["apns-priority"] != "5" {
		t.Errorf("APNS = %+v, want apns-priority 5", got.APNS)
	}
}

func TestNotify_nilClient_noSendFunc_returnsNotConfigured(t *testing.T) {
	t.Parallel()
	n := &notifier{}
	if err := n.Notify(t.Context(), validPush()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Notify() = %v, want ErrNotConfigured", err)
	}
}

func TestNotify_concurrentSafe(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		calls.Add(1)
		return "id", nil
	}}
	const workers = 8
	const perWorker = 25
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if err := n.Notify(t.Context(), validPush()); err != nil {
					errs[w] = err
					return
				}
			}
		}()
	}
	wg.Wait()
	for w, err := range errs {
		if err != nil {
			t.Errorf("worker %d Notify() = %v, want nil", w, err)
		}
	}
	if got := calls.Load(); got != workers*perWorker {
		t.Errorf("send calls = %d, want %d", got, workers*perWorker)
	}
}

func TestNotify_emptyTarget_rejects(t *testing.T) {
	t.Parallel()

	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		return "id", nil
	}}

	in := validPush()
	in.Target = ""

	if err := n.Notify(t.Context(), in); !errors.Is(err, notification.ErrInvalidTarget) {
		t.Errorf("Notify(empty target) = %v, want ErrInvalidTarget", err)
	}
}

func TestNotify_ttlBoundary_setsApnsExpiration(t *testing.T) {
	t.Parallel()

	var got *messaging.Message
	n := &notifier{send: func(_ context.Context, m *messaging.Message) (string, error) {
		got = m
		return "id", nil
	}}
	in := validPush()
	in.TTL = time.Nanosecond
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.Android == nil || got.Android.TTL == nil || *got.Android.TTL != in.TTL {
		t.Errorf("Android.TTL = %v, want %v", got.Android, in.TTL)
	}
	if _, ok := got.APNS.Headers["apns-expiration"]; !ok {
		t.Errorf("APNS headers = %v, want apns-expiration set", got.APNS.Headers)
	}
}

func TestInvalidOptions_isAndAs(t *testing.T) {
	t.Parallel()

	err := invalidOptions("bad path")
	if err == nil {
		t.Fatal("invalidOptions() = nil, want error")
	}

	if !errors.Is(err, notification.InvalidOptionsError{Reason: "bad path"}) {
		t.Errorf("errors.Is(InvalidOptionsError) = false, want true")
	}

	var target notification.InvalidOptionsError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(InvalidOptionsError) = false, want true")
	}

	if target.Reason != "bad path" {
		t.Errorf("Reason = %q, want bad path", target.Reason)
	}
}

func TestNotify_highPriorityWithDataAndTTL(t *testing.T) {
	t.Parallel()

	var got *messaging.Message
	n := &notifier{send: func(_ context.Context, m *messaging.Message) (string, error) {
		got = m
		return "id", nil
	}}
	in := validPush()
	in.Priority = notification.PriorityHigh
	in.TTL = 30 * time.Minute
	in.Data = map[string]string{"order": "42"}
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.Data["order"] != "42" {
		t.Errorf("Data = %v, want order=42", got.Data)
	}
	if got.Android == nil || got.Android.Priority != "high" {
		t.Errorf("Android = %+v, want priority high", got.Android)
	}
	if got.Android.TTL == nil || *got.Android.TTL != in.TTL {
		t.Errorf("Android.TTL = %v, want %v", got.Android, in.TTL)
	}
}
