package smtp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/mailer"
)

var _ mailer.Mailer = (*smtpMailer)(nil)

// randRead is a seam for hermetic tests: overriding it forces
// randomBoundary failures without touching crypto/rand.
var randRead = rand.Read

// setDeadline is a seam for hermetic tests: overriding it forces the
// dial SetDeadline error branches, which real sockets cannot trigger
// (SetDeadline on a live conn does not fail).
var setDeadline = func(c net.Conn, t time.Time) error { return c.SetDeadline(t) }

type smtpMailer struct {
	addr    string
	host    string
	enc     mailer.Encryption
	timeout time.Duration
	maxSize int
	auth    smtp.Auth

	closed atomic.Bool
}

// New builds an SMTP mailer.Mailer from opts. Zero Encryption, Timeout,
// and MaxMessageSize resolve to STARTTLS, DefaultTimeout, and
// DefaultMaxMessageSize.
func New(opts mailer.Options) (mailer.Mailer, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("smtp: invalid options: %w", err)
	}

	enc := opts.Encryption
	if enc == "" {
		enc = mailer.EncryptionSTARTTLS
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = mailer.DefaultTimeout
	}
	maxSize := opts.MaxMessageSize
	if maxSize == 0 {
		maxSize = mailer.DefaultMaxMessageSize
	}

	var auth smtp.Auth
	if opts.Username != "" && opts.Password != "" {
		auth = smtp.PlainAuth("", opts.Username, opts.Password, opts.Host)
	}

	return &smtpMailer{
		addr:    net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port)),
		host:    opts.Host,
		enc:     enc,
		timeout: timeout,
		maxSize: maxSize,
		auth:    auth,
	}, nil
}

// Send delivers msg over a fresh per-send connection.
func (m *smtpMailer) Send(ctx context.Context, msg *mailer.Mail) error {
	if msg == nil {
		return ErrNilMessage
	}
	if m.closed.Load() {
		return fmt.Errorf("smtp: send: %w", mailer.ErrClosed)
	}
	if err := msg.From.Validate(); err != nil {
		return fmt.Errorf("smtp: from: %w", err)
	}
	for i := range msg.To {
		if err := msg.To[i].Validate(); err != nil {
			return fmt.Errorf("smtp: to: %w", err)
		}
	}
	for i := range msg.Cc {
		if err := msg.Cc[i].Validate(); err != nil {
			return fmt.Errorf("smtp: cc: %w", err)
		}
	}
	for i := range msg.Bcc {
		if err := msg.Bcc[i].Validate(); err != nil {
			return fmt.Errorf("smtp: bcc: %w", err)
		}
	}

	rcpts := make([]string, 0, len(msg.To)+len(msg.Cc)+len(msg.Bcc))
	for _, a := range msg.To {
		rcpts = append(rcpts, a.Address)
	}
	for _, a := range msg.Cc {
		rcpts = append(rcpts, a.Address)
	}
	for _, a := range msg.Bcc {
		rcpts = append(rcpts, a.Address)
	}
	if len(rcpts) == 0 {
		return fmt.Errorf("smtp: send: %w", mailer.ErrNoRecipients)
	}
	if estimateSize(msg, len(rcpts)) > int64(m.maxSize) {
		return fmt.Errorf("smtp: message size exceeds limit: %w", mailer.ErrMessageTooLarge)
	}

	body, err := buildMIME(msg)
	if err != nil {
		return fmt.Errorf("smtp: build message: %w", err)
	}

	conn, err := m.dial(ctx)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err = client.Hello("zever"); err != nil {
		return fmt.Errorf("smtp: hello: %w", err)
	}

	if m.enc == mailer.EncryptionSTARTTLS {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return ErrSTARTTLSRequired
		}
		cfg := &tls.Config{
			ServerName: tlsServerName(m.host),
			MinVersion: tls.VersionTLS12,
		}
		if err = client.StartTLS(cfg); err != nil {
			return fmt.Errorf("smtp: STARTTLS: %w", err)
		}
	}

	if m.auth != nil {
		if err = client.Auth(m.auth); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}

	if err = client.Mail(msg.From.Address); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}
	for _, r := range rcpts {
		if err = client.Rcpt(r); err != nil {
			return fmt.Errorf("smtp: rcpt to: %w", err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp: write data: %w", err)
	}
	// Close response is the delivery verdict.
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: data close: %w", err)
	}

	// Custody already handed off at DATA 250; swallow QUIT errors.
	_ = client.Quit()
	return nil
}

// Close marks the mailer closed. Idempotent; no connections are held
// between sends so there is nothing to drain.
func (m *smtpMailer) Close() error {
	m.closed.Store(true)
	return nil
}

func (m *smtpMailer) dial(ctx context.Context) (net.Conn, error) {
	deadline := time.Now().Add(m.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	ctx2, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx2, "tcp", m.addr)
	if err != nil {
		return nil, fmt.Errorf("smtp: dial %s: %w", m.addr, err)
	}
	if err := setDeadline(conn, deadline); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp: set deadline: %w", err)
	}

	if m.enc != mailer.EncryptionImplicitTLS {
		return conn, nil
	}

	tconn := tls.Client(conn, &tls.Config{
		ServerName: tlsServerName(m.host),
		MinVersion: tls.VersionTLS12,
	})
	if err := tconn.HandshakeContext(ctx2); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp: TLS handshake: %w", err)
	}
	if err := setDeadline(tconn, deadline); err != nil {
		_ = tconn.Close()
		return nil, fmt.Errorf("smtp: set deadline: %w", err)
	}
	return tconn, nil
}

