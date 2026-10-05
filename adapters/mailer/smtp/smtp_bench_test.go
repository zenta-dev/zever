package smtp

import (
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
)

func benchOptions() mailer.Options {
	return mailer.Options{
		Host:           "smtp.example.com",
		Port:           587,
		Encryption:     mailer.EncryptionNone,
		MaxMessageSize: 1 << 20,
	}
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
		},
	}
}

func BenchmarkNew(b *testing.B) {
	opts := benchOptions()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m, err := New(opts)
		if err != nil {
			b.Fatalf("New err = %v", err)
		}
		if err := m.Close(); err != nil {
			b.Fatalf("Close err = %v", err)
		}
	}
}

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		Register()
	}
}

func BenchmarkBuildMIME(b *testing.B) {
	msg := benchMail()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		body, err := buildMIME(msg)
		if err != nil {
			b.Fatalf("buildMIME err = %v", err)
		}
		if len(body) == 0 {
			b.Fatal("buildMIME returned empty body")
		}
	}
}

func BenchmarkBuildMIMEAttachments(b *testing.B) {
	msg := benchMail()
	msg.Attachments = []mailer.Attachment{
		{Name: "a.txt", Content: []byte("first")},
		{Name: "b.bin", Content: []byte("second")},
		{Name: "c.dat", Content: []byte("third")},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := buildMIME(msg); err != nil {
			b.Fatalf("buildMIME err = %v", err)
		}
	}
}

func BenchmarkEstimateSize(b *testing.B) {
	msg := benchMail()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if estimateSize(msg, 3) <= 0 {
			b.Fatal("estimateSize <= 0")
		}
	}
}
