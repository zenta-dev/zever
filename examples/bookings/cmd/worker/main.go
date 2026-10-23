// Command worker runs the bookings background job worker and its embedded
// scheduler in one process.
//
// Hand-written to match the `zever generate worker` shape (one
// job.Register per job declared in schema/bookings.zen, one scheduler entry
// per declared schedule). The scheduler dispatches onto the queue and the
// worker consumes from it, so both must share one queue: with the in-process
// memory adapter that means one process. Swapping the queue adapter for
// redis/nats is a wiring change (workerProviders below, then rebuild);
// that is what makes this distributable.
//
// The container is wired here directly (config.Resolve + explicit adapter
// registration) so this track stays disjoint from the sibling server track
// that owns internal/app.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	outboxdb "github.com/zenta-dev/zever/adapters/outbox/db"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	webhookqueue "github.com/zenta-dev/zever/adapters/webhook/queue"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/db"
	eventbus "github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/jobs"
	"github.com/zenta-dev/zever/shared/dbconn"
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

// workerProviders wires the batteries the worker resolves. Adapters are
// compiled-in constructors, never env selections: changing one means
// editing this list and rebuilding.
func workerProviders() []container.Provider {
	return []container.Provider{
		container.Provide(log.DefaultRef(), func(o log.Options) (log.Logger, error) {
			return logslog.New(o), nil
		}),
		container.Provide(db.DefaultRef(), dbsqlite.New),
		container.Provide(mailer.DefaultRef(), mailerlog.New),
		container.Provide(notification.DefaultRef(), notificationlog.New),
		container.Provide(webhook.DefaultRef(), webhookqueue.New),
		container.Provide(secrets.DefaultRef(), func(o secrets.Options) (secrets.Secrets, error) {
			return secretsenv.New(secretsenv.Options{Prefix: o.Prefix})
		}),
		container.Provide(queue.DefaultRef(), queuememory.New),
		container.Provide(scheduler.DefaultRef(), schedulerembedded.New),
		container.Provide(eventbus.DefaultRef(), eventbusmemory.New),
		container.Provide(outbox.DefaultRef(), func(o outbox.Options) (outbox.Store, error) {
			poolOpts := dbconn.SplitDSN(o.DSN)
			poolOpts.DedicatedPool = o.DedicatedPool
			return outboxdb.New(outboxdb.Options{
				Options:      poolOpts,
				Table:        o.Table,
				InboxTable:   o.InboxTable,
				Transport:    o.Publisher,
				PollInterval: o.PollInterval,
				BatchSize:    o.BatchSize,
				MaxAttempts:  o.MaxAttempts,
				Retry:        o.Retry,
				Retention:    o.Retention,
				LockSeconds:  o.LockSeconds,
				Provider:     o.Provider,
			})
		}),
	}
}

func run() error {
	env, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("worker: load env: %w", err)
	}
	cfg, err := config.Resolve(env)
	if err != nil {
		return fmt.Errorf("worker: load config: %w", err)
	}

	c := container.New(cfg, workerProviders()...)

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

	// The outbox relay drains recorded events onto the configured transport
	// (queue by default). Safe to run in every worker replica: claims are
	// atomic (FOR UPDATE SKIP LOCKED), so each row publishes once and
	// consumers stay idempotent.
	if _, err := c.OutboxRelay(ctx); err != nil { //nolint:govet // generator-emitted relay block redeclares err; the inner err is confined to this if statement.
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
