package log

import (
	"io"
	"testing"

	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/mailer/mailertest"
)

// TestLogConformance proves the log adapter honors the mailer.Mailer
// contract via the shared conformance kit. Messages are discarded
// (no network, no relay).
func TestLogConformance(t *testing.T) {
	t.Parallel()

	mailertest.Conformance(t, func(t *testing.T) mailer.Mailer {
		t.Helper()

		m, err := NewWithWriter(mailer.Options{Host: "kit.example", Port: 587}, io.Discard)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = m.Close() })

		return m
	})
}
