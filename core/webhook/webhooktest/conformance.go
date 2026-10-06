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

// mustMatch fails the test when ok is false.
func mustMatch(t *testing.T, ok bool, msg string, args ...any) {
	t.Helper()
	if !ok {
		t.Fatalf(msg, args...)
	}
}

// expectMatch records a test error when ok is false.
func expectMatch(t *testing.T, ok bool, msg string, args ...any) {
	t.Helper()
	if !ok {
		t.Errorf(msg, args...)
	}
}

// ConformanceDelivery verifies live fan-out against target: Register,
// Deliver returning nil, Unregister, then Deliver failing with
// ErrNotFound. The caller owns target (an httptest server URL in adapter
// tests) and asserts receipt itself; the kit asserts the delivery
// contract only, so queue-riding adapters pass without a consumer.
func ConformanceDelivery(t *testing.T, w webhook.Webhook, event, target string) {
	t.Helper()

	ctx := t.Context()

	regErr := w.Register(ctx, event, target, "kit-secret")
	mustMatch(t, regErr == nil, "Register() error = %v", regErr)

	deliverErr := w.Deliver(ctx, event, []byte(`{"kit":true}`))
	mustMatch(t, deliverErr == nil, "Deliver() error = %v", deliverErr)

	unregErr := w.Unregister(ctx, event, target)
	mustMatch(t, unregErr == nil, "Unregister() error = %v", unregErr)

	afterErr := w.Deliver(ctx, event, []byte(`{}`))
	mustMatch(t, errors.Is(afterErr, webhook.ErrNotFound), "Deliver() after Unregister err = %v, want ErrNotFound", afterErr)
}

func conformanceLifecycle(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)

	const event = "kit.lifecycle"
	const target = "http://127.0.0.1/kit-hook"

	regErr := w.Register(ctx, event, target, "s3cret")
	mustMatch(t, regErr == nil, "Register() error = %v", regErr)

	// Re-registering the same target replaces its secret, never duplicates.
	rotErr := w.Register(ctx, event, target, "s3cret-rotated")
	mustMatch(t, rotErr == nil, "Register(same) error = %v", rotErr)

	unregErr := w.Unregister(ctx, event, target)
	mustMatch(t, unregErr == nil, "Unregister() error = %v", unregErr)

	againErr := w.Unregister(ctx, event, target)
	expectMatch(t, errors.Is(againErr, webhook.ErrNotFound), "Unregister(again) err = %v, want ErrNotFound", againErr)
}

func conformanceNotFound(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)

	missingUnregErr := w.Unregister(ctx, "kit.no-event", "http://127.0.0.1/none")
	expectMatch(t, errors.Is(missingUnregErr, webhook.ErrNotFound), "Unregister(missing) err = %v, want ErrNotFound", missingUnregErr)

	missingDeliverErr := w.Deliver(ctx, "kit.no-event", []byte(`{}`))
	expectMatch(t, errors.Is(missingDeliverErr, webhook.ErrNotFound), "Deliver(missing) err = %v, want ErrNotFound", missingDeliverErr)

	emptyEventErr := w.Register(ctx, "", "http://127.0.0.1/hook", "s")
	expectMatch(t, emptyEventErr != nil, "Register(empty event) = nil, want error")

	emptyTargetErr := w.Register(ctx, "kit.e", "", "s")
	expectMatch(t, emptyTargetErr != nil, "Register(empty target) = nil, want error")
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	name := webhook.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))
	probe := factory(t)

	regErr := webhook.Register(name, func(webhook.Options) (webhook.Webhook, error) { return probe, nil })
	mustMatch(t, regErr == nil, "Register() error = %v", regErr)

	dupErr := webhook.Register(name, func(webhook.Options) (webhook.Webhook, error) { return probe, nil })
	mustMatch(t, errors.Is(dupErr, webhook.ErrDuplicate), "Register(dup) err = %v, want ErrDuplicate", dupErr)

	opened, openErr := webhook.Open(name, webhook.Options{})
	mustMatch(t, openErr == nil, "Open() error = %v", openErr)

	expectMatch(t, opened == probe, "Open() did not return the registered webhook")

	_, unknownErr := webhook.Open("kit-no-such-adapter", webhook.Options{})
	expectMatch(t, errors.Is(unknownErr, webhook.ErrUnknownAdapter), "Open(unknown) err = %v, want ErrUnknownAdapter", unknownErr)
}

func conformanceClose(t *testing.T, factory func(t *testing.T) webhook.Webhook) {
	t.Helper()

	w := factory(t)

	closeErr := w.Close()
	mustMatch(t, closeErr == nil, "Close() error = %v", closeErr)

	closeAgainErr := w.Close()
	expectMatch(t, closeAgainErr == nil, "Close() second error = %v, want nil", closeAgainErr)
}
