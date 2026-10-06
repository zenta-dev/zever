package jobs_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/jobs"

	logslog "github.com/zenta-dev/zever/adapters/log/slog"
)

// BenchmarkRunReminder measures the due-soon scan plus mail fan-out over one booking.
func BenchmarkRunReminder(b *testing.B) {
	database, ctx := benchBookingDB(b)

	logger := logslog.NewWithWriter(log.Options{}, io.Discard)
	var mailBuf, notifyBuf bytes.Buffer
	m, err := mailerlog.NewWithWriter(mailer.Options{Host: "localhost", Port: 25}, &mailBuf)
	if err != nil {
		b.Fatalf("mailer: %v", err)
	}
	n, err := notificationlog.NewWithWriter(notification.Options{}, &notifyBuf)
	if err != nil {
		b.Fatalf("notifier: %v", err)
	}
	deps := jobs.Deps{DB: database, Mailer: m, Notifier: n, Logger: logger}
	now := time.Date(2030, 5, 30, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := jobs.RunReminder(ctx, deps, now); err != nil {
			b.Fatalf("RunReminder: %v", err)
		}
	}
}
