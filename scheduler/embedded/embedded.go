package embedded

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/log/noop"
	"github.com/zenta-dev/zever/scheduler"
)

// embedded is the in-process cron Scheduler implementation.
type embedded struct {
	mu           sync.Mutex
	sched        *job.Scheduler
	closeTimeout time.Duration
	logger       log.Logger
	cancel       context.CancelFunc
	runDone      chan struct{}
	started      bool
}

// New returns a scheduler.Scheduler dispatching through opts.Dispatcher with
// dedup via opts.Locker. The scheduler is idle until Start begins it.
func New(opts scheduler.Options) (scheduler.Scheduler, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("embedded: %w", err)
	}

	logger := opts.Logger
	if logger == nil {
		logger = noop.New()
	}

	closeTimeout := opts.CloseTimeout
	if closeTimeout == 0 {
		closeTimeout = scheduler.DefaultCloseTimeout
	}

	sched := job.NewScheduler(opts.Dispatcher, opts.Locker)
	sched.Logger = logger

	return &embedded{
		sched:        sched,
		closeTimeout: closeTimeout,
		logger:       logger,
	}, nil
}

// Schedule validates spec, jobName, and args, then registers the schedule.
// Unknown job names wrap job.ErrUnknownJob.
func (e *embedded) Schedule(ctx context.Context, spec, jobName string, args any) (scheduler.EntryID, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("scheduler: schedule %q: %w", spec, err)
	}

	if len(spec) == 0 || len(spec) > scheduler.MaxSpecLen {
		return 0, &scheduler.InvalidSpecError{Spec: spec, Err: errors.New("spec length must be 1-256")}
	}

	parsed, err := cron.ParseStandard(spec)
	if err != nil {
		return 0, &scheduler.InvalidSpecError{Spec: spec, Err: err}
	}

	if _, ok := job.Lookup(jobName); !ok {
		return 0, fmt.Errorf("scheduler: unknown job %q: %w", jobName, job.ErrUnknownJob)
	}

	if _, err := json.Marshal(args); err != nil {
		return 0, fmt.Errorf("scheduler: args: %w", err)
	}

	// parsed above is passed through so job reuses it without re-parsing.
	id, err := e.sched.EveryWithSchedule(spec, jobName, args, parsed)
	if err != nil {
		return 0, fmt.Errorf("scheduler: schedule %q: %w", spec, err)
	}

	return id, nil
}

// Remove unregisters the schedule with the given ID.
// Zero is invalid and fails; an unknown ID is a no-op returning nil.
func (e *embedded) Remove(id scheduler.EntryID) error {
	if id == 0 {
		return &scheduler.InvalidOptionsError{Reason: "invalid entry id"}
	}

	e.sched.Remove(id)

	return nil
}

// Entries returns a snapshot of the live entry IDs.
func (e *embedded) Entries() []scheduler.EntryID {
	return e.sched.Entries()
}

// Start begins cron ticks. It is idempotent and non-blocking.
func (e *embedded) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.started {
		return nil
	}

	ctx, cancel := context.WithCancel(context.WithoutCancel(context.Background()))
	e.cancel = cancel
	e.runDone = make(chan struct{})

	go func() {
		defer close(e.runDone)
		_ = e.sched.Run(ctx)
	}()

	e.started = true

	return nil
}

// Stop ends cron ticks, waiting for running ticks up to CloseTimeout.
// Stopping a scheduler that was never started is a no-op returning nil.
func (e *embedded) Stop() error {
	e.mu.Lock()

	if !e.started {
		e.mu.Unlock()

		return nil
	}

	e.cancel()
	done := e.runDone
	timeout := e.closeTimeout
	e.mu.Unlock()

	select {
	case <-done:
		e.mu.Lock()
		e.started = false
		e.mu.Unlock()

		return nil
	case <-time.After(timeout):
		return fmt.Errorf("scheduler: stop timeout: %w", context.DeadlineExceeded)
	}
}

// Name returns the adapter name for the scheduler.
func (e *embedded) Name() string { return "embedded" }
