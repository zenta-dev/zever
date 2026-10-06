package jobs_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/jobs"
)

// errJobsCover is the generic backend failure job cover stubs report.
var errJobsCover = errors.New("cover: backend failure")

// stubCoverMailer fails Send on demand.
type stubCoverMailer struct{ err error }

func (s stubCoverMailer) Send(context.Context, *mailer.Mail) error { return s.err }
func (stubCoverMailer) Close() error                               { return nil }

// stubCoverNotifier fails Notify on demand.
type stubCoverNotifier struct{ err error }

func (s stubCoverNotifier) Notify(context.Context, *notification.Notification) error {
	return s.err
}
func (stubCoverNotifier) Close() error { return nil }

// stubCoverWebhook fails Register/Deliver on demand.
type stubCoverWebhook struct {
	registerErr error
	deliverErr  error
}

func (s stubCoverWebhook) Register(context.Context, string, string, string) error {
	return s.registerErr
}
func (stubCoverWebhook) Unregister(context.Context, string, string) error { return nil }
func (s stubCoverWebhook) Deliver(context.Context, string, []byte) error {
	return s.deliverErr
}
func (stubCoverWebhook) Close() error { return nil }

// coverFailDB delegates to a real database but fails queries whose SQL
// contains the configured substring, pinning job lookup error branches.
type coverFailDB struct {
	db.DB
	failQuery string
}

func (c coverFailDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	if c.failQuery != "" && strings.Contains(strings.ToLower(query), strings.ToLower(c.failQuery)) {
		return nil, errJobsCover
	}
	return c.DB.Query(ctx, query, args...)
}

