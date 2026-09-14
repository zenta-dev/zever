// Hermetic tests for the SMTP adapter. Each test spins a fake SMTP
// server on 127.0.0.1:0; stdlib only.
package smtp_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/mailer"
	mailsmtp "github.com/zenta-dev/zever/mailer/smtp"
)

type serverConfig struct {
	dataEnd string // response after DATA dot, e.g. "250 OK\r\n"
	quit    string // response to QUIT, e.g. "221 Bye\r\n"
	hang    bool   // accept then never greet (timeout tests)
}

type fakeSMTP struct {
	t   *testing.T
	ln  net.Listener
	cfg serverConfig

	mu   sync.Mutex
	from []string
	rcpt []string
	data []string
}

func startFakeSMTP(t *testing.T, cfg serverConfig) *fakeSMTP {
	t.Helper()
	if cfg.dataEnd == "" {
		cfg.dataEnd = "250 OK\r\n"
	}
	if cfg.quit == "" {
		cfg.quit = "221 Bye\r\n"
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTP{t: t, ln: ln, cfg: cfg}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handle(c)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *fakeSMTP) port(t *testing.T) int {
	t.Helper()
	addr, ok := s.ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr = %T, want *net.TCPAddr", s.ln.Addr())
	}
	return addr.Port
}

func (s *fakeSMTP) handle(c net.Conn) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	if s.cfg.hang {
		time.Sleep(20 * time.Second)
		return
	}
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	fmt.Fprint(w, "220 fake ESMTP\r\n")
	_ = w.Flush()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			fmt.Fprint(w, "250 OK\r\n")
			_ = w.Flush()
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "EHLO":
			fmt.Fprint(w, "250-localhost\r\n250 AUTH PLAIN\r\n")
			_ = w.Flush()
		case "HELO":
			fmt.Fprint(w, "250 localhost\r\n")
			_ = w.Flush()
		case "MAIL":
			s.mu.Lock()
			s.from = append(s.from, line)
			s.mu.Unlock()
			fmt.Fprint(w, "250 OK\r\n")
			_ = w.Flush()
		case "RCPT":
			s.mu.Lock()
			s.rcpt = append(s.rcpt, line)
			s.mu.Unlock()
			fmt.Fprint(w, "250 OK\r\n")
			_ = w.Flush()
		case "DATA":
			fmt.Fprint(w, "354 End with .\r\n")
			_ = w.Flush()
			var b strings.Builder
			for {
				dl, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if dl == ".\r\n" || dl == ".\n" {
					break
				}
				b.WriteString(dl)
			}
			s.mu.Lock()
			s.data = append(s.data, b.String())
			s.mu.Unlock()
			fmt.Fprint(w, s.cfg.dataEnd)
			_ = w.Flush()
		case "QUIT":
			fmt.Fprint(w, s.cfg.quit)
			_ = w.Flush()
			return
		case "RSET", "NOOP":
			fmt.Fprint(w, "250 OK\r\n")
			_ = w.Flush()
		case "AUTH":
			fmt.Fprint(w, "235 OK\r\n")
			_ = w.Flush()
		case "STARTTLS":
			fmt.Fprint(w, "502 no TLS here\r\n")
			_ = w.Flush()
		default:
			fmt.Fprint(w, "250 OK\r\n")
			_ = w.Flush()
		}
	}
}

func (s *fakeSMTP) lastDATA(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data) == 0 {
		t.Fatal("server captured no DATA")
	}
	return s.data[len(s.data)-1]
}

func plainOpts(t *testing.T, s *fakeSMTP) mailer.Options {
	t.Helper()
	return mailer.Options{
		Host:           "127.0.0.1",
		Port:           s.port(t),
		Encryption:     mailer.EncryptionNone,
		Timeout:        5 * time.Second,
		MaxMessageSize: 1 << 20,
	}
}

