package log

import (
	"io"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
)

// TestEdgeSend_emptySubjectAndBody proves empty subject and body are valid
// and still emit one JSON line.
func TestEdgeSend_emptySubjectAndBody(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	m, err := NewWithWriter(mailer.Options{Host: "smtp.example.com", Port: 587}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	msg := &mailer.Mail{
		From: mailer.Address{Address: "from@example.com"},
		To:   []mailer.Address{{Address: "to@example.com"}},
	}

	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !strings.Contains(buf.String(), `"subject":""`) {
		t.Fatalf("output %q missing empty subject", buf.String())
	}
}

// TestEdgeSend_manyRecipients proves a large recipient set encodes without
// truncation.
func TestEdgeSend_manyRecipients(t *testing.T) {
	t.Parallel()

	m, err := NewWithWriter(mailer.Options{Host: "smtp.example.com", Port: 587}, io.Discard)
	if err != nil {
		t.Fatalf("NewWithWriter: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	to := make([]mailer.Address, 256)
	for i := range to {
		to[i] = mailer.Address{Address: "to@example.com"}
	}

	msg := &mailer.Mail{From: mailer.Address{Address: "from@example.com"}, To: to}

	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
}
