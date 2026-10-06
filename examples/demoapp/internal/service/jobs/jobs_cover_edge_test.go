package jobs_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/examples/demoapp/internal/service/jobs"
)

// errJobCover is the generic backend failure job stubs report.
var errJobCover = errors.New("cover: backend failure")

// stubMailer fails Send on demand.
type stubMailer struct{ err error }

func (s stubMailer) Send(context.Context, *mailer.Mail) error { return s.err }
func (stubMailer) Close() error                               { return nil }

// stubNotifier fails Notify on demand.
type stubNotifier struct{ err error }

func (s stubNotifier) Notify(context.Context, *notification.Notification) error { return s.err }
func (stubNotifier) Close() error                                               { return nil }

// failDB delegates to a real database but fails queries/execs whose SQL
// contains the configured substring, pinning job database error branches.
type failDB struct {
	db.DB
	failQuery string
	failExec  string
}

func (f failDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	if f.failQuery != "" && strings.Contains(strings.ToLower(query), f.failQuery) {
		return nil, errJobCover
	}
	return f.DB.Query(ctx, query, args...)
}

func (f failDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if f.failExec != "" && strings.Contains(strings.ToLower(query), f.failExec) {
		return 0, errJobCover
	}
	return f.DB.Exec(ctx, query, args...)
}

// TestJobsBackendErrors pins the mail/notify/database failure branches of
// every job runner.
func TestJobsBackendErrors(t *testing.T) {
	ctx, deps, _, _ := newDeps(t)

	mailDeps := deps
	mailDeps.Mailer = stubMailer{err: errJobCover}
	if err := jobs.RunWelcomeEmail(ctx, mailDeps, "ann@example.com", "Ann"); err == nil ||
		!strings.Contains(err.Error(), "jobs: welcome: send mail") {
		t.Fatalf("mail error = %v, want send mail", err)
	}

	notifyDeps := deps
	notifyDeps.Notifier = stubNotifier{err: errJobCover}
	if err := jobs.RunWelcomeEmail(ctx, notifyDeps, "ann@example.com", "Ann"); err == nil ||
		!strings.Contains(err.Error(), "jobs: welcome: notify") {
		t.Fatalf("notify error = %v, want notify", err)
	}

	seedOrder(t, deps.DB, "cover-1")
	orderDeps := deps
	orderDeps.DB = failDB{DB: deps.DB, failExec: "orders"}
	if err := jobs.RunProcessOrder(ctx, orderDeps, "cover-1"); err == nil ||
		!strings.Contains(err.Error(), "jobs: process: mark paid") {
		t.Fatalf("process db error = %v, want mark paid", err)
	}

	reindexDeps := deps
	reindexDeps.DB = failDB{DB: deps.DB, failQuery: "products"}
	if err := jobs.RunReindex(ctx, reindexDeps); err == nil ||
		!strings.Contains(err.Error(), "jobs: reindex: list products") {
		t.Fatalf("reindex db error = %v, want list products", err)
	}

	reportDeps := deps
	reportDeps.DB = failDB{DB: deps.DB, failQuery: "orders"}
	if err := jobs.RunDailyReport(ctx, reportDeps); err == nil ||
		!strings.Contains(err.Error(), "jobs: report: list orders") {
		t.Fatalf("report db error = %v, want list orders", err)
	}
}