func basicMail() *mailer.Mail {
	return &mailer.Mail{
		From:    mailer.Address{Address: "sender@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Cc:      []mailer.Address{{Address: "cc@example.com"}},
		Subject: "hello",
		Body:    "plain body",
		HTML:    "<p>hi</p>",
		Attachments: []mailer.Attachment{
			{Name: "a.txt", Content: []byte("abc")},
		},
	}
}

func openPlain(t *testing.T, s *fakeSMTP) mailer.Mailer {
	t.Helper()
	m, err := mailsmtp.New(plainOpts(t, s))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()
	cases := []mailer.Options{
		{Port: 25},                   // empty host
		{Host: "x.example", Port: 0}, // bad port
		{Host: "x.example", Port: 25, Encryption: "bogus"},                                             // bad encryption
		{Host: "x.example", Port: 25, Timeout: -time.Second},                                           // negative timeout
		{Host: "x.example", Port: 25, MaxMessageSize: -1},                                              // negative size
		{Host: "x.example", Port: 25, Encryption: mailer.EncryptionNone, Username: "u", Password: "p"}, // none+creds
		{Host: "x.example", Port: 25, Username: "u"},                                                   // unpaired creds
	}
	for i, opts := range cases {
		if _, err := mailsmtp.New(opts); err == nil {
			t.Errorf("case %d: expected error, got nil", i)
		}
	}
}

func TestSendPlaintext(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	if err := m.Send(context.Background(), basicMail()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.from) != 1 || !strings.Contains(s.from[0], "sender@example.com") {
		t.Errorf("MAIL FROM not seen: %q", s.from)
	}
	joined := strings.Join(s.rcpt, "\n")
	for _, want := range []string{"to@example.com", "cc@example.com"} {
		if !strings.Contains(joined, want) {
			t.Errorf("RCPT for %s not seen: %q", want, s.rcpt)
		}
	}
	if len(s.data) != 1 {
		t.Fatalf("expected 1 DATA, got %d", len(s.data))
	}
	raw := s.data[0]
	for _, want := range []string{
		"MIME-Version: 1.0",
		"From: sender@example.com",
		"To: to@example.com",
		"Cc: cc@example.com",
		"Subject: hello",
		"text/plain",
		"text/html",
		"a.txt",
		"YWJj", // base64("abc")
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("DATA missing %q", want)
		}
	}
}

func TestSendBccEnvelopeOnly(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc = nil
	msg.Bcc = []mailer.Address{{Address: "hidden@example.com"}}
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if joined := strings.Join(s.rcpt, "\n"); !strings.Contains(joined, "hidden@example.com") {
		t.Errorf("Bcc missing from envelope RCPT: %q", s.rcpt)
	}
	raw := s.data[len(s.data)-1]
	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "bcc:") {
			t.Errorf("Bcc leaked into headers: %q", line)
		}
	}
}

func TestSendSTARTTLSAbsentFails(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{}) // EHLO omits STARTTLS
	opts := plainOpts(t, s)
	opts.Encryption = mailer.EncryptionSTARTTLS
	m, err := mailsmtp.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = m.Close() }()

	err = m.Send(context.Background(), basicMail())
	if err == nil {
		t.Fatal("expected STARTTLS-mandatory failure, got nil")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "STARTTLS") {
		t.Errorf("error should mention STARTTLS: %v", err)
	}
}

func TestSendImplicitTLSHandshakeFails(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{}) // plaintext speaker
	opts := plainOpts(t, s)
	opts.Encryption = mailer.EncryptionImplicitTLS
	m, err := mailsmtp.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = m.Close() }()

	if err := m.Send(context.Background(), basicMail()); err == nil {
		t.Fatal("expected TLS handshake failure, got nil")
	}
}

func TestSendNilMessage(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)
	if err := m.Send(context.Background(), nil); err == nil {
		t.Fatal("expected nil-message error, got nil")
	}
}

func TestSendInvalidAddresses(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	badFrom := basicMail()
	badFrom.From = mailer.Address{Address: "not-an-address"}
	if err := m.Send(context.Background(), badFrom); err == nil {
		t.Error("expected invalid-from error, got nil")
	} else if !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("want ErrInvalidAddress, got %v", err)
	}

	badTo := basicMail()
	badTo.To = []mailer.Address{{Address: "bad\r\n@example.com"}}
	if err := m.Send(context.Background(), badTo); err == nil {
		t.Error("expected invalid-to error, got nil")
	} else if !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("want ErrInvalidAddress, got %v", err)
	}
}