// tlsServerName returns "" for IP literals so verification uses IP SANs,
// else the hostname for SNI and hostname verification.
func tlsServerName(host string) string {
	if net.ParseIP(host) != nil {
		return ""
	}
	return host
}

func estimateSize(msg *mailer.Mail, nrcpt int) int64 {
	// 512 covers headers, MIME boundaries and envelope overhead.
	est := int64(len(msg.Subject)+len(msg.Body)+len(msg.HTML)) + 512
	for _, att := range msg.Attachments {
		est += 256 // per-attachment MIME headers
		n := int64(len(att.Content))
		encoded := (n + 2) / 3 * 4 // base64 33% overhead
		if encoded > 0 {
			encoded += (encoded - 1) / 76 * 2 // CRLF every 76 chars
		}
		est += encoded
	}
	est += int64(nrcpt) * 32 // recipient headers
	return est
}

// buildMIME renders msg as multipart/mixed with a multipart/alternative
// body part plus base64 attachments. Bcc never appears in headers.
func buildMIME(msg *mailer.Mail) ([]byte, error) {
	outerBoundary, err := randomBoundary()
	if err != nil {
		return nil, err
	}
	altBoundary, err := randomBoundary()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	outer := multipart.NewWriter(&buf)
	// SetBoundary only fails on invalid bytes; "zever-"+hex is always valid.
	_ = outer.SetBoundary(outerBoundary)

	buf.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "From: %s\r\n", msg.From.String())
	if len(msg.To) > 0 {
		fmt.Fprintf(&buf, "To: %s\r\n", joinAddresses(msg.To))
	}
	if len(msg.Cc) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", joinAddresses(msg.Cc))
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", encodeHeader(sanitizeHeader(msg.Subject)))
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=%q\r\n", outer.Boundary())
	buf.WriteString("\r\n")

	altHeader := textproto.MIMEHeader{}
	altHeader.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", altBoundary))
	altPart, _ := outer.CreatePart(altHeader) // Buffer-backed writer never fails.
	alt := multipart.NewWriter(altPart)
	// SetBoundary only fails on invalid bytes; "zever-"+hex is always valid.
	_ = alt.SetBoundary(altBoundary)
	if msg.Body != "" {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		p, _ := alt.CreatePart(h) // Buffer-backed writer never fails.
		// The part writer feeds a bytes.Buffer and cannot fail; writeQP
		// keeps its error return, unit-tested with a failing writer.
		_ = writeQP(p, msg.Body)
	}
	if msg.HTML != "" {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		p, _ := alt.CreatePart(h) // Buffer-backed writer never fails.
		// The part writer feeds a bytes.Buffer and cannot fail; writeQP
		// keeps its error return, unit-tested with a failing writer.
		_ = writeQP(p, msg.HTML)
	}
	// Closing a buffer-backed writer only emits the final boundary.
	_ = alt.Close()

	for _, att := range msg.Attachments {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", contentType(att.Name))
		h.Set("Content-Transfer-Encoding", "base64")
		name := sanitizeFilename(att.Name)
		if att.Inline && att.ContentID != "" {
			h.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, name))
			h.Set("Content-ID", "<"+sanitizeHeader(att.ContentID)+">")
		} else {
			h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
		}
		p, _ := outer.CreatePart(h) // Buffer-backed writer never fails.
		// The part writer feeds a bytes.Buffer and cannot fail; writeBase64
		// keeps its error return, unit-tested with a failing writer.
		_ = writeBase64(p, att.Content)
	}

	// Closing a buffer-backed writer only emits the final boundary.
	_ = outer.Close()
	return buf.Bytes(), nil
}

func randomBoundary() (string, error) {
	var b [16]byte
	if _, err := randRead(b[:]); err != nil {
		return "", fmt.Errorf("smtp: random boundary: %w", err)
	}
	return "zever-" + hex.EncodeToString(b[:]), nil
}

func writeQP(w io.Writer, s string) error {
	qp := quotedprintable.NewWriter(w)
	if _, err := qp.Write([]byte(s)); err != nil {
		_ = qp.Close()
		return err
	}
	return qp.Close()
}

func writeBase64(w io.Writer, content []byte) error {
	encoded := base64.StdEncoding.EncodeToString(content)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		if _, err := io.WriteString(w, encoded[i:end]+"\r\n"); err != nil {
			return err
		}
	}
	return nil
}

func joinAddresses(addrs []mailer.Address) string {
	parts := make([]string, len(addrs))
	for i, a := range addrs {
		parts[i] = a.String()
	}
	return strings.Join(parts, ", ")
}

func encodeHeader(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return mime.QEncoding.Encode("utf-8", s)
		}
	}
	return s
}

// sanitizeHeader strips CR/LF to block header injection.
func sanitizeHeader(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, s)
}

func sanitizeFilename(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, s)
	return encodeHeader(s)
}

func contentType(name string) string {
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
