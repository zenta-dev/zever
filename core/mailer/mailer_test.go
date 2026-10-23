package mailer

import (
	"context"
	"sync/atomic"
	"testing"
)

var freshSeq atomic.Int64

type stubMailer struct {
	sent *Mail
	err  error
}

func (s *stubMailer) Send(_ context.Context, msg *Mail) error {
	s.sent = msg
	return s.err
}

func (s *stubMailer) Close() error { return nil }

func TestSender_Send_setsFrom(t *testing.T) {
	t.Parallel()
	from := Address{Name: "Svc", Address: "svc@example.com"}
	s := Sender{From: from}
	stub := &stubMailer{}
	mail := &Mail{
		To:      []Address{{Address: "to@example.com"}},
		Subject: "hi",
		Body:    "body",
	}
	if err := s.Send(t.Context(), stub, mail); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	if mail.From != from {
		t.Fatalf("mail.From = %+v want %+v", mail.From, from)
	}
	if stub.sent != mail {
		t.Fatalf("stub did not capture sent mail")
	}
	if stub.sent.From != from {
		t.Fatalf("captured From = %+v want %+v", stub.sent.From, from)
	}
}

func TestNewMail_setsFields(t *testing.T) {
	t.Parallel()
	from := Address{Address: "from@example.com"}
	to := []Address{{Address: "to@example.com"}}
	m := NewMail(from, to, "subj", "body")
	if m.From != from {
		t.Errorf("From = %+v want %+v", m.From, from)
	}
	if len(m.To) != 1 || m.To[0] != to[0] {
		t.Errorf("To = %+v want %+v", m.To, to)
	}
	if m.Subject != "subj" || m.Body != "body" {
		t.Errorf("Subject/Body = %q/%q", m.Subject, m.Body)
	}
}

func TestMail_Clone_deepCopy(t *testing.T) {
	t.Parallel()
	orig := Mail{
		From:    Address{Address: "from@example.com"},
		To:      []Address{{Address: "to@example.com"}},
		Cc:      []Address{{Address: "cc@example.com"}},
		Bcc:     []Address{{Address: "bcc@example.com"}},
		Subject: "s",
		Body:    "b",
		HTML:    "<b>b</b>",
		Attachments: []Attachment{
			{Name: "a.txt", Content: []byte("hello")},
		},
	}
	cl := orig.Clone()
	if cl.Subject != orig.Subject || cl.Body != orig.Body || cl.HTML != orig.HTML {
		t.Fatalf("clone scalars differ: %+v vs %+v", cl, orig)
	}
	// Mutate clone slices; orig must not change.
	cl.To[0].Address = "changed@example.com"
	cl.Cc[0].Address = "changed@example.com"
	cl.Bcc[0].Address = "changed@example.com"
	cl.Attachments[0].Content[0] = 'X'
	if orig.To[0].Address == "changed@example.com" {
		t.Error("Clone To shares backing array")
	}
	if orig.Cc[0].Address == "changed@example.com" {
		t.Error("Clone Cc shares backing array")
	}
	if orig.Bcc[0].Address == "changed@example.com" {
		t.Error("Clone Bcc shares backing array")
	}
	if orig.Attachments[0].Content[0] == 'X' {
		t.Error("Clone Attachment Content shares backing array")
	}
}
