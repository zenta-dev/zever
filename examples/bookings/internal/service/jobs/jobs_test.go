package jobs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/jobs"
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/mailer"
	mailerlog "github.com/zenta-dev/zever/mailer/log"
	"github.com/zenta-dev/zever/notification"
	notificationlog "github.com/zenta-dev/zever/notification/log"
	"github.com/zenta-dev/zever/queue"

	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/queue/memory"
	"github.com/zenta-dev/zever/webhook"
	_ "github.com/zenta-dev/zever/webhook/queue"
)

func migrate(t *testing.T, database db.DB) {
	t.Helper()
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
}

func newTestDB(t *testing.T) (db.DB, log.Logger, context.Context) {
	t.Helper()
	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	logger, err := c.Log()
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	ctx := context.Background()
	migrate(t, database)
	return database, logger, ctx
}

func seedBooking(t *testing.T, database db.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, []any{"host-1", "host@example.com", "hash", now}},
		{`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, []any{"guest-1", "guest@example.com", "hash", now}},
		{`INSERT INTO spaces (id, host_id, title, description, lat, lng, price_cents, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, []any{"space-1", "host-1", "Cabin", "Woods cabin", 1.0, 2.0, int64(10000), now}},
		{`INSERT INTO bookings (id, space_id, guest_id, start_date, end_date, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, []any{"booking-1", "space-1", "guest-1", "2030-06-01", "2030-06-03", "confirmed", now}},
	} {
		if _, err := database.Exec(ctx, q.query, q.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func testDeps(t *testing.T, database db.DB, logger log.Logger) (jobs.Deps, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var mailBuf, notifyBuf bytes.Buffer
	m, err := mailerlog.NewWithWriter(mailer.Options{Host: "localhost", Port: 25}, &mailBuf)
	if err != nil {
		t.Fatalf("mailer: %v", err)
	}
	n, err := notificationlog.NewWithWriter(notification.Options{}, &notifyBuf)
	if err != nil {
		t.Fatalf("notifier: %v", err)
	}
	wh, err := webhook.Open(webhook.AdapterQueue, webhook.Options{
		AllowPrivateTargets: true,
		QueueAdapter:        "memory",
		// Short poll so a lost push wakeup in the memory queue costs at most
		// ~100ms instead of the 5s default; the 5s delivery deadline below
		// then holds deterministically under -race.
		QueueOpts:       queue.Options{VisibilityTimeout: 30 * time.Second, PollTimeout: 100 * time.Millisecond},
		DeadLetterTopic: "webhook.dead",
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("webhook open: %v", err)
	}
	t.Cleanup(func() { _ = wh.Close() })
	return jobs.Deps{DB: database, Mailer: m, Notifier: n, Webhook: wh, Logger: logger}, &mailBuf, &notifyBuf
}

// TestRunConfirmationSendsReceiptAndFansOutWebhook seeds one booking, runs the
// confirmation, and asserts mail output plus webhook delivery.
func TestRunConfirmationSendsReceiptAndFansOutWebhook(t *testing.T) {
	database, logger, ctx := newTestDB(t)
	seedBooking(t, database)
	deps, mailBuf, _ := testDeps(t, database, logger)

	var got atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		got.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := deps.Webhook.Register(ctx, jobs.EventCreated, srv.URL+"/hook", "test-secret"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := jobs.RunConfirmation(ctx, deps, "booking-1"); err != nil {
		t.Fatalf("RunConfirmation: %v", err)
	}

	if !strings.Contains(mailBuf.String(), "guest@example.com") {
		t.Fatalf("mail output missing guest address: %q", mailBuf.String())
	}

	deadline := time.Now().Add(5 * time.Second)
	for got.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got.Load() == 0 {
		t.Fatalf("webhook target got no delivery")
	}
}

// TestConfirmationHandlerShape ensures the job.Register-compatible handler
// decodes the booking_id payload and runs.
func TestConfirmationHandlerShape(t *testing.T) {
	database, logger, ctx := newTestDB(t)
	seedBooking(t, database)
	deps, _, _ := testDeps(t, database, logger)

	handle := jobs.ConfirmationHandler(deps)
	raw, _ := json.Marshal(jobs.SendConfirmationArgs{BookingID: "booking-1"})
	_ = raw
	if err := handle(ctx, jobs.SendConfirmationArgs{BookingID: "booking-1"}); err != nil {
		t.Fatalf("Handler: %v", err)
	}
	if err := handle(ctx, jobs.SendConfirmationArgs{BookingID: "missing"}); err == nil {
		t.Fatalf("Handler missing booking: want error")
	}
}

// TestReminderCountsDueSoonBookings seeds one upcoming and one past booking,
// then asserts only the upcoming one is reminded.
func TestReminderCountsDueSoonBookings(t *testing.T) {
	database, logger, ctx := newTestDB(t)
	seedBooking(t, database)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(ctx,
		`INSERT INTO bookings (id, space_id, guest_id, start_date, end_date, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"booking-past", "space-1", "guest-1", "2020-01-01", "2020-01-02", "confirmed", now,
	); err != nil {
		t.Fatalf("seed past: %v", err)
	}
	deps, _, notifyBuf := testDeps(t, database, logger)

	n, err := jobs.RunReminder(ctx, deps, time.Date(2030, 5, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunReminder: %v", err)
	}
	if n != 1 {
		t.Fatalf("RunReminder = %d, want 1", n)
	}
	if !strings.Contains(notifyBuf.String(), "booking-1") {
		t.Fatalf("notify output missing booking id: %q", notifyBuf.String())
	}

	handle := jobs.ReminderHandler(deps)
	if err := handle(ctx, jobs.SendReminderArgs{}); err != nil {
		t.Fatalf("ReminderHandler: %v", err)
	}
}

// TestRegisterDemoTargetsFanout registers httptest targets via env lookup and
// delivers to both events.
func TestRegisterDemoTargetsFanout(t *testing.T) {
	database, logger, _ := newTestDB(t)
	deps, _, _ := testDeps(t, database, logger)
	ctx := context.Background()

	var created, cancelled atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/created":
			created.Add(1)
		case "/cancelled":
			cancelled.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv(jobs.EnvCreatedTarget, srv.URL+"/created")
	t.Setenv(jobs.EnvCreatedSecret, "s1")
	t.Setenv(jobs.EnvCancelledTarget, srv.URL+"/cancelled")
	t.Setenv(jobs.EnvCancelledSecret, "s2")

	if err := jobs.RegisterDemoTargets(ctx, deps.Webhook, os.Getenv); err != nil {
		t.Fatalf("RegisterDemoTargets: %v", err)
	}

	if err := deps.Webhook.Deliver(ctx, jobs.EventCreated, []byte(`{"id":"b1"}`)); err != nil {
		t.Fatalf("Deliver created: %v", err)
	}
	if err := deps.Webhook.Deliver(ctx, jobs.EventCancelled, []byte(`{"id":"b1"}`)); err != nil {
		t.Fatalf("Deliver cancelled: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for (created.Load() == 0 || cancelled.Load() == 0) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if created.Load() == 0 || cancelled.Load() == 0 {
		t.Fatalf("fanout incomplete: created=%d cancelled=%d", created.Load(), cancelled.Load())
	}
}
