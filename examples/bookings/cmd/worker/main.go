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
// The container is wired here directly (config.Load + explicit adapter
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

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/jobs"

	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	permissionrbac "github.com/zenta-dev/zever/adapters/permission/rbac"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	searchdb "github.com/zenta-dev/zever/adapters/search/db"
	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	vectorstoredb "github.com/zenta-dev/zever/adapters/vectorstore/db"
	webhookqueue "github.com/zenta-dev/zever/adapters/webhook/queue"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
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
		return fmt.Errorf("worker: load config: %w", err)
	}

	analyticslog.Register()
	authjwt.Register()
	billingstub.Register()
	cachememory.Register()
	cryptolocal.Register()
	dbsqlite.Register()
	documentlocal.Register()
	eventbusmemory.Register()
	flagstatic.Register()
	geostatic.Register()
	i18nembed.Register()
	idempotencymemory.Register()
	lockmemory.Register()
	logslog.Register()
	mailerlog.Register()
	medialocal.Register()
	notificationlog.Register()
	observabilitystdout.Register()
	passwordargon2.Register()
	paymentstub.Register()
	permissionrbac.Register()
	queuememory.Register()
	ratelimitmemory.Register()
	routerstdhttp.Register()
	schedulerembedded.Register()
	searchdb.Register()
	secretsenv.Register()
	sessionmemory.Register()
	storagelocal.Register()
	tenantsingle.Register()
	vectorstoredb.Register()
	webhookqueue.Register()
	workflowmemory.Register()

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

	// The outbox relay drains recorded events onto the configured transport
	// (queue by default). Safe to run in every worker replica: claims are
	// atomic (FOR UPDATE SKIP LOCKED), so each row publishes once and
	// consumers stay idempotent.
	if _, err := c.OutboxRelay(ctx); err != nil {
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
