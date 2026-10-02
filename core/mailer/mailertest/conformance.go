// Package mailertest provides the conformance kit third-party mailer adapters run to prove backend parity.
package mailertest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
)

// Conformance verifies factory-built backends implement the
// mailer.Mailer contract: open/register round-trip, valid delivery,
// nil-message / no-recipient / bad-address sentinels, and Close.
// Each subtest takes a fresh instance from factory so cases stay
// isolated. Tests never call time.Sleep and never touch the network.
//
// Documented smtp exemption: the smtp adapter returns its local
// ErrNilMessage instead of an error errors.Is-compatible with
// mailer.ErrNilMessage, so its conformance test skips with a reason
// until the adapter wraps the core sentinel. All other subtests pass
// against the adapter's loopback fake relay; the kit runs against
// the log adapter in its own package.
func Conformance(t *testing.T, factory func(t *testing.T) mailer.Mailer) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("Send", func(t *testing.T) { conformanceSend(t, factory) })
	t.Run("NilMessage", func(t *testing.T) { conformanceNilMessage(t, factory) })
	t.Run("NoRecipients", func(t *testing.T) { conformanceNoRecipients(t, factory) })
	t.Run("InvalidAddress", func(t *testing.T) { conformanceInvalidAddress(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := mailer.Open(mailer.Adapter("conformance-missing-adapter"), mailer.Options{Host: "kit.example", Port: 587}); !errors.Is(err, mailer.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := mailer.Adapter("conformance-probe-mailer")

	if err := mailer.Register(probe, nil); !errors.Is(err, mailer.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(mailer.Options) (mailer.Mailer, error) {
		return nil, errors.New("mailertest: probe factory must not run")
	}

	_ = mailer.Register(probe, stub)

	if err := mailer.Register(probe, stub); !errors.Is(err, mailer.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

// kitMail returns a minimal valid message for delivery probes.
func kitMail() *mailer.Mail {
	return &mailer.Mail{
		From:    mailer.Address{Address: "kit-from@example.com"},
		To:      []mailer.Address{{Address: "kit-to@example.com"}},
		Subject: "kit subject",
		Body:    "kit body",
	}
}

func conformanceSend(t *testing.T, factory func(t *testing.T) mailer.Mailer) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	if err := m.Send(ctx, kitMail()); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func conformanceNilMessage(t *testing.T, factory func(t *testing.T) mailer.Mailer) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	if err := m.Send(ctx, nil); !errors.Is(err, mailer.ErrNilMessage) {
		t.Errorf("Send(nil) err = %v, want ErrNilMessage", err)
	}
}

func conformanceNoRecipients(t *testing.T, factory func(t *testing.T) mailer.Mailer) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	msg := kitMail()
	msg.To = nil

	if err := m.Send(ctx, msg); !errors.Is(err, mailer.ErrNoRecipients) {
		t.Errorf("Send(no recipients) err = %v, want ErrNoRecipients", err)
	}
}

func conformanceInvalidAddress(t *testing.T, factory func(t *testing.T) mailer.Mailer) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	msg := kitMail()
	msg.From = mailer.Address{Address: "not-an-address"}

	if err := m.Send(ctx, msg); !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("Send(bad from) err = %v, want ErrInvalidAddress", err)
	}

	msg = kitMail()
	msg.To = []mailer.Address{{Address: "bad"}}

	if err := m.Send(ctx, msg); !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("Send(bad to) err = %v, want ErrInvalidAddress", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) mailer.Mailer) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := m.Send(ctx, kitMail()); !errors.Is(err, mailer.ErrClosed) {
		t.Errorf("Send(after Close) err = %v, want ErrClosed", err)
	}

	if err := m.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
