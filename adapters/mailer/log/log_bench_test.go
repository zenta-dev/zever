package log_test

import (
	"io"
	"testing"

	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	"github.com/zenta-dev/zever/core/mailer"
)

func benchOptions() mailer.Options {
	return mailer.Options{Host: "smtp.example.com", Port: 587}
}

func benchMail() *mailer.Mail {
	return &mailer.Mail{
		From:    mailer.Address{Name: "From", Address: "from@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Cc:      []mailer.Address{{Address: "cc@example.com"}},
		Subject: "bench subject",
		Body:    "bench body",
		HTML:    "<p>bench body</p>",
		Attachments: []mailer.Attachment{
			{Name: "a.txt", Content: []byte("attachment-bytes")},
			{Name: "b.png", Content: []byte("12345"), Inline: true, ContentID: "cid-b"},
		},
	}
}

func BenchmarkNewWithWriter(b *testing.B) {
	opts := benchOptions()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m, err := mailerlog.NewWithWriter(opts, io.Discard)
		if err != nil {
			b.Fatalf("NewWithWriter err = %v", err)
		}
		if err := m.Close(); err != nil {
			b.Fatalf("Close err = %v", err)
		}
	}
}

func BenchmarkSend(b *testing.B) {
	m, err := mailerlog.NewWithWriter(benchOptions(), io.Discard)
	if err != nil {
		b.Fatalf("NewWithWriter err = %v", err)
	}
	defer func() { _ = m.Close() }()
	msg := benchMail()
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.Send(ctx, msg); err != nil {
			b.Fatalf("Send err = %v", err)
		}
	}
}

func BenchmarkSendParallel(b *testing.B) {
	m, err := mailerlog.NewWithWriter(benchOptions(), io.Discard)
	if err != nil {
		b.Fatalf("NewWithWriter err = %v", err)
	}
	defer func() { _ = m.Close() }()
	msg := benchMail()
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := m.Send(ctx, msg); err != nil {
				b.Fatalf("Send err = %v", err)
			}
		}
	})
}

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		mailerlog.Register()
	}
}
