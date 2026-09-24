// White-box coverage for the SMTP adapter: field asserts, error-seam
// tests, and unit tests for unexported helpers. Tests that mutate
// package-wide seams or process env stay sequential on purpose.
package smtp

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/mailer"
)

var errCoverFail = errors.New("cover: forced failure")

type coverFailWriter struct{}

func (coverFailWriter) Write([]byte) (int, error) { return 0, errCoverFail }

func coverInternalMail() *mailer.Mail {
	return &mailer.Mail{
		From:    mailer.Address{Address: "sender@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Subject: "s",
		Body:    "b",
	}
}

func TestCoverNewDefaults(t *testing.T) {
	t.Parallel()
	m, err := New(mailer.Options{Host: "mail.example.com", Port: 25})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = m.Close() }()
	sm, ok := m.(*smtpMailer)
	if !ok {
		t.Fatalf("New type = %T, want *smtpMailer", m)
	}
	if sm.enc != mailer.EncryptionSTARTTLS {
		t.Errorf("enc = %q, want STARTTLS default", sm.enc)
	}
	if sm.timeout != mailer.DefaultTimeout {
		t.Errorf("timeout = %v, want %v", sm.timeout, mailer.DefaultTimeout)
	}
	if sm.maxSize != mailer.DefaultMaxMessageSize {
		t.Errorf("maxSize = %d, want %d", sm.maxSize, mailer.DefaultMaxMessageSize)
	}
	if sm.auth != nil {
		t.Error("auth must be nil without credentials")
	}
	if sm.addr != net.JoinHostPort("mail.example.com", "25") {
		t.Errorf("addr = %q", sm.addr)
	}
	if sm.host != "mail.example.com" {
		t.Errorf("host = %q", sm.host)
	}
}

func TestCoverNewEncryptionModes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		enc      mailer.Encryption
		user     string
		pass     string
		wantAuth bool
	}{
		{mailer.EncryptionNone, "", "", false},
		{mailer.EncryptionSTARTTLS, "", "", false},
		{mailer.EncryptionSTARTTLS, "u", "p", true},
		{mailer.EncryptionImplicitTLS, "u", "p", true},
	}
	for _, c := range cases {
		m, err := New(mailer.Options{
			Host: "h.example", Port: 587,
			Encryption: c.enc, Username: c.user, Password: c.pass,
		})
		if err != nil {
			t.Fatalf("New(%q): %v", c.enc, err)
		}
		sm, ok := m.(*smtpMailer)
		if !ok {
			t.Fatalf("New type = %T, want *smtpMailer", m)
		}
		if sm.enc != c.enc {
			t.Errorf("enc = %q, want %q", sm.enc, c.enc)
		}
		if (sm.auth != nil) != c.wantAuth {
			t.Errorf("enc %q auth non-nil = %v, want %v", c.enc, sm.auth != nil, c.wantAuth)
		}
		_ = m.Close()
	}
}

func TestCoverRandomBoundaryOK(t *testing.T) {
	t.Parallel()
	b1, err := randomBoundary()
	if err != nil {
		t.Fatalf("randomBoundary: %v", err)
	}
	b2, err := randomBoundary()
	if err != nil {
		t.Fatalf("randomBoundary: %v", err)
	}
	if !strings.HasPrefix(b1, "zever-") || len(b1) != len("zever-")+32 {
		t.Errorf("bad boundary shape: %q", b1)
	}
	if b1 == b2 {
		t.Error("boundaries must differ")
	}
}

