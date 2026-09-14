// Scripted-server coverage for the SMTP adapter: every Send error path
// reachable over plaintext, plus envelope validation. Reuses the
// helpers from smtp_test.go; each test owns its listener on 127.0.0.1:0.
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

type coverScript struct {
	ehloResp     string
	heloResp     string
	starttlsResp string
	authResp     string
	mailResp     string
	rcptResp     string
	dataResp     string // "RST" means 354 then destroy the connection.
	dataEnd      string
}

type coverCap struct {
	mu   sync.Mutex
	auth []string
	from []string
	rcpt []string
	data []string
}

func startCoverScript(t *testing.T, cfg coverScript) (int, *coverCap) {
	t.Helper()
	if cfg.ehloResp == "" {
		cfg.ehloResp = "250-localhost\r\n250 AUTH PLAIN\r\n"
	}
	if cfg.heloResp == "" {
		cfg.heloResp = "250 localhost\r\n"
	}
	if cfg.starttlsResp == "" {
		cfg.starttlsResp = "502 no TLS here\r\n"
	}
	if cfg.authResp == "" {
		cfg.authResp = "235 OK\r\n"
	}
	if cfg.mailResp == "" {
		cfg.mailResp = "250 OK\r\n"
	}
	if cfg.rcptResp == "" {
		cfg.rcptResp = "250 OK\r\n"
	}
	if cfg.dataEnd == "" {
		cfg.dataEnd = "250 OK\r\n"
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	seen := &coverCap{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveCoverScript(c, cfg, seen)
		}
	}()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr = %T, want *net.TCPAddr", ln.Addr())
	}
	return addr.Port, seen
}

func serveCoverScript(c net.Conn, cfg coverScript, seen *coverCap) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	say := func(s string) bool {
		if _, err := w.WriteString(s); err != nil {
			return false
		}
		return w.Flush() == nil
	}
	if !say("220 fake ESMTP\r\n") {
		return
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			if !say("250 OK\r\n") {
				return
			}
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "EHLO":
			if !say(cfg.ehloResp) {
				return
			}
		case "HELO":
			if !say(cfg.heloResp) {
				return
			}
		case "STARTTLS":
			if !say(cfg.starttlsResp) {
				return
			}
		case "AUTH":
			seen.mu.Lock()
			seen.auth = append(seen.auth, line)
			seen.mu.Unlock()
			if !say(cfg.authResp) {
				return
			}
		case "MAIL":
			seen.mu.Lock()
			seen.from = append(seen.from, line)
			seen.mu.Unlock()
			if !say(cfg.mailResp) {
				return
			}
		case "RCPT":
			seen.mu.Lock()
			seen.rcpt = append(seen.rcpt, line)
			seen.mu.Unlock()
			if !say(cfg.rcptResp) {
				return
			}
		case "DATA":
			if cfg.dataResp == "RST" {
				if !say("354 End with .\r\n") {
					return
				}
				// RST the connection so the client's body write fails.
				if tc, ok := c.(*net.TCPConn); ok {
					_ = tc.SetLinger(0)
				}
				_ = c.Close()
				return
			}
			if cfg.dataResp != "" {
				if !say(cfg.dataResp) {
					return
				}
				continue
			}
			if !say("354 End with .\r\n") {
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
			if !say(cfg.dataEnd) {
				return
			}
		case "QUIT":
			if !say("221 Bye\r\n") {
				return
			}
			return
		default:
			if !say("250 OK\r\n") {
				return
			}
		}
	}
}

func coverOpts(port int, enc mailer.Encryption) mailer.Options {
	return mailer.Options{
		Host:           "127.0.0.1",
		Port:           port,
		Encryption:     enc,
		Timeout:        5 * time.Second,
		MaxMessageSize: 1 << 20,
	}
}

