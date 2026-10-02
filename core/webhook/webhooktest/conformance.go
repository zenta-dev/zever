// Package webhooktest provides the conformance kit third-party webhook
// backends run to prove backend parity.
package webhooktest

import (
	"errors"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/webhook"
)

// adapterSeq keeps throwaway registry names unique across Conformance runs
// in one process.
var adapterSeq atomic.Uint64

// Conformance verifies factory-built webhooks implement the webhook.Webhook
// contract: Register/Unregister lifecycle, Deliver fan-out invocation,
// ErrNotFound sentinels, Open/Register registry wiring, and Close. Each
// subtest takes a fresh instance from factory so cases stay isolated. No
// test delivers to a live server: targets are syntactically valid private
// URLs that factories must accept with private targets allowed, and live
// fan-out belongs in ConformanceDelivery. Tests never touch the network.
func Conformance(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	t.Run("Lifecycle", func(t *testing.T) { conformanceLifecycle(t, factory) })
	t.Run("NotFound", func(t *testing.T) { conformanceNotFound(t, factory) })
	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

// ConformanceDelivery verifies live fan-out against target: Register,
// Deliver returning nil, Unregister, then Deliver failing with
// ErrNotFound. The caller owns target (an httptest server URL in adapter
// tests) and asserts receipt itself; the kit asserts the delivery
// contract only, so queue-riding adapters pass without a consumer.
func ConformanceDelivery(t *testing.T, w webhook.Webhook, event, target string) {
	t.Helper()

	ctx := t.Context()

	if err := w.Register(ctx, event, target, "kit-secret"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := w.Deliver(ctx, event, []byte(`{"kit":true}`)); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}

	if err := w.Unregister(ctx, event, target); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	if err := w.Deliver(ctx, event, []byte(`{}`)); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Deliver() after Unregister err = %v, want ErrNotFound", err)
	}
}

func conformanceLifecycle(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)

	const event = "kit.lifecycle"
	const target = "http://127.0.0.1/kit-hook"

	if err := w.Register(ctx, event, target, "s3cret"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Re-registering the same target replaces its secret, never duplicates.
	if err := w.Register(ctx, event, target, "s3cret-rotated"); err != nil {
		t.Fatalf("Register(same) error = %v", err)
	}

	if err := w.Unregister(ctx, event, target); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	if err := w.Unregister(ctx, event, target); !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("Unregister(again) err = %v, want ErrNotFound", err)
	}
}

func conformanceNotFound(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)

	if err := w.Unregister(ctx, "kit.no-event", "http://127.0.0.1/none"); !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("Unregister(missing) err = %v, want ErrNotFound", err)
	}

	if err := w.Deliver(ctx, "kit.no-event", []byte(`{}`)); !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("Deliver(missing) err = %v, want ErrNotFound", err)
	}

	if err := w.Register(ctx, "", "http://127.0.0.1/hook", "s"); err == nil {
		t.Error("Register(empty event) = nil, want error")
	}

	if err := w.Register(ctx, "kit.e", "", "s"); err == nil {
		t.Error("Register(empty target) = nil, want error")
	}
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	name := webhook.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))
	probe := factory(t)

	if err := webhook.Register(name, func(webhook.Options) (webhook.Webhook, error) { return probe, nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := webhook.Register(name, func(webhook.Options) (webhook.Webhook, error) { return probe, nil }); !errors.Is(err, webhook.ErrDuplicate) {
		t.Fatalf("Register(dup) err = %v, want ErrDuplicate", err)
	}

	opened, err := webhook.Open(name, webhook.Options{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if opened != probe {
		t.Error("Open() did not return the registered webhook")
	}

	if _, err := webhook.Open("kit-no-such-adapter", webhook.Options{}); !errors.Is(err, webhook.ErrUnknownAdapter) {
		t.Errorf("Open(unknown) err = %v, want ErrUnknownAdapter", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	w := factory(t)

	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := w.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
