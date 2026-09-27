package mailer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

var freshSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", 1000+int(freshSeq.Add(1))))
}

type stubMailer struct {
	sent *Mail
	err  error
}

func (s *stubMailer) Send(_ context.Context, msg *Mail) error {
	s.sent = msg
	return s.err
}

func (s *stubMailer) Close() error { return nil }

func TestRegister_nilFactory_returnsErrNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Mailer, error) { return &stubMailer{}, nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	if err := Register(a, ok); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Register err = %v, want ErrDuplicate", err)
	}
	var de *DuplicateError
	if err := Register(a, ok); !errors.As(err, &de) {
		t.Fatalf("dup err %T is not *DuplicateError", err)
	} else if de.Adapter != a {
		t.Fatalf("dup carried adapter = %v want %v", de.Adapter, a)
	}
}

func TestOpen_unknownAdapter_returnsUnknownError(t *testing.T) {
	a := Adapter("test-9999")
	_, err := Open(a, Options{Host: "h.example.com", Port: 587})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("carried adapter = %v want %v", ue.Adapter, a)
	}
}

func TestOpen_factoryError_wrappedWithAdapter(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (Mailer, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{Host: "h.example.com", Port: 587})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "mailer: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "mailer: open")
	}
}

func TestOpen_success_returnsMailer(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Mailer, error) { return &stubMailer{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	m, err := Open(a, Options{Host: "h.example.com", Port: 587})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

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