// Sequential: overrides the package-wide randRead seam.
func TestCoverRandomSeam(t *testing.T) {
	old := randRead
	t.Cleanup(func() { randRead = old })
	boom := errors.New("cover: no entropy")
	randRead = func([]byte) (int, error) { return 0, boom }

	if _, err := randomBoundary(); !errors.Is(err, boom) {
		t.Errorf("randomBoundary err = %v, want wrap of %v", err, boom)
	}
	if _, err := buildMIME(coverInternalMail()); !errors.Is(err, boom) {
		t.Errorf("buildMIME first-boundary err = %v, want wrap of %v", err, boom)
	}

	m, err := New(mailer.Options{
		Host: "127.0.0.1", Port: 9,
		Encryption: mailer.EncryptionNone, Timeout: time.Second, MaxMessageSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = m.Close() }()
	err = m.Send(t.Context(), coverInternalMail())
	if err == nil || !strings.Contains(err.Error(), "build message") {
		t.Errorf("Send err = %v, want build-message wrap", err)
	}

	calls := 0
	randRead = func(b []byte) (int, error) {
		calls++
		if calls == 2 {
			return 0, boom
		}
		return rand.Read(b)
	}
	if _, err := buildMIME(coverInternalMail()); !errors.Is(err, boom) {
		t.Errorf("buildMIME second-boundary err = %v, want wrap of %v", err, boom)
	}
}

func TestCoverBuildMIMEInline(t *testing.T) {
	t.Parallel()
	msg := coverInternalMail()
	msg.Attachments = []mailer.Attachment{{
		Name: "p.png", Content: []byte("x"), Inline: true, ContentID: "img1",
	}}
	raw, err := buildMIME(msg)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	if !strings.Contains(string(raw), "inline;") || !strings.Contains(string(raw), "<img1>") {
		t.Errorf("inline attachment not rendered:\n%s", raw)
	}
}

func TestCoverTLSServerName(t *testing.T) {
	t.Parallel()
	if got := tlsServerName("127.0.0.1"); got != "" {
		t.Errorf("IPv4 -> %q, want empty", got)
	}
	if got := tlsServerName("::1"); got != "" {
		t.Errorf("IPv6 -> %q, want empty", got)
	}
	if got := tlsServerName("mail.example.com"); got != "mail.example.com" {
		t.Errorf("hostname -> %q", got)
	}
	if got := tlsServerName("localhost"); got != "localhost" {
		t.Errorf("localhost -> %q", got)
	}
}

func TestCoverEstimateSize(t *testing.T) {
	t.Parallel()
	msg := &mailer.Mail{Subject: "s", Body: "b"}
	// Base: 1 + 1 + 0 + 512 header overhead + 1 recipient * 32.
	if got := estimateSize(msg, 1); got != 546 {
		t.Errorf("base = %d, want 546", got)
	}
	empty := &mailer.Mail{
		Subject: "s", Body: "b",
		Attachments: []mailer.Attachment{{Name: "e.bin"}},
	}
	// Empty content encodes to zero bytes, so no CRLF padding applies.
	if got := estimateSize(empty, 1); got != 546+256 {
		t.Errorf("empty attachment = %d, want %d", got, 546+256)
	}
	full := func(n int) *mailer.Mail {
		return &mailer.Mail{
			Subject: "s", Body: "b",
			Attachments: []mailer.Attachment{{Name: "a.bin", Content: bytes.Repeat([]byte("x"), n)}},
		}
	}
	// 57 bytes -> 76 base64 chars on a single line, no CRLF.
	if got := estimateSize(full(57), 1); got != 546+256+76 {
		t.Errorf("57-byte attachment = %d, want %d", got, 546+256+76)
	}
	// 58 bytes -> 80 base64 chars spanning two lines, one CRLF pair.
	if got := estimateSize(full(58), 1); got != 546+256+82 {
		t.Errorf("58-byte attachment = %d, want %d", got, 546+256+82)
	}
	// 200 bytes -> 268 base64 chars, three CRLF pairs (6 bytes).
	if got := estimateSize(full(200), 1); got != 546+256+274 {
		t.Errorf("200-byte attachment = %d, want %d", got, 546+256+274)
	}
}

func TestCoverWriteQP(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeQP(&buf, "plain ascii"); err != nil {
		t.Fatalf("writeQP: %v", err)
	}
	if !strings.Contains(buf.String(), "plain ascii") {
		t.Errorf("qp output = %q", buf.String())
	}
	buf.Reset()
	if err := writeQP(&buf, "héllo wörld, this line is long enough to force soft breaks\r\n"); err != nil {
		t.Fatalf("writeQP unicode: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("qp output must not be empty")
	}
	if err := writeQP(coverFailWriter{}, strings.Repeat("x", 200)); err == nil {
		t.Error("writeQP with failing writer must fail")
	}
}

func TestCoverWriteBase64(t *testing.T) {
	t.Parallel()
	content := bytes.Repeat([]byte("abcdefgh"), 25) // 200 bytes, multi-line output.
	var buf bytes.Buffer
	if err := writeBase64(&buf, content); err != nil {
		t.Fatalf("writeBase64: %v", err)
	}
	var raw strings.Builder
	for _, line := range strings.Split(buf.String(), "\r\n") {
		if line == "" {
			continue
		}
		if len(line) > 76 {
			t.Errorf("base64 line too long: %d", len(line))
		}
		raw.WriteString(line)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw.String())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(decoded, content) {
		t.Error("base64 round-trip mismatch")
	}
	buf.Reset()
	if err := writeBase64(&buf, nil); err != nil || buf.Len() != 0 {
		t.Errorf("empty content: err=%v len=%d", err, buf.Len())
	}
	if err := writeBase64(coverFailWriter{}, []byte("boom")); err == nil {
		t.Error("writeBase64 with failing writer must fail")
	}
}

func TestCoverEncodeHeader(t *testing.T) {
	t.Parallel()
	if got := encodeHeader("plain ascii"); got != "plain ascii" {
		t.Errorf("ascii -> %q", got)
	}
	got := encodeHeader("héllo")
	if !strings.HasPrefix(got, "=?utf-8?") {
		t.Errorf("unicode -> %q, want Q-encoding", got)
	}
}

func TestCoverJoinAddresses(t *testing.T) {
	t.Parallel()
	got := joinAddresses([]mailer.Address{{Address: "a@example.com"}, {Address: "b@example.com"}})
	if got != "a@example.com, b@example.com" {
		t.Errorf("join = %q", got)
	}
}

func TestCoverSanitize(t *testing.T) {
	t.Parallel()
	if got := sanitizeHeader("a\r\nBcc: evil@example.com"); got != "aBcc: evil@example.com" {
		t.Errorf("sanitizeHeader = %q", got)
	}
	if got := sanitizeFilename("a\"b\\c\r\nd.txt"); got != "abcd.txt" {
		t.Errorf("sanitizeFilename = %q", got)
	}
}

func TestCoverContentType(t *testing.T) {
	t.Parallel()
	if got := contentType("a.txt"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("txt -> %q", got)
	}
	if got := contentType("a.unknownext123"); got != "application/octet-stream" {
		t.Errorf("unknown -> %q", got)
	}
	if got := contentType("noext"); got != "application/octet-stream" {
		t.Errorf("no ext -> %q", got)
	}
}

func startCoverBlackhole(t *testing.T) int {
	t.Helper()
	lc := net.ListenConfig{}
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr = %T, want *net.TCPAddr", ln.Addr())
	}
	return addr.Port
}

// Sequential: overrides the package-wide setDeadline seam.
func TestCoverDialSetDeadlineFail(t *testing.T) {
	old := setDeadline
	t.Cleanup(func() { setDeadline = old })
	setDeadline = func(net.Conn, time.Time) error { return errCoverFail }

	m, err := New(mailer.Options{
		Host: "127.0.0.1", Port: startCoverBlackhole(t),
		Encryption: mailer.EncryptionNone, Timeout: 5 * time.Second, MaxMessageSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = m.Close() }()
	err = m.Send(t.Context(), coverInternalMail())
	if err == nil || !strings.Contains(err.Error(), "set deadline") {
		t.Errorf("Send err = %v, want set-deadline failure", err)
	}
}

type coverTLSCap struct {
	mu   sync.Mutex
	auth []string
	from []string
	rcpt []string
	data []string
}

func coverTestCert(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("CA key: %v", err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "zever test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}
	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("server key: %v", err)
	}
	srvTpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("server cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(srvKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	srvCert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatalf("key pair: %v", err)
	}
	return srvCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

func startCoverTLSServer(t *testing.T, cert tls.Certificate, authResp string, seen *coverTLSCap) int {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("TLS listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveCoverTLSConn(c, authResp, seen)
		}
	}()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr = %T, want *net.TCPAddr", ln.Addr())
	}
	return addr.Port
}

func serveCoverTLSConn(c net.Conn, authResp string, seen *coverTLSCap) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	reply := func(s string) bool {
		if _, err := w.WriteString(s); err != nil {
			return false
		}
		return w.Flush() == nil
	}
	if !reply("220 fake SMTPS\r\n") {
		return
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.Fields(line)[0])
		switch cmd {
		case "EHLO":
			if !reply("250-localhost\r\n250 AUTH PLAIN\r\n") {
				return
			}
		case "HELO":
			if !reply("250 localhost\r\n") {
				return
			}
		case "AUTH":
			seen.mu.Lock()
			seen.auth = append(seen.auth, line)
			seen.mu.Unlock()
			if !reply(authResp) {
				return
			}
		case "MAIL":
			seen.mu.Lock()
			seen.from = append(seen.from, line)
			seen.mu.Unlock()
			if !reply("250 OK\r\n") {
				return
			}
		case "RCPT":
			seen.mu.Lock()
			seen.rcpt = append(seen.rcpt, line)
			seen.mu.Unlock()
			if !reply("250 OK\r\n") {
				return
			}
		case "DATA":
			if !reply("354 End with .\r\n") {
				return
			}
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
			seen.mu.Lock()
			seen.data = append(seen.data, b.String())
			seen.mu.Unlock()
			if !reply("250 OK\r\n") {
				return
			}
		case "QUIT":
			_ = reply("221 Bye\r\n")
			return
		default:
			if !reply("250 OK\r\n") {
				return
			}
		}
	}
}

