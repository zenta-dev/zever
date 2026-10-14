package paddle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/payment"
)

const noIDBody = `{"event_type":"transaction.paid","data":{"id":"txn_wh1","status":"paid","currency_code":"USD","details":{"totals":{"total":"100.00"}}}}`

func mustHandling(t *testing.T) payment.WebhookHandling {
	t.Helper()

	wh, ok := mustReplayDriver(t, newWebhookStore(nil)).(payment.WebhookHandling)
	if !ok {
		t.Fatal("driver does not implement payment.WebhookHandling")
	}

	return wh
}

func sig(t *testing.T, body []byte) string {
	t.Helper()

	return signWebhook(t, "whsec_test", body, time.Now())
}

func TestHandleWebhook_handlerErrorAllowsRetry(t *testing.T) {
	t.Parallel()

	wh := mustHandling(t)
	body := []byte(replayBody)
	boom := errors.New("boom")

	err := wh.HandleWebhook(t.Context(), body, sig(t, body), func(context.Context, payment.Event) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("first err = %v, want boom", err)
	}

	calls := 0

	err = wh.HandleWebhook(t.Context(), body, sig(t, body), func(_ context.Context, ev payment.Event) error {
		calls++

		if ev.ID != "evt_1" {
			t.Errorf("ev.ID = %q, want evt_1", ev.ID)
		}

		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("retry err = %v calls = %d, want nil/1", err, calls)
	}
}

func TestHandleWebhook_successRejectsReplay(t *testing.T) {
	t.Parallel()

	wh := mustHandling(t)
	body := []byte(replayBody)

	if err := wh.HandleWebhook(t.Context(), body, sig(t, body), func(context.Context, payment.Event) error { return nil }); err != nil {
		t.Fatalf("first err = %v", err)
	}

	err := wh.HandleWebhook(t.Context(), body, sig(t, body), func(context.Context, payment.Event) error {
		t.Error("handler ran on replay")

		return nil
	})
	if !errors.Is(err, payment.ErrDuplicateEvent) {
		t.Fatalf("replay err = %v, want ErrDuplicateEvent", err)
	}
}

func TestHandleWebhook_panicReleasesKey(t *testing.T) {
	t.Parallel()

	wh := mustHandling(t)
	body := []byte(replayBody)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic not propagated")
			}
		}()

		_ = wh.HandleWebhook(t.Context(), body, sig(t, body), func(context.Context, payment.Event) error { panic("kaboom") })
	}()

	if err := wh.HandleWebhook(t.Context(), body, sig(t, body), func(context.Context, payment.Event) error { return nil }); err != nil {
		t.Fatalf("retry after panic err = %v, want nil", err)
	}
}

func TestHandleWebhook_inFlightDuplicate(t *testing.T) {
	t.Parallel()

	wh := mustHandling(t)
	body := []byte(replayBody)

	var inner error

	err := wh.HandleWebhook(t.Context(), body, sig(t, body), func(context.Context, payment.Event) error {
		inner = wh.HandleWebhook(t.Context(), body, sig(t, body), func(context.Context, payment.Event) error {
			t.Error("concurrent handler ran")

			return nil
		})

		return nil
	})
	if err != nil {
		t.Fatalf("outer err = %v", err)
	}

	if !errors.Is(inner, idempotency.ErrInProgress) {
		t.Fatalf("in-flight err = %v, want ErrInProgress", inner)
	}
}

func TestHandleWebhook_missingID(t *testing.T) {
	t.Parallel()

	wh := mustHandling(t)
	body := []byte(noIDBody)

	err := wh.HandleWebhook(t.Context(), body, sig(t, body), func(context.Context, payment.Event) error {
		t.Error("handler ran without an event id")

		return nil
	})
	if !errors.Is(err, payment.ErrMissingEventID) {
		t.Fatalf("err = %v, want ErrMissingEventID", err)
	}
}
