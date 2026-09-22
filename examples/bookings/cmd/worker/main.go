// Command worker runs the bookings background job worker and its embedded
// scheduler in one process.
//
// Hand-written to match the `zever generate worker` shape (one
// job.Register per job declared in schema/bookings.zen, one scheduler entry
// per declared schedule). The scheduler dispatches onto the queue and the
// worker consumes from it, so both must share one queue: with the in-process
// memory adapter that means one process. Swapping the queue adapter for
// redis/nats in zever.yaml is what makes this distributable, with no code
// change here.
//
// The container is wired here directly (config.Load + blank adapter imports)
// so this track stays disjoint from the sibling server track that owns
// internal/app.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/jobs"
	"github.com/zenta-dev/zever/job"

	// Blank imports register the adapters selected in zever.yaml.
	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/mailer/log"
	_ "github.com/zenta-dev/zever/notification/log"
	_ "github.com/zenta-dev/zever/password/argon2"
	_ "github.com/zenta-dev/zever/queue/memory"
	_ "github.com/zenta-dev/zever/scheduler/embedded"
	_ "github.com/zenta-dev/zever/secrets/env"
	_ "github.com/zenta-dev/zever/webhook/queue"
)

const (
	shutdownGrace = 10 * time.Second
	concurrency   = 4
)

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString("worker: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("[worker] load config: %w", err)
	}
	c := container.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = c.Close(closeCtx)
	}()

	logger, err := c.Log()
	if err != nil {
		return err
	}

	database, err := c.DB()
	if err != nil {
		return err
	}

	mailerSvc, err := c.Mailer()
	if err != nil {
		return err
	}

	notifier, err := c.Notification()
	if err != nil {
		return err
	}

	hooks, err := c.Webhook()
	if err != nil {
		return err
	}

	secretsSvc, err := c.Secrets()
	if err != nil {
		return err
	}

	deps := jobs.Deps{DB: database, Mailer: mailerSvc, Notifier: notifier, Webhook: hooks, Logger: logger}

	// One handler per job declared in the schema. A job that is not
	// registered in this process is never executed, so keep this list in
	// step with the schema.
	if regErr := job.Register("SendConfirmation", jobs.ConfirmationHandler(deps)); regErr != nil {
		return regErr
	}
	if regErr := job.Register("SendReminder", jobs.ReminderHandler(deps)); regErr != nil {
		return regErr
	}

	// Demo webhook targets (booking.created, booking.cancelled). Secrets
	// resolve via the secrets service (env adapter): see jobs.EnvCreatedTarget
	// and friends. Empty targets are skipped, so the worker runs without them.
	lookup := func(name string) string {
		raw, gerr := secretsSvc.Get(ctx, name)
		if gerr != nil {
			return ""
		}
		return string(raw)
	}
	if terr := jobs.RegisterDemoTargets(ctx, hooks, lookup); terr != nil {
		return terr
	}

	q, err := c.Queue()
	if err != nil {
		return err
	}

	sched, err := c.Scheduler()
	if err != nil {
		return err
	}

	// Schedule Reminder: every day at 09:00, dispatch SendReminder.
	if _, err := sched.Schedule(ctx, "0 9 * * *", "SendReminder", json.RawMessage(`{}`)); err != nil {
		return err
	}

	if err := sched.Start(); err != nil {
		return err
	}

	logger.Info().Int("jobs", 2).Int("schedules", 1).Msg("worker started")

	worker := &job.Worker{
		Q:           q,
		Queues:      []string{"default", "low"},
		Concurrency: concurrency,
		Logger:      logger,
	}

	return worker.Run(ctx)
}