// coverTrustCA points the process root pool at our test CA. Callers stay
// sequential because SSL_CERT_FILE is process-wide.
func coverTrustCA(t *testing.T, caPEM []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, caPEM, 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}
	t.Setenv("SSL_CERT_FILE", path)
}

// Sequential: sets process-wide SSL_CERT_FILE. Both steps share one CA
// because the process caches the system root pool after first use.
func TestCoverImplicitTLS(t *testing.T) {
	srvCert, caPEM := coverTestCert(t)
	seen := &coverTLSCap{}
	port := startCoverTLSServer(t, srvCert, "235 OK\r\n", seen)
	coverTrustCA(t, caPEM)

	t.Run("success", func(t *testing.T) {
		m, err := New(mailer.Options{
			Host: "localhost", Port: port, Username: "u", Password: "p",
			Encryption: mailer.EncryptionImplicitTLS, Timeout: 5 * time.Second, MaxMessageSize: 1 << 20,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = m.Close() }()
		if err := m.Send(t.Context(), coverInternalMail()); err != nil {
			t.Fatalf("Send: %v", err)
		}
		seen.mu.Lock()
		defer seen.mu.Unlock()
		if len(seen.auth) != 1 {
			t.Errorf("server saw %d AUTH, want 1", len(seen.auth))
		}
		if len(seen.from) != 1 || len(seen.rcpt) != 1 || len(seen.data) != 1 {
			t.Errorf("envelope = %d/%d/%d, want 1/1/1", len(seen.from), len(seen.rcpt), len(seen.data))
		}
	})

	t.Run("authfail", func(t *testing.T) {
		badPort := startCoverTLSServer(t, srvCert, "535 bad credentials\r\n", &coverTLSCap{})
		m, err := New(mailer.Options{
			Host: "localhost", Port: badPort, Username: "u", Password: "p",
			Encryption: mailer.EncryptionImplicitTLS, Timeout: 5 * time.Second, MaxMessageSize: 1 << 20,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = m.Close() }()
		err = m.Send(t.Context(), coverInternalMail())
		if err == nil || !strings.Contains(err.Error(), "auth") {
			t.Errorf("Send err = %v, want auth failure", err)
		}
	})

	t.Run("deadline", func(t *testing.T) {
		old := setDeadline
		t.Cleanup(func() { setDeadline = old })
		setDeadline = func(c net.Conn, d time.Time) error {
			if _, ok := c.(*tls.Conn); ok {
				return errCoverFail
			}
			return c.SetDeadline(d)
		}

		m, err := New(mailer.Options{
			Host: "localhost", Port: port,
			Encryption: mailer.EncryptionImplicitTLS, Timeout: 5 * time.Second, MaxMessageSize: 1 << 20,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = m.Close() }()
		err = m.Send(t.Context(), coverInternalMail())
		if err == nil || !strings.Contains(err.Error(), "set deadline") {
			t.Errorf("Send err = %v, want set-deadline failure", err)
		}
	})
}
