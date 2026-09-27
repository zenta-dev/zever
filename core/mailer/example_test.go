package mailer_test

import (
	"context"
	"errors"
	"io"

	maillog "github.com/zenta-dev/zever/adapters/mailer/log"
	"github.com/zenta-dev/zever/core/mailer"
)

// ExampleOpen sends one message through the log adapter.
func ExampleOpen() {
	if err := mailer.Register(mailer.Log, func(o mailer.Options) (mailer.Mailer, error) {
		return maillog.NewWithWriter(o, io.Discard)
	}); err != nil && !errors.Is(err, mailer.ErrDuplicate) {
		return
	}

	m, err := mailer.Open(mailer.Log, mailer.Options{Host: "localhost", Port: 25})
	if err != nil {
		return
	}
	defer func() { _ = m.Close() }()

	msg := mailer.NewMail(
		mailer.Address{Address: "from@example.com"},
		[]mailer.Address{{Address: "to@example.com"}},
		"hello",
		"welcome",
	)

	_ = m.Send(context.Background(), &msg)
}