func TestSendNoRecipients(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.To, msg.Cc, msg.Bcc = nil, nil, nil
	if err := m.Send(context.Background(), msg); err == nil {
		t.Fatal("expected no-recipients error, got nil")
	} else if !errors.Is(err, mailer.ErrNoRecipients) {
		t.Errorf("want ErrNoRecipients, got %v", err)
	}
}

func TestSendTooLargeBeforeDial(t *testing.T) {
	t.Parallel()
	// Unroutable host proves the size check runs before any dial:
	// with a 20s timeout this must still fail fast.
	opts := mailer.Options{
		Host: "192.0.2.1", Port: 25,
		Encryption:     mailer.EncryptionNone,
		Timeout:        20 * time.Second,
		MaxMessageSize: 16,
	}
	m, err := mailsmtp.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = m.Close() }()

	start := time.Now()
	err = m.Send(context.Background(), basicMail())
	if !errors.Is(err, mailer.ErrMessageTooLarge) {
		t.Fatalf("want ErrMessageTooLarge, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("oversize check too slow (%v): likely dialed before checking", elapsed)
	}
}

func TestSendAfterClose(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m, err := mailsmtp.New(plainOpts(t, s))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close must be idempotent nil, got %v", err)
	}
	if err := m.Send(context.Background(), basicMail()); !errors.Is(err, mailer.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}

func TestSendQuit421Swallowed(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{quit: "421 closing\r\n"})
	m := openPlain(t, s)
	// Custody already handed off at DATA 250; QUIT failure must not fail Send.
	if err := m.Send(context.Background(), basicMail()); err != nil {
		t.Errorf("QUIT 421 must be swallowed, got %v", err)
	}
}

func TestSendData554Fails(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{dataEnd: "554 rejected\r\n"})
	m := openPlain(t, s)
	if err := m.Send(context.Background(), basicMail()); err == nil {
		t.Fatal("expected DATA-verdict error, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "data") {
		t.Errorf("error should mention data: %v", err)
	}
}

func TestSendContextTimeoutVsHangingServer(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{hang: true})
	opts := plainOpts(t, s)
	opts.Timeout = 30 * time.Second
	m, err := mailsmtp.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = m.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := m.Send(ctx, basicMail()); err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("ctx deadline not respected, took %v", elapsed)
	}
}

func TestBoundariesDistinct(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	if err := m.Send(context.Background(), basicMail()); err != nil {
		t.Fatalf("Send 1: %v", err)
	}
	if err := m.Send(context.Background(), basicMail()); err != nil {
		t.Fatalf("Send 2: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data) != 2 {
		t.Fatalf("expected 2 DATA captures, got %d", len(s.data))
	}
	b1, b2 := mixedBoundary(t, s.data[0]), mixedBoundary(t, s.data[1])
	if b1 == b2 {
		t.Errorf("boundaries must differ, both %q", b1)
	}
}

func mixedBoundary(t *testing.T, raw string) string {
	t.Helper()
	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "content-type: multipart/mixed;") {
			idx := strings.Index(line, "boundary=")
			if idx < 0 {
				t.Fatalf("mixed part without boundary: %q", line)
			}
			return strings.Trim(line[idx+len("boundary="):], `"`)
		}
	}
	t.Fatal("no multipart/mixed header found")
	return ""
}

func TestHeaderInjectionStripped(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)

	msg := basicMail()
	msg.Cc, msg.Bcc, msg.Attachments = nil, nil, nil
	msg.Subject = "hi\r\nBcc: evil@example.com"
	msg.Body = "hello"
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	raw := s.lastDATA(t)
	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "bcc:") {
			t.Fatalf("injection leaked header line: %q", line)
		}
	}
	if !strings.Contains(raw, "Subject: hiBcc: evil@example.com") {
		t.Errorf("sanitized subject not found in:\n%s", raw)
	}
}
