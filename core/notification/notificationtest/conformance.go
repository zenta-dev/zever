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

	pushErr := n.Notify(ctx, &push)
	if pushErr == nil {
		*working = push

		return
	}
	mustMatch(t, errors.Is(pushErr, notification.ErrChannelNotSupported), "Notify(push) error = %v", pushErr)

	sms := notification.Notification{
		Target:  "+14155552671",
		Channel: notification.ChannelSMS,
		Body:    "hello",
	}

	smsErr := n.Notify(ctx, &sms)
	mustMatch(t, smsErr == nil, "Notify(push) unsupported, Notify(sms) error = %v", smsErr)

	*working = sms
}

func conformanceValidation(t *testing.T, factory func(t *testing.T) notification.Notifier) {
	t.Helper()

	ctx := t.Context()
	n := factory(t)

	nilErr := n.Notify(ctx, nil)
	expectMatch(t, errors.Is(nilErr, notification.ErrNilNotification), "Notify(nil) err = %v, want ErrNilNotification", nilErr)

	empty := notification.Notification{Channel: notification.ChannelPush, Body: "hi"}
	emptyErr := n.Notify(ctx, &empty)
	expectMatch(t, errors.Is(emptyErr, notification.ErrInvalidTarget), "Notify(empty target) err = %v, want ErrInvalidTarget", emptyErr)

	asErr := n.Notify(ctx, &empty)
	var targetErr notification.InvalidTargetError
	expectMatch(t, errors.As(asErr, &targetErr), "errors.As(err, InvalidTargetError) = false (err = %T %v)", asErr, asErr)

	badChannel := notification.Notification{Target: "t", Channel: "pager", Body: "hi"}
	channelErr := n.Notify(ctx, &badChannel)
	expectMatch(t, errors.Is(channelErr, notification.ErrInvalidChannel), "Notify(bad channel) err = %v, want ErrInvalidChannel", channelErr)

	badSMS := notification.Notification{Target: "not-a-number", Channel: notification.ChannelSMS, Body: "hi"}
	smsTargetErr := n.Notify(ctx, &badSMS)
	expectMatch(t, errors.Is(smsTargetErr, notification.ErrInvalidTarget), "Notify(bad sms target) err = %v, want ErrInvalidTarget", smsTargetErr)

	smsTitle := notification.Notification{Target: "+14155552671", Channel: notification.ChannelSMS, Title: "nope", Body: "hi"}
	titleErr := n.Notify(ctx, &smsTitle)
	expectMatch(t, errors.Is(titleErr, notification.ErrInvalidNotification), "Notify(sms title) err = %v, want ErrInvalidNotification", titleErr)
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T) notification.Notifier) {
	t.Helper()

	name := notification.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))
	probe := factory(t)

	regErr := notification.Register(name, func(notification.Options) (notification.Notifier, error) { return probe, nil })
	mustMatch(t, regErr == nil, "Register() error = %v", regErr)

	dupErr := notification.Register(name, func(notification.Options) (notification.Notifier, error) { return probe, nil })
	mustMatch(t, errors.Is(dupErr, notification.ErrDuplicate), "Register(dup) err = %v, want ErrDuplicate", dupErr)

	opened, openErr := notification.Open(name, notification.Options{})
	mustMatch(t, openErr == nil, "Open() error = %v", openErr)

	expectMatch(t, opened == probe, "Open() did not return the registered notifier")

	_, unknownErr := notification.Open("kit-no-such-adapter", notification.Options{})
	expectMatch(t, errors.Is(unknownErr, notification.ErrUnknownAdapter), "Open(unknown) err = %v, want ErrUnknownAdapter", unknownErr)
}

func conformanceClose(t *testing.T, factory func(t *testing.T) notification.Notifier, _ *notification.Notification) {
	t.Helper()

	n := factory(t)

	// Close is idempotent. Post-close Notify gating is adapter-scoped
	// (the log adapter rejects with ErrClosed; network adapters are
	// stateless and stay usable), so the kit asserts Close only.
	closeErr := n.Close()
	mustMatch(t, closeErr == nil, "Close() error = %v", closeErr)

	closeAgainErr := n.Close()
	expectMatch(t, closeAgainErr == nil, "Close() second error = %v, want nil", closeAgainErr)
}
