// Command worker runs the demoapp background job worker and its embedded
// scheduler in one process.
//
// The scheduler dispatches onto the queue and the worker consumes from it,
// so both share one queue: with the in-process memory adapter that means
// one process. Swapping the queue adapter in zever.yaml makes this
// distributable with no code change here.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/examples/demoapp/internal/app"
	"github.com/zenta-dev/zever/examples/demoapp/internal/service/jobs"
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
	c, err := app.New()
	if err != nil {
		return err
	}

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

	deps := jobs.Deps{DB: database, Mailer: mailerSvc, Notifier: notifier, Logger: logger}

	// One handler per job declared in the schema, implemented in
	// internal/service/jobs. A job that is not registered in this process
	// is never executed, so keep this list in step with the schema.
	if regErr := job.Register("GenerateDailyReport", jobs.HandleGenerateDailyReport(deps)); regErr != nil {
		return regErr
	}
	if regErr := job.Register("ProcessOrder", jobs.HandleProcessOrder(deps)); regErr != nil {
		return regErr
	}
	if regErr := job.Register("ReindexSearch", jobs.HandleReindexSearch(deps)); regErr != nil {
		return regErr
	}
	if regErr := job.Register("SendWelcomeEmail", jobs.HandleSendWelcomeEmail(deps)); regErr != nil {
		return regErr
	}

	q, err := c.Queue()
	if err != nil {
		return err
	}

	sched, err := c.Scheduler()
	if err != nil {
		return err
	}

	// Schedules from the schema.
	if _, err := sched.Schedule(ctx, "0 9 * * *", "GenerateDailyReport", map[string]any{}); err != nil {
		return err
	}
	if _, err := sched.Schedule(ctx, "0 * * * *", "ReindexSearch", map[string]any{}); err != nil {
		return err
	}

	if err := sched.Start(); err != nil {
		return err
	}

	logger.Info().Int("jobs", 4).Int("schedules", 2).Msg("demo worker started")

	worker := &job.Worker{
		Q:           q,
		Queues:      []string{"default", "orders", "low"},
		Concurrency: concurrency,
		Logger:      logger,
	}

	return worker.Run(ctx)
}
