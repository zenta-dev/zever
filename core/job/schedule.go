package job

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/log"
)

// EntryID wraps cron.EntryID identifying a registered schedule.
// It is a uint64 scheduler handle, not a uuid BatchID.
type EntryID uint64

// ScheduleSpanScope identifies the tracer that starts per-fire schedule root spans.
const ScheduleSpanScope = "github.com/zenta-dev/zever/core/job"

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
	now    func() time.Time

	mu        sync.RWMutex
	schedules map[string]cron.Schedule

	lifeMu     sync.Mutex
	runDone    chan struct{}
	runStarted bool
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
	// Each cron fire is a trace root: start a new span named schedule.<name>
	// from Background so fires never inherit the registration caller's trace
	// (EveryWithSchedule deliberately does not take ctx) nor any Run ctx's
	// trace. No fake parent is synthesized; the SDK assigns a fresh traceID
	// per fire, and Dispatcher/queue Push propagates it downstream via
	// traceprop.Inject. Cancellation from the Run lifecycle is merged without
	// merging trace (AfterFunc), so Stop still aborts in-flight fires.
	traceCtx, span := otel.Tracer(ScheduleSpanScope).Start(context.Background(), "schedule."+jobName)
	defer span.End()
	ctx, stop := context.WithCancel(traceCtx)
	defer stop()
	stopAfter := context.AfterFunc(lifecycleCtx{s.lifecycle()}, stop)
	defer stopAfter()

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

// lifecycle returns the channel closed when the scheduler's owning Run
// returns. In-flight fires abort via AfterFunc when it closes. The channel
// is created lazily and recreated after each Run, so a restarted scheduler
// gets a fresh one. It replaces the old stored context: no request-scoped
// ctx is retained by the Scheduler, and concurrent Run calls can no longer
// overwrite each other's cancellation signal.
func (s *Scheduler) lifecycle() <-chan struct{} {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()

	if s.runDone == nil {
		s.runDone = make(chan struct{})
	}

	return s.runDone
}

// lifecycleCtx adapts the lifecycle channel to context.Context so
// context.AfterFunc can watch it. It carries no values and no deadline.
type lifecycleCtx struct {
	done <-chan struct{}
}

func (c lifecycleCtx) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c lifecycleCtx) Done() <-chan struct{}       { return c.done }

func (c lifecycleCtx) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

func (c lifecycleCtx) Value(any) any { return nil }

// Run starts cron ticks and blocks until ctx is done.
// It stops the cron scheduler cleanly before returning nil.
// The first Run call owns the scheduler lifecycle: it starts cron and, when
// its ctx ends, stops cron and closes the lifecycle channel so in-flight
// fires abort. A concurrent or later Run only waits for its own ctx; once
// the owning Run has returned, a subsequent Run may start cron again.
func (s *Scheduler) Run(ctx context.Context) error {
	s.lifeMu.Lock()
	owner := !s.runStarted
	if owner {
		s.runStarted = true
	}
	s.lifeMu.Unlock()

	if owner {
		s.cron.Start()
	}

	<-ctx.Done()

	if !owner {
		return nil
	}

	stopCtx := s.cron.Stop()
	<-stopCtx.Done()

	s.lifeMu.Lock()
	s.runStarted = false
	if s.runDone != nil {
		close(s.runDone)
		s.runDone = nil
	}
	s.lifeMu.Unlock()

	return nil
}
