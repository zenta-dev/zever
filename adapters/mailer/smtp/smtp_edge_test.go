package smtp_test

import (
	"context"
	"strings"
	"testing"

	mailsmtp "github.com/zenta-dev/zever/adapters/mailer/smtp"
	"github.com/zenta-dev/zever/core/mailer"
)

func TestSend_emptySubjectBody(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc, msg.Attachments = nil, nil, nil
	msg.Subject = ""
	msg.Body = ""
	msg.HTML = ""
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	raw := s.lastDATA(t)
	if !strings.Contains(raw, "Subject: \r\n") {
		t.Errorf("empty Subject header missing in:\n%s", raw)
	}
	if strings.Contains(raw, "text/plain") {
		t.Errorf("text/plain part emitted for empty body:\n%s", raw)
	}
}

func TestSend_bccOnly(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.To, msg.Cc, msg.Attachments = nil, nil, nil
	msg.Bcc = []mailer.Address{{Address: "hidden@example.com"}}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if joined := strings.Join(s.rcpt, "\n"); !strings.Contains(joined, "hidden@example.com") {
		t.Errorf("Bcc missing from envelope: %q", s.rcpt)
	}
	raw := s.data[len(s.data)-1]
	for _, line := range strings.Split(raw, "\r\n") {
		l := strings.ToLower(line)
		if strings.HasPrefix(l, "to:") || strings.HasPrefix(l, "cc:") || strings.HasPrefix(l, "bcc:") {
			t.Errorf("recipient header leaked for bcc-only mail: %q", line)
		}
	}
}

func TestSend_attachmentEmptyContent(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc = nil, nil
	msg.Attachments = []mailer.Attachment{{Name: "empty.txt", Content: nil}}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	raw := s.lastDATA(t)
	if !strings.Contains(raw, `filename="empty.txt"`) {
		t.Errorf("attachment part missing in:\n%s", raw)
	}
}

func TestSend_attachmentEmptyName(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc = nil, nil
	msg.Attachments = []mailer.Attachment{{Name: "", Content: []byte("x")}}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	raw := s.lastDATA(t)
	if !strings.Contains(raw, "application/octet-stream") {
		t.Errorf("empty attachment name should fall back to octet-stream:\n%s", raw)
	}
}

func TestSend_contextCanceled(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := m.Send(ctx, basicMail()); err == nil {
		t.Fatal("Send with canceled ctx = nil, want error")
	}
}

func TestSend_fromDisplayName(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc, msg.Attachments = nil, nil, nil
	msg.From = mailer.Address{Name: "Alice", Address: "alice@example.com"}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	raw := s.lastDATA(t)
	if !strings.Contains(raw, "From: Alice <alice@example.com>") {
		t.Errorf("display-name From header missing in:\n%s", raw)
	}
}

func TestSend_unicodeSubject(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc, msg.Attachments = nil, nil, nil
	msg.Subject = "héllo wörld"
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	raw := s.lastDATA(t)
	if !strings.Contains(raw, "=?utf-8?q?h=C3=A9llo_w=C3=B6rld?=") {
		t.Errorf("Q-encoded subject missing in:\n%s", raw)
	}
}

func TestSend_newlineInAttachmentName(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc = nil, nil
	msg.Attachments = []mailer.Attachment{{Name: "evil\r\nBcc: injected@example.com.txt", Content: []byte("x")}}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	raw := s.lastDATA(t)
	if !strings.Contains(raw, `filename="evilBcc: injected@example.com.txt"`) {
		t.Errorf("sanitized attachment filename missing in:\n%s", raw)
	}
	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "bcc:") {
			t.Errorf("header injection via attachment name leaked: %q", line)
		}
	}
}

func TestSend_htmlOnly(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc, msg.Attachments = nil, nil, nil
	msg.Body = ""
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	raw := s.lastDATA(t)
	if !strings.Contains(raw, "text/html") {
		t.Errorf("text/html part missing in:\n%s", raw)
	}
	if strings.Contains(raw, "text/plain") {
		t.Errorf("text/plain part emitted for empty body:\n%s", raw)
	}
}

func TestSend_multipleToRecipients(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc, msg.Attachments = nil, nil, nil
	msg.To = []mailer.Address{{Address: "a@example.com"}, {Address: "b@example.com"}}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	raw := s.lastDATA(t)
	if !strings.Contains(raw, "To: a@example.com, b@example.com") {
		t.Errorf("joined To header missing in:\n%s", raw)
	}
}

func TestSend_defaultMaxMessageSize(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	opts := plainOpts(t, s)
	opts.MaxMessageSize = 0
	m, err := mailsmtp.New(opts)
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	if err := m.Send(t.Context(), basicMail()); err != nil {
		t.Fatalf("Send with default size limit err = %v", err)
	}
}
