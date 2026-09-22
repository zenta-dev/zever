package jobs_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	gen "github.com/zenta-dev/zever/examples/showcase/generated/zenorm/orm/gen/shop"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/jobs"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/seed"
	"github.com/zenta-dev/zever/orm"

	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/mailer/log"
	_ "github.com/zenta-dev/zever/notification/log"
)

// testDeps builds a seeded database and job deps per test.
func testDeps(t *testing.T) (context.Context, jobs.Deps) {
	t.Helper()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "jobs.db")
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	ctx := context.Background()
	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	mailerSvc, err := c.Mailer()
	if err != nil {
		t.Fatalf("Mailer: %v", err)
	}
	notifier, err := c.Notification()
	if err != nil {
		t.Fatalf("Notification: %v", err)
	}
	logger, err := c.Log()
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

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

	if err := seed.Run(ctx, database); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return ctx, jobs.Deps{DB: database, Mailer: mailerSvc, Notifier: notifier, Logger: logger}
}

func TestConfirmationSendsReceipt(t *testing.T) {
	ctx, deps := testDeps(t)

	if err := jobs.RunConfirmation(ctx, deps, seed.OrderID); err != nil {
		t.Fatalf("RunConfirmation: %v", err)
	}

	if err := jobs.RunConfirmation(ctx, deps, ""); err == nil {
		t.Fatalf("empty order id: want error")
	}

	if err := jobs.RunConfirmation(ctx, deps, "missing"); err == nil {
		t.Fatalf("missing order: want error")
	}
}

func TestProcessOrderMarksPaid(t *testing.T) {
	ctx, deps := testDeps(t)

	if err := jobs.RunProcessOrder(ctx, deps, seed.OrderID); err != nil {
		t.Fatalf("RunProcessOrder: %v", err)
	}

	order, ok, err := orm.From(gen.Orders).Where(gen.OrderCols.ID.Eq(seed.OrderID)).First(ctx, deps.DB)
	if err != nil || !ok {
		t.Fatalf("lookup: %v ok=%v", err, ok)
	}
	if order.Status != gen.OrderStatusPaid {
		t.Fatalf("status = %q, want paid", order.Status)
	}

	if err := jobs.RunProcessOrder(ctx, deps, "missing"); err == nil {
		t.Fatalf("missing order: want error")
	}
}

func TestScheduledJobs(t *testing.T) {
	ctx, deps := testDeps(t)

	if err := jobs.RunReindex(ctx, deps); err != nil {
		t.Fatalf("RunReindex: %v", err)
	}

	if err := jobs.RunDailyReport(ctx, deps); err != nil {
		t.Fatalf("RunDailyReport: %v", err)
	}
}