// TestRunConfirmationErrors pins every RunConfirmation failure branch:
// empty id, booking lookup error, missing guest/space, mail/notify errors,
// and webhook delivery errors including the ErrNotFound no-subscriber path.
func TestRunConfirmationErrors(t *testing.T) {
	database, logger, ctx := newTestDB(t)
	seedBooking(t, database)
	deps, _, _ := testDeps(t, database, logger)

	if err := jobs.RunConfirmation(ctx, deps, ""); err == nil {
		t.Fatal("empty booking id: want error")
	}

	lookupDeps := deps
	lookupDeps.DB = coverFailDB{DB: database, failQuery: "bookings"}
	if err := jobs.RunConfirmation(ctx, lookupDeps, "booking-1"); err == nil ||
		!strings.Contains(err.Error(), "lookup booking") {
		t.Fatalf("booking lookup = %v, want lookup booking", err)
	}

	if _, err := database.Exec(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if _, err := database.Exec(ctx, `UPDATE bookings SET guest_id = ? WHERE id = ?`, "ghost", "booking-1"); err != nil {
		t.Fatalf("orphan booking: %v", err)
	}
	if err := jobs.RunConfirmation(ctx, deps, "booking-1"); !errors.Is(err, jobs.ErrGuestNotFound) {
		t.Fatalf("missing guest = %v, want ErrGuestNotFound", err)
	}

	database2, logger2, ctx2 := newTestDB(t)
	seedBooking(t, database2)
	deps2, _, _ := testDeps(t, database2, logger2)

	mailDeps := deps2
	mailDeps.Mailer = stubCoverMailer{err: errJobsCover}
	if err := jobs.RunConfirmation(ctx2, mailDeps, "booking-1"); err == nil ||
		!strings.Contains(err.Error(), "send mail") {
		t.Fatalf("mail error = %v, want send mail", err)
	}

	notifyDeps := deps2
	notifyDeps.Notifier = stubCoverNotifier{err: errJobsCover}
	if err := jobs.RunConfirmation(ctx2, notifyDeps, "booking-1"); err == nil ||
		!strings.Contains(err.Error(), "notify guest") {
		t.Fatalf("notify error = %v, want notify guest", err)
	}

	deliverDeps := deps2
	deliverDeps.Webhook = stubCoverWebhook{deliverErr: errJobsCover}
	if err := jobs.RunConfirmation(ctx2, deliverDeps, "booking-1"); err == nil ||
		!strings.Contains(err.Error(), "deliver webhook") {
		t.Fatalf("deliver error = %v, want deliver webhook", err)
	}

	quietDeps := deps2
	quietDeps.Webhook = stubCoverWebhook{deliverErr: webhook.ErrNotFound}
	if err := jobs.RunConfirmation(ctx2, quietDeps, "booking-1"); err != nil {
		t.Fatalf("no-subscriber deliver = %v, want nil", err)
	}

	if _, err := database2.Exec(ctx2, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if _, err := database2.Exec(ctx2, `UPDATE bookings SET space_id = ? WHERE id = ?`, "ghost-space", "booking-1"); err != nil {
		t.Fatalf("orphan booking space: %v", err)
	}
	if err := jobs.RunConfirmation(ctx2, deps2, "booking-1"); !errors.Is(err, jobs.ErrSpaceNotFound) {
		t.Fatalf("missing space = %v, want ErrSpaceNotFound", err)
	}

	if err := jobs.ConfirmationHandler(deps2)(ctx2, jobs.SendConfirmationArgs{}); err == nil {
		t.Fatal("handler empty booking id: want error")
	}
}

// TestRunReminderErrors pins DueBookings list errors, missing guests,
// notify failures, unparsable-date skipping, and RFC3339 date parsing.
func TestRunReminderErrors(t *testing.T) {
	database, logger, ctx := newTestDB(t)
	seedBooking(t, database)
	deps, _, _ := testDeps(t, database, logger)
	now := time.Date(2030, 5, 30, 0, 0, 0, 0, time.UTC)
	stamp := time.Now().UTC().Format(time.RFC3339)

	listDeps := deps
	listDeps.DB = coverFailDB{DB: database, failQuery: "bookings"}
	if _, err := jobs.RunReminder(ctx, listDeps, now); err == nil ||
		!strings.Contains(err.Error(), "list bookings") {
		t.Fatalf("list error = %v, want list bookings", err)
	}

	if _, err := database.Exec(ctx,
		`INSERT INTO bookings (id, space_id, guest_id, start_date, end_date, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"booking-bad-date", "space-1", "guest-1", "not-a-date", "not-a-date", "confirmed", stamp); err != nil {
		t.Fatalf("seed bad date: %v", err)
	}
	if _, err := database.Exec(ctx,
		`INSERT INTO bookings (id, space_id, guest_id, start_date, end_date, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"booking-rfc", "space-1", "guest-1", "2030-06-02T00:00:00Z", "2030-06-04", "confirmed", stamp); err != nil {
		t.Fatalf("seed rfc date: %v", err)
	}
	n, err := jobs.RunReminder(ctx, deps, now)
	if err != nil {
		t.Fatalf("RunReminder: %v", err)
	}
	if n != 2 {
		t.Fatalf("RunReminder = %d, want 2 (bad date skipped)", n)
	}

	notifyDeps := deps
	notifyDeps.Notifier = stubCoverNotifier{err: errJobsCover}
	if _, err := jobs.RunReminder(ctx, notifyDeps, now); err == nil ||
		!strings.Contains(err.Error(), "notify guest") {
		t.Fatalf("notify error = %v, want notify guest", err)
	}

	if _, err := database.Exec(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if _, err := database.Exec(ctx, `UPDATE bookings SET guest_id = ?`, "ghost"); err != nil {
		t.Fatalf("orphan bookings: %v", err)
	}
	if _, err := jobs.RunReminder(ctx, deps, now); !errors.Is(err, jobs.ErrGuestNotFound) {
		t.Fatalf("missing guest = %v, want ErrGuestNotFound", err)
	}
	if err := jobs.ReminderHandler(listDeps)(ctx, jobs.SendReminderArgs{}); err == nil {
		t.Fatal("reminder handler list error: want error")
	}
}

// TestRegisterDemoTargetsEdge pins the empty-target skip and the register
// error branch.
func TestRegisterDemoTargetsEdge(t *testing.T) {
	database, logger, ctx := newTestDB(t)
	deps, _, _ := testDeps(t, database, logger)

	if err := jobs.RegisterDemoTargets(ctx, deps.Webhook, func(string) string { return "" }); err != nil {
		t.Fatalf("empty targets: %v", err)
	}
	if err := jobs.RegisterDemoTargets(ctx, stubCoverWebhook{registerErr: errJobsCover},
		func(name string) string {
			if strings.HasSuffix(name, "TARGET") {
				return "https://example.com/hook"
			}
			return "secret"
		}); !errors.Is(err, jobs.ErrWebhookRegister) {
		t.Fatalf("register error = %v, want ErrWebhookRegister", err)
	}
}
