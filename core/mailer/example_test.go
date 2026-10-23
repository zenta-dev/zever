package mailer_test

import (
	"context"
	"io"

	maillog "github.com/zenta-dev/zever/adapters/mailer/log"
	"github.com/zenta-dev/zever/core/mailer"
)

// Example sends one message through the log adapter.
func Example() {
	m, err := maillog.NewWithWriter(mailer.Options{Host: "localhost", Port: 25}, io.Discard)
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
