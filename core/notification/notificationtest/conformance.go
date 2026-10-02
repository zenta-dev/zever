// Package notificationtest provides the conformance kit third-party
// notification notifiers run to prove backend parity.
package notificationtest

import (
	"errors"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/notification"
)

// adapterSeq keeps throwaway registry names unique across Conformance runs
// in one process.
var adapterSeq atomic.Uint64

// Conformance verifies factory-built notifiers implement the
// notification.Notifier contract: validated delivery on at least one
// channel, validation errors, Open/Register registry wiring, and Close.
// Each subtest takes a fresh instance from factory so cases stay
// isolated. No test touches the network: push targets are opaque tokens
// and SMS targets are E.164 fixtures, and adapters under test must stub
// their transports.
//
// Adapters are channel-scoped (SMS-only, push-only): the happy path
// accepts ErrChannelNotSupported on one channel and requires delivery on
// the other, recording the working notification for the Close subtest so
// post-close assertions use a shape the adapter accepts.
func Conformance(t *testing.T, factory func(t *testing.T) notification.Notifier) {
	t.Helper()

	var working notification.Notification

	t.Run("HappyPath", func(t *testing.T) { conformanceHappyPath(t, factory, &working) })
	t.Run("Validation", func(t *testing.T) { conformanceValidation(t, factory) })
	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory, &working) })
}

func conformanceHappyPath(t *testing.T, factory func(t *testing.T) notification.Notifier, working *notification.Notification) {
	t.Helper()

	ctx := t.Context()
	n := factory(t)

	push := notification.Notification{
		Target:  "kit-device-token",
		Channel: notification.ChannelPush,
		Title:   "Kit",
		Body:    "hello",
		Data:    map[string]string{"k": "v"},
	}

	if err := n.Notify(ctx, &push); err == nil {
		*working = push

		return
	} else if !errors.Is(err, notification.ErrChannelNotSupported) {
		t.Fatalf("Notify(push) error = %v", err)
	}

	sms := notification.Notification{
		Target:  "+14155552671",
		Channel: notification.ChannelSMS,
		Body:    "hello",
	}

	if err := n.Notify(ctx, &sms); err != nil {
		t.Fatalf("Notify(push) unsupported, Notify(sms) error = %v", err)
	}

	*working = sms
}

func conformanceValidation(t *testing.T, factory func(t *testing.T) notification.Notifier) {
	t.Helper()

	ctx := t.Context()
	n := factory(t)

	if err := n.Notify(ctx, nil); !errors.Is(err, notification.ErrNilNotification) {
		t.Errorf("Notify(nil) err = %v, want ErrNilNotification", err)
	}

	empty := notification.Notification{Channel: notification.ChannelPush, Body: "hi"}
	if err := n.Notify(ctx, &empty); !errors.Is(err, notification.ErrInvalidTarget) {
		t.Errorf("Notify(empty target) err = %v, want ErrInvalidTarget", err)
	}

	var targetErr *notification.InvalidTargetError
	if err := n.Notify(ctx, &empty); !errors.As(err, &targetErr) {
		t.Errorf("errors.As(err, InvalidTargetError) = false (err = %T %v)", err, err)
	}

	badChannel := notification.Notification{Target: "t", Channel: "pager", Body: "hi"}
	if err := n.Notify(ctx, &badChannel); !errors.Is(err, notification.ErrInvalidChannel) {
		t.Errorf("Notify(bad channel) err = %v, want ErrInvalidChannel", err)
	}

	badSMS := notification.Notification{Target: "not-a-number", Channel: notification.ChannelSMS, Body: "hi"}
	if err := n.Notify(ctx, &badSMS); !errors.Is(err, notification.ErrInvalidTarget) {
		t.Errorf("Notify(bad sms target) err = %v, want ErrInvalidTarget", err)
	}

	smsTitle := notification.Notification{Target: "+14155552671", Channel: notification.ChannelSMS, Title: "nope", Body: "hi"}
	if err := n.Notify(ctx, &smsTitle); !errors.Is(err, notification.ErrInvalidNotification) {
		t.Errorf("Notify(sms title) err = %v, want ErrInvalidNotification", err)
	}
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T) notification.Notifier) {
	t.Helper()

	name := notification.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))
	probe := factory(t)

	if err := notification.Register(name, func(notification.Options) (notification.Notifier, error) { return probe, nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := notification.Register(name, func(notification.Options) (notification.Notifier, error) { return probe, nil }); !errors.Is(err, notification.ErrDuplicate) {
		t.Fatalf("Register(dup) err = %v, want ErrDuplicate", err)
	}

	opened, err := notification.Open(name, notification.Options{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if opened != probe {
		t.Error("Open() did not return the registered notifier")
	}

	if _, err := notification.Open("kit-no-such-adapter", notification.Options{}); !errors.Is(err, notification.ErrUnknownAdapter) {
		t.Errorf("Open(unknown) err = %v, want ErrUnknownAdapter", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) notification.Notifier, _ *notification.Notification) {
	t.Helper()

	n := factory(t)

	// Close is idempotent. Post-close Notify gating is adapter-scoped
	// (the log adapter rejects with ErrClosed; network adapters are
	// stateless and stay usable), so the kit asserts Close only.
	if err := n.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := n.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
