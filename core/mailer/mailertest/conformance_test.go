package mailertest_test

import (
	"io"
	"testing"

	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/mailer/mailertest"
)

// TestConformanceLog proves the kit passes against the log adapter.
func TestConformanceLog(t *testing.T) {
	t.Parallel()

	mailertest.Conformance(t, func(t *testing.T) mailer.Mailer {
		t.Helper()

		m, err := mailerlog.NewWithWriter(mailer.Options{Host: "kit.example", Port: 587}, io.Discard)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = m.Close() })

		return m
	})
}
