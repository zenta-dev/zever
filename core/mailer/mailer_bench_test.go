package mailer

import "testing"

func benchOptions() Options {
	return Options{Host: "smtp.example.com", Port: 587}
}

func BenchmarkAddressValidate(b *testing.B) {
	a := Address{Name: "Alice", Address: "alice@example.com"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := a.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

func BenchmarkAddressString(b *testing.B) {
	a := Address{Name: "Alice", Address: "alice@example.com"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = a.String()
	}
}

func BenchmarkNewMail(b *testing.B) {
	from := Address{Address: "a@example.com"}
	to := []Address{{Address: "b@example.com"}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = NewMail(from, to, "subject", "body")
	}
}

func BenchmarkMailClone(b *testing.B) {
	m := Mail{
		From: Address{Address: "a@example.com"},
		To:   []Address{{Address: "b@example.com"}},
		Attachments: []Attachment{
			{Name: "a.txt", Content: []byte("hello")},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = m.Clone()
	}
}

func BenchmarkSenderSend(b *testing.B) {
	s := Sender{From: Address{Address: "a@example.com"}}
	m := &stubMailer{}
	msg := &Mail{To: []Address{{Address: "b@example.com"}}}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := s.Send(ctx, m, msg); err != nil {
			b.Fatalf("Send err = %v", err)
		}
	}
}
