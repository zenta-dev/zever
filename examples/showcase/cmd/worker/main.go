// Command worker runs the showcase background job worker and its embedded
// scheduler in one process.
//
// Scaffolded to match the `zever generate worker` shape (see the canonical
// generator output): the scheduler dispatches onto the queue and the worker
// consumes from it, so both must share one queue: with the in-process memory
// adapter that means one process. Swapping the queue adapter for redis/nats
// in zever.yaml is what makes this distributable, with no code change here.
//
// Unlike the canonical stubs (which take no deps), the handlers here close
// over jobs.Deps, which is the filled-in stub form.
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/examples/showcase/internal/app"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/jobs"
	"github.com/zenta-dev/zever/job"
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
	// internal/service/jobs. A job that is not registered in this process is
	// never executed, so keep this list in step with the schema.
	if regErr := job.Register("GenerateDailyReport", jobs.HandleGenerateDailyReport(deps)); regErr != nil {
		return regErr
	}
	if regErr := job.Register("ProcessOrder", jobs.HandleProcessOrder(deps)); regErr != nil {
		return regErr
	}
	if regErr := job.Register("ReindexSearch", jobs.HandleReindexSearch(deps)); regErr != nil {
		return regErr
	}
	if regErr := job.Register("SendConfirmation", jobs.HandleSendConfirmation(deps)); regErr != nil {
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

	// schedule DailyReport
	if _, err := sched.Schedule(ctx, "0 9 * * *", "GenerateDailyReport", json.RawMessage(`{}`)); err != nil {
		return err
	}

	// schedule HourlyReindex
	if _, err := sched.Schedule(ctx, "0 * * * *", "ReindexSearch", json.RawMessage(`{}`)); err != nil {
		return err
	}

	if err := sched.Start(); err != nil {
		return err
	}

	logger.Info().Int("jobs", 4).Int("schedules", 2).Msg("worker started")

	worker := &job.Worker{
		Q:           q,
		Queues:      []string{"default", "orders", "low"},
		Concurrency: concurrency,
		Logger:      logger,
	}

	return worker.Run(ctx)
}
