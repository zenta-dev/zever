package log_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	"github.com/zenta-dev/zever/core/mailer"
)

func sendAndDecode(t *testing.T, msg *mailer.Mail) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer func() { _ = m.Close() }()
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	return decoded
}

func TestSend_emptySubjectBody(t *testing.T) {
	t.Parallel()

	msg := validMail()
	msg.Subject = ""
	msg.Body = ""
	decoded := sendAndDecode(t, msg)

	if decoded["subject"] != "" || decoded["body"] != "" {
		t.Errorf("fields = %v, want empty subject/body", decoded)
	}
}

func TestSend_htmlOnly(t *testing.T) {
	t.Parallel()

	msg := validMail()
	msg.Body = ""
	msg.HTML = "<p>hi</p>"
	decoded := sendAndDecode(t, msg)

	if decoded["body"] != "" || decoded["html"] != "<p>hi</p>" {
		t.Errorf("fields = %v, want empty body and html", decoded)
	}
}

func TestSend_bccOnlyRecipients(t *testing.T) {
	t.Parallel()

	msg := validMail()
	msg.To = nil
	msg.Cc = nil
	msg.Bcc = []mailer.Address{{Address: "hidden@example.com"}}
	decoded := sendAndDecode(t, msg)

	to, ok := decoded["to"].([]any)
	if !ok || len(to) != 0 {
		t.Errorf("to = %v, want empty", decoded["to"])
	}
	bcc, ok := decoded["bcc"].([]any)
	if !ok || len(bcc) != 1 {
		t.Errorf("bcc = %v, want one entry", decoded["bcc"])
	}
}

func TestSend_invalidCcAddress(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer func() { _ = m.Close() }()

	msg := validMail()
	msg.Cc = []mailer.Address{{Address: "bad"}}
	if err := m.Send(t.Context(), msg); !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Fatalf("Send err = %v, want ErrInvalidAddress", err)
	}
}

func TestSend_invalidBccAddress(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer func() { _ = m.Close() }()

	msg := validMail()
	msg.Bcc = []mailer.Address{{Address: "bad"}}
	if err := m.Send(t.Context(), msg); !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Fatalf("Send err = %v, want ErrInvalidAddress", err)
	}
}

func TestSend_attachmentEmptyContent(t *testing.T) {
	t.Parallel()

	msg := validMail()
	msg.Attachments = []mailer.Attachment{{Name: "empty.txt", Content: nil}}
	decoded := sendAndDecode(t, msg)

	atts, ok := decoded["attachments"].([]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("attachments = %v, want one entry", decoded["attachments"])
	}
	att, ok := atts[0].(map[string]any)
	if !ok {
		t.Fatalf("attachment = %T, want map", atts[0])
	}
	if att["size"] != float64(0) {
		t.Errorf("attachment size = %v, want 0", att["size"])
	}
}

func TestSend_attachmentEmptyName(t *testing.T) {
	t.Parallel()

	msg := validMail()
	msg.Attachments = []mailer.Attachment{{Name: "", Content: []byte("x")}}
	decoded := sendAndDecode(t, msg)

	atts, ok := decoded["attachments"].([]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("attachments = %v, want one entry", decoded["attachments"])
	}
	att, ok := atts[0].(map[string]any)
	if !ok {
		t.Fatalf("attachment = %T, want map", atts[0])
	}
	if att["name"] != "" {
		t.Errorf("attachment name = %v, want empty", att["name"])
	}
}

func TestSend_unicodeSubject(t *testing.T) {
	t.Parallel()

	msg := validMail()
	msg.Subject = "héllo wörld"
	decoded := sendAndDecode(t, msg)

	if decoded["subject"] != "héllo wörld" {
		t.Errorf("subject = %v, want unicode preserved", decoded["subject"])
	}
}

func TestSend_allRecipientKinds(t *testing.T) {
	t.Parallel()

	msg := validMail()
	msg.Cc = []mailer.Address{{Address: "cc@example.com"}}
	msg.Bcc = []mailer.Address{{Address: "bcc@example.com"}}
	decoded := sendAndDecode(t, msg)

	for _, kind := range []string{"to", "cc", "bcc"} {
		list, ok := decoded[kind].([]any)
		if !ok || len(list) != 1 {
			t.Errorf("%s = %v, want one entry", kind, decoded[kind])
		}
	}
}
