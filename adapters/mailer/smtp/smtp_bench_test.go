package smtp

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
)

// benchMail builds a minimal valid message.
func benchMail() *mailer.Mail {
	return &mailer.Mail{
		From:    mailer.Address{Address: "from@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Subject: "subject",
		Body:    "body",
	}
}

// BenchmarkBuildMIME measures MIME rendering (boundaries, headers, base64).
func BenchmarkBuildMIME(b *testing.B) {
	msg := benchMail()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := buildMIME(msg); err != nil {
			b.Fatalf("buildMIME: %v", err)
		}
	}
}

// BenchmarkEstimateSize measures the pre-dial size gate.
func BenchmarkEstimateSize(b *testing.B) {
	msg := benchMail()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = estimateSize(msg, 1)
	}
}

// benchServer starts a minimal in-process SMTP server accepting one DATA
// transaction per connection, returning its address.
func benchServer(b *testing.B) string {
	b.Helper()

	ln, err := (&net.ListenConfig{}).Listen(b.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}

	b.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			go serveBenchSMTP(c)
		}
	}()

	return ln.Addr().String()
}

// serveBenchSMTP implements the minimal ESMTP dialogue needed by Send.
func serveBenchSMTP(c net.Conn) {
	defer func() { _ = c.Close() }()

	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)

	fmt.Fprint(w, "220 bench ESMTP\r\n")
	_ = w.Flush()

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}

		switch {
		case strings.HasPrefix(strings.ToUpper(line), "EHLO"):
			fmt.Fprint(w, "250-bench\r\n250 OK\r\n")
		case strings.HasPrefix(strings.ToUpper(line), "DATA"):
			fmt.Fprint(w, "354 go\r\n")
			_ = w.Flush()

			for {
				dl, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dl, "\r\n") == "." {
					break
				}
			}

			fmt.Fprint(w, "250 OK\r\n")
		case strings.HasPrefix(strings.ToUpper(line), "QUIT"):
			fmt.Fprint(w, "221 Bye\r\n")
			_ = w.Flush()
			return
		default:
			fmt.Fprint(w, "250 OK\r\n")
		}

		_ = w.Flush()
	}
}

// BenchmarkSend measures a full plaintext delivery over an in-process server.
func BenchmarkSend(b *testing.B) {
	addr := benchServer(b)

	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		b.Fatalf("SplitHostPort: %v", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		b.Fatalf("Atoi: %v", err)
	}

	m, err := New(mailer.Options{Host: "127.0.0.1", Port: port, Encryption: mailer.EncryptionNone})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = m.Close() })

	ctx := b.Context()
	msg := benchMail()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := m.Send(ctx, msg); err != nil {
			b.Fatalf("Send: %v", err)
		}
	}
}
