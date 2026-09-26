package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/mailer"
	mailerlog "github.com/zenta-dev/zever/mailer/log"
)

func validOptions() mailer.Options {
	return mailer.Options{Host: "smtp.example.com", Port: 587}
}

func validMail() *mailer.Mail {
	return &mailer.Mail{
		From:    mailer.Address{Address: "from@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Subject: "hello",
		Body:    "world",
	}
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()
	if _, err := mailerlog.New(mailer.Options{}); !errors.Is(err, mailer.ErrInvalidOptions) {
		t.Fatalf("New err = %v, want ErrInvalidOptions", err)
	}
	var buf bytes.Buffer
	if _, err := mailerlog.NewWithWriter(mailer.Options{}, &buf); !errors.Is(err, mailer.ErrInvalidOptions) {
		t.Fatalf("NewWithWriter err = %v, want ErrInvalidOptions", err)
	}
}

func TestSend_nilMessage(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer m.Close()
	if err := m.Send(t.Context(), nil); err == nil {
		t.Fatal("Send(nil) = nil, want error")
	}
}

func TestSend_noRecipients(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer m.Close()
	msg := validMail()
	msg.To = nil
	msg.Cc = nil
	msg.Bcc = nil
	if err := m.Send(t.Context(), msg); !errors.Is(err, mailer.ErrNoRecipients) {
		t.Fatalf("Send err = %v, want ErrNoRecipients", err)
	}
}

func TestSend_invalidAddress(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer m.Close()

	badFrom := validMail()
	badFrom.From = mailer.Address{Address: "not-an-address"}
	if err := m.Send(t.Context(), badFrom); !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("bad From err = %v, want ErrInvalidAddress", err)
	}

	badTo := validMail()
	badTo.To = []mailer.Address{{Address: "bad"}}
	if err := m.Send(t.Context(), badTo); !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("bad To err = %v, want ErrInvalidAddress", err)
	}
}

func TestSend_contextCanceled(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer m.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := m.Send(ctx, validMail()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send err = %v, want context.Canceled", err)
	}
}

func TestSend_emitsJSONRedacted(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer m.Close()

	secret := "super-secret-bytes"
	msg := &mailer.Mail{
		From:    mailer.Address{Name: "From", Address: "from@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Cc:      []mailer.Address{{Address: "cc@example.com"}},
		Bcc:     []mailer.Address{{Address: "bcc@example.com"}},
		Subject: "subj",
		Body:    "body",
		HTML:    "<p>body</p>",
		Attachments: []mailer.Attachment{
			{Name: "a.txt", Content: []byte(secret), Inline: false, ContentID: ""},
			{Name: "b.png", Content: []byte("12345"), Inline: true, ContentID: "cid-b"},
		},
	}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}

	line := strings.TrimSpace(buf.String())
	var decoded struct {
		From        map[string]any   `json:"from"`
		To          []map[string]any `json:"to"`
		Cc          []map[string]any `json:"cc"`
		Bcc         []map[string]any `json:"bcc"`
		Subject     string           `json:"subject"`
		Body        string           `json:"body"`
		HTML        string           `json:"html"`
		Attachments []struct {
			Name      string `json:"name"`
			Size      int    `json:"size"`
			Inline    bool   `json:"inline"`
			ContentID string `json:"content_id"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, line)
	}
	if decoded.Subject != "subj" || decoded.Body != "body" || decoded.HTML != "<p>body</p>" {
		t.Errorf("fields = %+v, want subj/body/html", decoded)
	}
	if len(decoded.Bcc) != 1 {
		t.Fatalf("bcc missing in JSON (dev log must include bcc): %s", line)
	}
	if len(decoded.Attachments) != 2 {
		t.Fatalf("attachments = %d, want 2", len(decoded.Attachments))
	}
	if decoded.Attachments[0].Size != len(secret) {
		t.Errorf("attachment size = %d, want %d", decoded.Attachments[0].Size, len(secret))
	}
	if decoded.Attachments[1].ContentID != "cid-b" || !decoded.Attachments[1].Inline {
		t.Errorf("attachment meta = %+v", decoded.Attachments[1])
	}
	if strings.Contains(line, secret) {
		t.Errorf("JSON contains raw attachment bytes (secret leaked)")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		t.Fatalf("re-decode err = %v", err)
	}
	atts, ok := raw["attachments"].([]any)
	if !ok {
		t.Fatalf("attachments = %T, want []any", raw["attachments"])
	}
	for _, att := range atts {
		am, ok := att.(map[string]any)
		if !ok {
			t.Fatalf("attachment = %T, want map", att)
		}
		if _, ok := am["content"]; ok {
			t.Errorf("attachment entry has raw content key: %v", am)
		}
		for _, v := range am {
			if s, ok := v.(string); ok && s == secret {
				t.Errorf("raw bytes leaked in attachment field")
			}
		}
	}
}

func TestClose_idempotentAndAfterClose(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close err = %v, want nil", err)
	}
	if err := m.Send(t.Context(), validMail()); !errors.Is(err, mailer.ErrClosed) {
		t.Fatalf("Send after close err = %v, want ErrClosed", err)
	}
}

func TestConcurrentSends(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer m.Close()

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = m.Send(t.Context(), validMail())
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("send %d err = %v", i, err)
		}
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != n {
		t.Fatalf("lines = %d, want %d", len(lines), n)
	}
	for i, line := range lines {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
	}
}
