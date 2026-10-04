package log

import (
	"io"
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
)

// benchMailer builds a JSON-line mailer writing to io.Discard.
func benchMailer(b *testing.B) mailer.Mailer {
	b.Helper()

	m, err := NewWithWriter(mailer.Options{Host: "smtp.example.com", Port: 587}, io.Discard)
	if err != nil {
		b.Fatalf("NewWithWriter: %v", err)
	}

	b.Cleanup(func() { _ = m.Close() })

	return m
}

// benchMail builds a minimal valid message.
func benchMail() *mailer.Mail {
	return &mailer.Mail{
		From:    mailer.Address{Address: "from@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Subject: "subject",
		Body:    "body",
	}
}

// BenchmarkSend measures validation, encoding, and the redacted JSON write.
func BenchmarkSend(b *testing.B) {
	m := benchMailer(b)
	ctx := b.Context()
	msg := benchMail()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := m.Send(ctx, msg); err != nil {
			b.Fatalf("Send: %v", err)
		}
	}
}

// BenchmarkSendParallel measures concurrent sends serialized on the writer mutex.
func BenchmarkSendParallel(b *testing.B) {
	m := benchMailer(b)
	ctx := b.Context()
	msg := benchMail()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := m.Send(ctx, msg); err != nil {
				b.Fatalf("Send: %v", err)
			}
		}
	})
}
