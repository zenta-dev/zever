package jobs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/notification"
	gen "github.com/zenta-dev/zever/examples/demoapp/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/examples/demoapp/internal/service/jobs"
	"github.com/zenta-dev/zever/orm"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
)

// newDeps opens a migrated sqlite database and returns job dependencies with
// captured mail and notification output.
func newDeps(t *testing.T) (context.Context, jobs.Deps, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	dbsqlite.Register()

	ctx := t.Context()
	conn, err := db.Open(db.SQLite, db.Options{Path: filepath.Join(t.TempDir(), "jobs.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, execErr := conn.Exec(ctx, stmt); execErr != nil {
			t.Fatalf("migrate: %v", execErr)
		}
	}

	var mailBuf, notifyBuf bytes.Buffer

	m, err := mailerlog.NewWithWriter(mailer.Options{Host: "localhost", Port: 25}, &mailBuf)
	if err != nil {
		t.Fatalf("mailer: %v", err)
	}

	n, err := notificationlog.NewWithWriter(notification.Options{}, &notifyBuf)
	if err != nil {
		t.Fatalf("notifier: %v", err)
	}

	deps := jobs.Deps{
		DB:       conn,
		Mailer:   m,
		Notifier: n,
		Logger:   logslog.NewWithWriter(log.Options{}, io.Discard),
	}

	return ctx, deps, &mailBuf, &notifyBuf
}

// seedOrder inserts one pending order owned by a fresh user.
func seedOrder(t *testing.T, conn db.DB, id string) {
	t.Helper()

	ctx := t.Context()
	const stamp = "2026-01-02T03:04:05Z"

	if _, err := conn.Exec(ctx,
		`INSERT INTO users (id, email, name, role, password_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"u-"+id, id+"@example.com", "U", "member", "hash", stamp); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if _, err := conn.Exec(ctx,
		`INSERT INTO orders (id, user_id, total_cents, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, "u-"+id, 2500, "pending", stamp); err != nil {
		t.Fatalf("seed order: %v", err)
	}
}

// TestRunWelcomeEmail covers the happy path and the empty-email guard.
func TestRunWelcomeEmail(t *testing.T) {
	t.Parallel()

	ctx, deps, mailBuf, notifyBuf := newDeps(t)

	if err := jobs.RunWelcomeEmail(ctx, deps, "ada@example.com", "Ada"); err != nil {
		t.Fatalf("RunWelcomeEmail: %v", err)
	}

	if !strings.Contains(mailBuf.String(), "ada@example.com") {
		t.Fatalf("mail output missing recipient: %q", mailBuf.String())
	}

	if !strings.Contains(notifyBuf.String(), "ada@example.com") {
		t.Fatalf("notify output missing recipient: %q", notifyBuf.String())
	}

	if err := jobs.RunWelcomeEmail(ctx, deps, "", "Ada"); err == nil {
		t.Fatal("RunWelcomeEmail empty email: want error")
	}
}

// TestHandleSendWelcomeEmail proves the registered handler decodes the
// payload and delegates to RunWelcomeEmail.
func TestHandleSendWelcomeEmail(t *testing.T) {
	t.Parallel()

	ctx, deps, _, _ := newDeps(t)
	handle := jobs.HandleSendWelcomeEmail(deps)

	if err := handle(ctx, jobs.SendWelcomeEmailArgs{Email: "ada@example.com", Name: "Ada"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if err := handle(ctx, jobs.SendWelcomeEmailArgs{}); err == nil {
		t.Fatal("handler empty email: want error")
	}
}

// TestRunProcessOrder marks a pending order paid and pins the not-found and
// empty-id guards.
func TestRunProcessOrder(t *testing.T) {
	t.Parallel()

	ctx, deps, _, _ := newDeps(t)
	seedOrder(t, deps.DB, "o-1")

	if err := jobs.RunProcessOrder(ctx, deps, "o-1"); err != nil {
		t.Fatalf("RunProcessOrder: %v", err)
	}

	order, ok, err := orm.From(gen.Orders).Where(gen.OrderCols.ID.Eq("o-1")).First(ctx, deps.DB)
	if err != nil || !ok {
		t.Fatalf("lookup: %v ok=%v", err, ok)
	}

	if order.Status != "paid" {
		t.Fatalf("status = %q, want paid", order.Status)
	}

	if err := jobs.RunProcessOrder(ctx, deps, ""); err == nil {
		t.Fatal("empty order id: want error")
	}

	if err := jobs.RunProcessOrder(ctx, deps, "missing"); !errors.Is(err, jobs.ErrOrderNotFound) {
		t.Fatalf("missing order = %v, want ErrOrderNotFound", err)
	}
}

// TestHandleProcessOrder proves the handler delegates to RunProcessOrder.
func TestHandleProcessOrder(t *testing.T) {
	t.Parallel()

	ctx, deps, _, _ := newDeps(t)
	seedOrder(t, deps.DB, "o-2")
	handle := jobs.HandleProcessOrder(deps)

	if err := handle(ctx, jobs.ProcessOrderArgs{OrderID: "o-2"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if err := handle(ctx, jobs.ProcessOrderArgs{OrderID: "missing"}); !errors.Is(err, jobs.ErrOrderNotFound) {
		t.Fatalf("handler missing = %v, want ErrOrderNotFound", err)
	}
}

// TestScheduledJobHandlers covers the two no-argument handlers that wrap the
// scheduled reindex and daily-report jobs.
func TestScheduledJobHandlers(t *testing.T) {
	t.Parallel()

	ctx, deps, _, _ := newDeps(t)

	if err := jobs.HandleReindexSearch(deps)(ctx, struct{}{}); err != nil {
		t.Fatalf("HandleReindexSearch: %v", err)
	}

	if err := jobs.HandleGenerateDailyReport(deps)(ctx, struct{}{}); err != nil {
		t.Fatalf("HandleGenerateDailyReport: %v", err)
	}
}

// BenchmarkRunProcessOrder measures the single-row status update.
func BenchmarkRunProcessOrder(b *testing.B) {
	dbsqlite.Register()

	ctx := b.Context()
	conn, err := db.Open(db.SQLite, db.Options{Path: filepath.Join(b.TempDir(), "bench.db")})
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close(b.Context()) })

	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
	if err != nil {
		b.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, execErr := conn.Exec(ctx, stmt); execErr != nil {
			b.Fatalf("migrate: %v", execErr)
		}
	}

	const stamp = "2026-01-02T03:04:05Z"
	if _, err := conn.Exec(ctx,
		`INSERT INTO users (id, email, name, role, password_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"u-1", "u@example.com", "U", "member", "hash", stamp); err != nil {
		b.Fatalf("seed user: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO orders (id, user_id, total_cents, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		"o-1", "u-1", 2500, "pending", stamp); err != nil {
		b.Fatalf("seed order: %v", err)
	}

	deps := jobs.Deps{DB: conn, Logger: logslog.NewWithWriter(log.Options{}, io.Discard)}

	b.ReportAllocs()

	for b.Loop() {
		if err := jobs.RunProcessOrder(ctx, deps, "o-1"); err != nil {
			b.Fatalf("RunProcessOrder: %v", err)
		}
	}
}