func coverOpen(t *testing.T, port int, enc mailer.Encryption) mailer.Mailer {
	t.Helper()
	m, err := mailsmtp.New(coverOpts(port, enc))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func startCoverDead(t *testing.T) int {
	t.Helper()
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
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

func TestCoverDialRefused(t *testing.T) {
	t.Parallel()
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr = %T, want *net.TCPAddr", ln.Addr())
	}
	_ = ln.Close()

	m := coverOpen(t, addr.Port, mailer.EncryptionNone)
	err = m.Send(context.Background(), basicMail())
	if err == nil || !strings.Contains(err.Error(), "dial") {
		t.Errorf("Send err = %v, want dial failure", err)
	}
}

func TestCoverNewClientFails(t *testing.T) {
	t.Parallel()
	m := coverOpen(t, startCoverDead(t), mailer.EncryptionNone)
	err := m.Send(context.Background(), basicMail())
	if err == nil || !strings.Contains(err.Error(), "new client") {
		t.Errorf("Send err = %v, want new-client failure", err)
	}
}

func TestCoverHelloFails(t *testing.T) {
	t.Parallel()
	port, _ := startCoverScript(t, coverScript{
		ehloResp: "500 no EHLO\r\n",
		heloResp: "500 no HELO\r\n",
	})
	m := coverOpen(t, port, mailer.EncryptionNone)
	err := m.Send(context.Background(), basicMail())
	if err == nil || !strings.Contains(err.Error(), "hello") {
		t.Errorf("Send err = %v, want hello failure", err)
	}
}

func TestCoverSTARTTLSCommandFails(t *testing.T) {
	t.Parallel()
	port, _ := startCoverScript(t, coverScript{
		ehloResp:     "250-localhost\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n",
		starttlsResp: "502 no TLS here\r\n",
	})
	m := coverOpen(t, port, mailer.EncryptionSTARTTLS)
	err := m.Send(context.Background(), basicMail())
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("Send err = %v, want STARTTLS failure", err)
	}
}

func TestCoverMailFails(t *testing.T) {
	t.Parallel()
	port, _ := startCoverScript(t, coverScript{mailResp: "550 no such sender\r\n"})
	m := coverOpen(t, port, mailer.EncryptionNone)
	err := m.Send(context.Background(), basicMail())
	if err == nil || !strings.Contains(err.Error(), "mail from") {
		t.Errorf("Send err = %v, want mail-from failure", err)
	}
}

func TestCoverRcptFails(t *testing.T) {
	t.Parallel()
	port, _ := startCoverScript(t, coverScript{rcptResp: "550 no such user\r\n"})
	m := coverOpen(t, port, mailer.EncryptionNone)
	err := m.Send(context.Background(), basicMail())
	if err == nil || !strings.Contains(err.Error(), "rcpt to") {
		t.Errorf("Send err = %v, want rcpt-to failure", err)
	}
}

func TestCoverDataFails(t *testing.T) {
	t.Parallel()
	port, _ := startCoverScript(t, coverScript{dataResp: "503 bad sequence\r\n"})
	m := coverOpen(t, port, mailer.EncryptionNone)
	err := m.Send(context.Background(), basicMail())
	if err == nil || !strings.Contains(err.Error(), "smtp: data:") {
		t.Errorf("Send err = %v, want DATA failure", err)
	}
}

func TestCoverWriteFails(t *testing.T) {
	t.Parallel()
	port, _ := startCoverScript(t, coverScript{dataResp: "RST"})
	m := coverOpen(t, port, mailer.EncryptionNone)
	msg := basicMail()
	msg.Attachments = append(msg.Attachments, mailer.Attachment{
		Name:    "big.bin",
		Content: make([]byte, 1<<16),
	})
	err := m.Send(context.Background(), msg)
	if err == nil || !strings.Contains(err.Error(), "write data") {
		t.Errorf("Send err = %v, want write-data failure", err)
	}
}

func TestCoverCcInvalid(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)
	msg := basicMail()
	msg.Cc = []mailer.Address{{Address: "bad\r\n@example.com"}}
	err := m.Send(context.Background(), msg)
	if err == nil || !strings.Contains(err.Error(), "cc") {
		t.Errorf("Send err = %v, want cc failure", err)
	} else if !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("want ErrInvalidAddress, got %v", err)
	}
}

func TestCoverBccInvalid(t *testing.T) {
	t.Parallel()
	s := startFakeSMTP(t, serverConfig{})
	m := openPlain(t, s)
	msg := basicMail()
	msg.Bcc = []mailer.Address{{Address: "no-at-sign"}}
	err := m.Send(context.Background(), msg)
	if err == nil || !strings.Contains(err.Error(), "bcc") {
		t.Errorf("Send err = %v, want bcc failure", err)
	} else if !errors.Is(err, mailer.ErrInvalidAddress) {
		t.Errorf("want ErrInvalidAddress, got %v", err)
	}
}

func TestCoverAuthCapture(t *testing.T) {
	t.Parallel()
	port, seen := startCoverScript(t, coverScript{})
	m := coverOpen(t, port, mailer.EncryptionNone)
	msg := basicMail()
	msg.Cc, msg.Bcc, msg.Attachments = nil, nil, nil
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	seen.mu.Lock()
	defer seen.mu.Unlock()
	if len(seen.from) != 1 || !strings.Contains(seen.from[0], "sender@example.com") {
		t.Errorf("MAIL FROM not seen: %q", seen.from)
	}
	if fmt.Sprint(seen.rcpt) == "" || len(seen.data) != 1 {
		t.Errorf("envelope incomplete: rcpt=%q data=%d", seen.rcpt, len(seen.data))
	}
}
