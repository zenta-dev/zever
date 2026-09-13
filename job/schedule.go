package job

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/log/noop"
)

// Scheduler fires registered jobs on cron specs using Dispatcher.
// Dispatcher enqueues due jobs, Locker suppresses duplicate slots, and Logger reports schedule errors.
type Scheduler struct {
	// Dispatcher enqueues jobs when a cron tick fires.
	Dispatcher *Dispatcher
	// Locker deduplicates each schedule slot across scheduler instances.
	Locker *UniqueLocker
	// Logger reports locker and dispatch errors and defaults to noop.
	Logger log.Logger
	cron   *cron.Cron
	ctx    atomic.Value
	now    func() time.Time

	mu        sync.RWMutex
	schedules map[string]cron.Schedule
}

// NewScheduler returns a Scheduler dispatching through d with dedup via locker.
// The scheduler is idle until Run starts it.
func NewScheduler(d *Dispatcher, locker *UniqueLocker) *Scheduler {
	return &Scheduler{
		Dispatcher: d,
		Locker:     locker,
		cron:       cron.New(),
		now:        time.Now,
		schedules:  make(map[string]cron.Schedule),
	}
}

// Every registers jobName with args on cron spec.
// It returns an error for invalid specs or scheduler registration failures.
func (s *Scheduler) Every(spec, jobName string, args any) error {
	sched, err := s.cachedSchedule(spec)
	if err != nil {
		return fmt.Errorf("job: schedule spec %q: %w", spec, err)
	}

	_, err = s.cron.AddFunc(spec, func() {
		s.fireWithSchedule(jobName, args, spec, sched)
	})
	if err != nil {
		return fmt.Errorf("job: schedule add %q: %w", spec, err)
	}

	return nil
}

func (s *Scheduler) cachedSchedule(spec string) (cron.Schedule, error) {
	s.mu.RLock()

	if sched, ok := s.schedules[spec]; ok {
		s.mu.RUnlock()

		return sched, nil
	}

	s.mu.RUnlock()

	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return nil, err
	}

	return s.storeSchedule(spec, sched), nil
}

// storeSchedule caches sched under spec and returns the cached schedule.
// A schedule inserted concurrently wins over sched.
func (s *Scheduler) storeSchedule(spec string, sched cron.Schedule) cron.Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sched2, ok := s.schedules[spec]; ok {
		return sched2
	}

	if s.schedules == nil {
		s.schedules = make(map[string]cron.Schedule)
	}

	s.schedules[spec] = sched

	return sched
}

func (s *Scheduler) fireWithSchedule(jobName string, args any, spec string, sched cron.Schedule) {
	ctx := s.loadCtx()

	now := s.now()
	next := sched.Next(now)
	slot := next.Unix()
	interval := sched.Next(next).Sub(next)
	lockKey := ":schedule" + spec + ":" + jobName + ":" + strconv.FormatInt(slot, 10)
	ttl := interval + interval/2

	acquired, err := s.Locker.Acquire(ctx, lockKey, ttl)
	if err != nil {
		s.log().Warn().Str("job", jobName).Err(err).Msg("job: schedule locker error")
		return
	}

	if !acquired {
		return
	}

	if err := s.Dispatcher.Dispatch(ctx, jobName, args); err != nil {
		s.log().Warn().Str("job", jobName).Err(err).Msg("job: failed to dispatch scheduled job")
	}
}

func (s *Scheduler) log() log.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return noop.New()
}

func (s *Scheduler) loadCtx() context.Context {
	v := s.ctx.Load()
	if v == nil {
		return context.Background()
	}

	if ctx, ok := v.(context.Context); ok && ctx != nil {
		return ctx
	}

	return context.Background()
}

// Run starts cron ticks and blocks until ctx is done.
// It stops the cron scheduler cleanly before returning nil.
func (s *Scheduler) Run(ctx context.Context) error {
	s.ctx.Store(ctx)
	s.cron.Start()
	<-ctx.Done()

	stopCtx := s.cron.Stop()
	<-stopCtx.Done()

	return nil
}
