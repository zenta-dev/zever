package job

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/log"
)

// EntryID wraps cron.EntryID identifying a registered schedule.
// It is a uint64 scheduler handle, not a uuid BatchID.
type EntryID uint64

// Scheduler fires registered jobs on cron specs using Dispatcher.
// Dispatcher enqueues due jobs, Locker suppresses duplicate slots, and Logger reports schedule errors.
//
// A nil Locker means no cross-instance dedup: fireWithSchedule dispatches
// directly without acquiring a schedule-slot lock (single-instance mode).
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

// Every registers jobName with args on cron spec and returns its entry ID.
// It parses spec once via cachedSchedule and registers with cron.Schedule,
// so the cached schedule drives both firing and dedup. It returns an error
// for a nil Dispatcher or invalid specs.
func (s *Scheduler) Every(spec, jobName string, args any) (EntryID, error) {
	if s.Dispatcher == nil {
		return 0, errors.New("job: scheduler dispatcher is nil")
	}

	sched, err := s.cachedSchedule(spec)
	if err != nil {
		return 0, fmt.Errorf("job: schedule spec %q: %w", spec, err)
	}

	return s.addSchedule(spec, jobName, args, sched), nil
}

// EveryWithSchedule registers jobName with args on spec using the already
// parsed sched, caching it and registering with cron.Schedule without
// re-parsing. A concurrently cached schedule wins over sched. It returns an
// error for a nil Dispatcher or nil sched.
func (s *Scheduler) EveryWithSchedule(spec, jobName string, args any, sched cron.Schedule) (EntryID, error) {
	if s.Dispatcher == nil {
		return 0, errors.New("job: scheduler dispatcher is nil")
	}

	if sched == nil {
		return 0, fmt.Errorf("job: schedule spec %q: nil schedule", spec)
	}

	sched = s.storeSchedule(spec, sched)

	return s.addSchedule(spec, jobName, args, sched), nil
}

// addSchedule registers the firing closure on the given parsed schedule
// without parsing. Callers must have cached sched already.
func (s *Scheduler) addSchedule(spec, jobName string, args any, sched cron.Schedule) EntryID {
	id := s.cron.Schedule(sched, cron.FuncJob(func() {
		s.fireWithSchedule(jobName, args, spec, sched)
	}))

	//nolint:gosec // cron EntryIDs are a small positive sequence starting at 1.
	return EntryID(id)
}

// Remove unregisters the schedule with the given ID.
// An unknown ID is a no-op returning nothing, per cron.Cron.Remove semantics.
func (s *Scheduler) Remove(id EntryID) {
	//nolint:gosec // IDs originate from Every, a small positive cron sequence.
	s.cron.Remove(cron.EntryID(id))
}

// Entries returns a snapshot of the live cron entry IDs.
func (s *Scheduler) Entries() []EntryID {
	entries := s.cron.Entries()
	ids := make([]EntryID, 0, len(entries))
	for _, e := range entries {
		//nolint:gosec // cron EntryIDs are a small positive sequence starting at 1.
		ids = append(ids, EntryID(e.ID))
	}

	return ids
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

	// Nil Locker = no cross-instance dedup; dispatch directly in single-instance mode.
	if s.Locker != nil {
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
	}

	if s.Dispatcher == nil {
		s.log().Warn().Str("job", jobName).Msg("job: scheduler dispatcher is nil")
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
