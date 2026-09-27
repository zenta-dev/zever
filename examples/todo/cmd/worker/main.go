// Command worker runs this project's background job worker and its embedded
// scheduler in one process.
//
// Scaffolded by `zever generate worker` (hand-written to match its shape).
// The scheduler dispatches onto the queue and the worker consumes from it, so
// both must share one queue: with the in-process memory adapter that means
// one process. Swapping the queue adapter for redis/nats in zever.yaml is
// what makes this distributable, with no code change here.
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/examples/todo/internal/app"
	"github.com/zenta-dev/zever/examples/todo/internal/service/jobs"
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

	// One handler per job declared in the schema. A job that is not registered
	// in this process is never executed, so keep this list in step with the
	// schema.
	if regErr := job.Register("SendOverdueDigest", jobs.Handler(database, logger)); regErr != nil {
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

	// Schedule OverdueDigest: every minute, dispatch SendOverdueDigest.
	if _, err := sched.Schedule(ctx, "* * * * *", "SendOverdueDigest", json.RawMessage(`{}`)); err != nil {
		return err
	}

	if err := sched.Start(); err != nil {
		return err
	}

	logger.Info().Int("jobs", 1).Int("schedules", 1).Msg("worker started")

	worker := &job.Worker{
		Q:           q,
		Queues:      []string{"low"},
		Concurrency: concurrency,
		Logger:      logger,
	}

	return worker.Run(ctx)
}
