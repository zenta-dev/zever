package job

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/zenta-dev/zever/cache"
	cachememory "github.com/zenta-dev/zever/cache/memory"
	"github.com/zenta-dev/zever/log/noop"
)

func TestSchedulerNewScheduler(t *testing.T) {
	Reset()
	d := &Dispatcher{Q: &stubQueue{}}
	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("cachememory.New: %v", err)
	}
	defer c.Close(context.Background())
	l := NewUniqueLocker(c)
	s := NewScheduler(d, l)
	if s.Dispatcher != d {
		t.Fatal("Dispatcher not set")
	}
	if s.Locker != l {
		t.Fatal("Locker not set")
	}
	if s.cron == nil {
		t.Fatal("cron is nil")
	}
	if s.now == nil {
		t.Fatal("now is nil")
	}
	if s.schedules == nil {
		t.Fatal("schedules map is nil")
	}
	if len(s.cron.Entries()) != 0 {
		t.Fatalf("Entries=%d want 0", len(s.cron.Entries()))
	}
}

func TestSchedulerEveryValid(t *testing.T) {
	Reset()
	sq := &stubQueue{}
	s := NewScheduler(&Dispatcher{Q: sq}, NewUniqueLocker(&fakeCache{}))
	if _, err := s.Every("0 * * * *", "some-job", nil); err != nil {
		t.Fatalf("Every valid: %v", err)
	}
	if got := len(s.cron.Entries()); got != 1 {
		t.Fatalf("Entries=%d want 1", got)
	}
	// Execute the registered cron closure to cover the AddFunc func body.
	// Locker default (fakeCache SetIfAbsent → false) means !acquired → return.
	s.cron.Entries()[0].Job.Run()
	if sq.pushes != 0 {
		t.Fatalf("closure pushes=%d want 0 (!acquired)", sq.pushes)
	}
}

func TestSchedulerEveryInvalidSpec(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	_, err := s.Every("not-a-spec", "some-job", nil)
	if err == nil {
		t.Fatal("Every invalid spec want error")
	}
	if !strings.Contains(err.Error(), "not-a-spec") {
		t.Fatalf("err %q missing spec", err.Error())
	}
	if got := len(s.cron.Entries()); got != 0 {
		t.Fatalf("Entries=%d want 0", got)
	}
}

func TestSchedulerEveryAddFuncError(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	// Minute-only parser rejects 5-field standard specs, while
	// cachedSchedule (ParseStandard) still accepts them.
	s.cron = cron.New(cron.WithParser(cron.NewParser(cron.Minute)))
	_, err := s.Every("0 0 * * *", "some-job", nil)
	if err == nil {
		t.Fatal("Every AddFunc reject want error")
	}
	if !strings.Contains(err.Error(), "schedule add") {
		t.Fatalf("err %q missing schedule add", err.Error())
	}
	if !strings.Contains(err.Error(), "0 0 * * *") {
		t.Fatalf("err %q missing spec", err.Error())
	}
}

func TestScheduleCachedHitAndMiss(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	schedA, err := s.cachedSchedule("0 * * * *")
	if err != nil {
		t.Fatalf("cachedSchedule miss: %v", err)
	}
	if schedA == nil {
		t.Fatal("cachedSchedule returned nil schedule")
	}
	schedB, err2 := s.cachedSchedule("0 * * * *")
	if err2 != nil {
		t.Fatalf("cachedSchedule hit: %v", err2)
	}
	if schedB == nil {
		t.Fatal("cachedSchedule hit returned nil")
	}
}

func TestScheduleCachedHitReturnsOverwritten(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	spec := "15 * * * *"
	if _, err := s.cachedSchedule(spec); err != nil {
		t.Fatalf("cachedSchedule: %v", err)
	}
	other, err := cron.ParseStandard("30 * * * *")
	if err != nil {
		t.Fatalf("ParseStandard: %v", err)
	}
	s.schedules[spec] = other
	got, err2 := s.cachedSchedule(spec)
	if err2 != nil {
		t.Fatalf("cachedSchedule hit: %v", err2)
	}
	fixed := time.Date(2026, 1, 1, 0, 7, 0, 0, time.UTC)
	want := other.Next(fixed)
	gotNext := got.Next(fixed)
	if !gotNext.Equal(want) {
		t.Fatalf("hit Next=%v want %v", gotNext, want)
	}
}

func TestScheduleCachedParseError(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	sched, err := s.cachedSchedule("bogus-spec")
	if err == nil {
		t.Fatal("cachedSchedule bogus want error")
	}
	if sched != nil {
		t.Fatalf("cachedSchedule bogus sched=%v want nil", sched)
	}
	if _, ok := s.schedules["bogus-spec"]; ok {
		t.Fatal("bogus spec should not be cached")
	}
}

func TestScheduleCachedNilMap(t *testing.T) {
	Reset()
	s := &Scheduler{}
	sched, err := s.cachedSchedule("0 0 * * *")
	if err != nil {
		t.Fatalf("cachedSchedule nil map: %v", err)
	}
	if sched == nil {
		t.Fatal("cachedSchedule nil map returned nil")
	}
	if s.schedules == nil {
		t.Fatal("schedules map not initialized")
	}
	if _, ok := s.schedules["0 0 * * *"]; !ok {
		t.Fatal("spec not inserted into initialized map")
	}
}

func TestScheduleStoreScheduleConcurrentWins(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	spec := "*/7 * * * *"
	presched, err := cron.ParseStandard(spec)
	if err != nil {
		t.Fatalf("ParseStandard: %v", err)
	}
	other, err := cron.ParseStandard("*/11 * * * *")
	if err != nil {
		t.Fatalf("ParseStandard: %v", err)
	}
	// A concurrently inserted schedule wins over the one being stored.
	s.schedules[spec] = presched
	if got := s.storeSchedule(spec, other); got != presched {
		t.Fatal("storeSchedule did not return the concurrently inserted schedule")
	}
	if s.schedules[spec] != presched {
		t.Fatal("storeSchedule overwrote the concurrently inserted schedule")
	}
}

func TestScheduleStoreScheduleInsert(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	sched, err := cron.ParseStandard("*/7 * * * *")
	if err != nil {
		t.Fatalf("ParseStandard: %v", err)
	}
	if got := s.storeSchedule("*/7 * * * *", sched); got != sched {
		t.Fatal("storeSchedule did not return the inserted schedule")
	}
	// Nil-map schedulers initialize the map on store.
	zero := &Scheduler{}
	if got := zero.storeSchedule("*/7 * * * *", sched); got != sched {
		t.Fatal("storeSchedule on zero Scheduler did not return the inserted schedule")
	}
	if zero.schedules == nil {
		t.Fatal("storeSchedule did not initialize nil map")
	}
}

func TestScheduleCachedConcurrentDoubleCheck(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	scheds := make([]cron.Schedule, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			sc, err := s.cachedSchedule("*/5 * * * *")
			scheds[idx] = sc
			errs[idx] = err
		}(i)
	}
	close(start)
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("goroutine %d err=%v", i, errs[i])
		}
		if scheds[i] == nil {
			t.Fatalf("goroutine %d nil schedule", i)
		}
	}
}

func newScheduleTestCtx(t *testing.T, q *stubQueue, fc *fakeCache, now time.Time) *Scheduler {
	t.Helper()
	Reset()
	if err := Register("sched-job", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	s := NewScheduler(&Dispatcher{Q: q}, NewUniqueLocker(fc))
	s.now = func() time.Time { return now }
	return s
}

func TestScheduleFireSuccessAndDedup(t *testing.T) {
	fixed := time.Date(2026, time.January, 1, 0, 7, 0, 0, time.UTC)
	sq := &stubQueue{}
	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("cachememory.New: %v", err)
	}
	defer c.Close(context.Background())
	s := newScheduleTestCtx(t, sq, nil, fixed)
	s.Locker = NewUniqueLocker(c)
	sched, err2 := cron.ParseStandard("0 * * * *")
	if err2 != nil {
		t.Fatalf("ParseStandard: %v", err2)
	}
	s.fireWithSchedule("sched-job", "arg", "0 * * * *", sched)
	if sq.pushes != 1 {
		t.Fatalf("pushes=%d want 1", sq.pushes)
	}
	// Same slot → same lock key → not acquired.
	s.fireWithSchedule("sched-job", "arg", "0 * * * *", sched)
	if sq.pushes != 1 {
		t.Fatalf("dedup pushes=%d want 1", sq.pushes)
	}
	// Advance past next tick → new slot → push again.
	s.now = func() time.Time { return fixed.Add(2 * time.Hour) }
	s.fireWithSchedule("sched-job", "arg", "0 * * * *", sched)
	if sq.pushes != 2 {
		t.Fatalf("new slot pushes=%d want 2", sq.pushes)
	}
}

func TestScheduleFireLockError(t *testing.T) {
	fixed := time.Date(2026, time.January, 1, 0, 7, 0, 0, time.UTC)
	sq := &stubQueue{}
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return false, errors.New("lock boom")
		},
	}
	s := newScheduleTestCtx(t, sq, fc, fixed)
	sched, err := s.cachedSchedule("0 * * * *")
	if err != nil {
		t.Fatalf("cachedSchedule: %v", err)
	}
	s.fireWithSchedule("sched-job", "arg", "0 * * * *", sched)
	if sq.pushes != 0 {
		t.Fatalf("pushes=%d want 0 on lock error", sq.pushes)
	}
}

func TestScheduleFireUnknownJob(t *testing.T) {
	Reset()
	sq := &stubQueue{}
	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("cachememory.New: %v", err)
	}
	defer c.Close(context.Background())
	s := NewScheduler(&Dispatcher{Q: sq}, NewUniqueLocker(c))
	fixed := time.Date(2026, time.January, 1, 0, 7, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }
	sched, err2 := cron.ParseStandard("0 * * * *")
	if err2 != nil {
		t.Fatalf("ParseStandard: %v", err2)
	}
	// Unknown job → Dispatch error is logged, not propagated; no push.
	s.fireWithSchedule("no-such-job", "arg", "0 * * * *", sched)
	if sq.pushes != 0 {
		t.Fatalf("pushes=%d want 0 for unknown job", sq.pushes)
	}
}

func TestSchedulerLog(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	if got := s.log(); got == nil {
		t.Fatal("log() nil with nil Logger")
	} else if got.Name() != "noop" {
		t.Fatalf("log() Name=%q want noop", got.Name())
	}
	n := noop.New()
	s.Logger = n
	if got := s.log(); got != n {
		t.Fatal("log() with Logger should return it")
	}
}

func TestSchedulerLoadCtx(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	if got := s.loadCtx(); got == nil {
		t.Fatal("loadCtx fresh want non-nil")
	}
	s.ctx.Store(42)
	if got := s.loadCtx(); got == nil {
		t.Fatal("loadCtx wrong type want non-nil Background")
	}
	type ctxKey string
	want := context.WithValue(context.Background(), ctxKey("k"), "v")
	s2 := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	s2.ctx.Store(want)
	if got := s2.loadCtx(); got != want {
		t.Fatal("loadCtx stored ctx not returned")
	}
}

func TestSchedulerRunCancel(t *testing.T) {
	Reset()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	if _, err := s.Every("@every 1s", "some-job", nil); err != nil {
		t.Fatalf("Every: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run err=%v want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestErrorStringsDuplicateJob(t *testing.T) {
	Reset()
	val := DuplicateJobError{Name: "x"}
	if got, want := val.Error(), `job: duplicate registration: "x"`; got != want {
		t.Fatalf("Error()=%q want %q", got, want)
	}
	if !errors.Is(val, ErrDuplicateJob) {
		t.Fatalf("errors.Is value %v want ErrDuplicateJob", val)
	}
	ptr := &DuplicateJobError{Name: "x"}
	if got, want := ptr.Error(), `job: duplicate registration: "x"`; got != want {
		t.Fatalf("ptr Error()=%q want %q", got, want)
	}
	if !errors.Is(ptr, ErrDuplicateJob) {
		t.Fatalf("errors.Is ptr %v want ErrDuplicateJob", ptr)
	}
}

func TestErrorStringsUnknownJob(t *testing.T) {
	Reset()
	val := UnknownJobError{Name: "nope"}
	if got, want := val.Error(), `job: unknown job: "nope"`; got != want {
		t.Fatalf("Error()=%q want %q", got, want)
	}
	if !errors.Is(val, ErrUnknownJob) {
		t.Fatalf("errors.Is value %v want ErrUnknownJob", val)
	}
	ptr := &UnknownJobError{Name: "nope"}
	if got, want := ptr.Error(), `job: unknown job: "nope"`; got != want {
		t.Fatalf("ptr Error()=%q want %q", got, want)
	}
	if !errors.Is(ptr, ErrUnknownJob) {
		t.Fatalf("errors.Is ptr %v want ErrUnknownJob", ptr)
	}
}

func TestEvery_returnsEntryID(t *testing.T) {
	t.Parallel()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	id, err := s.Every("0 * * * *", "some-job", nil)
	if err != nil {
		t.Fatalf("Every: %v", err)
	}
	if id == 0 {
		t.Fatal("Every id=0 want nonzero")
	}
	found := false
	for _, got := range s.Entries() {
		if got == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Entries %v missing id %v", s.Entries(), id)
	}
}

func TestRemove_unknownID_nilError(t *testing.T) {
	t.Parallel()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	s.Remove(EntryID(999999))
	if got := len(s.Entries()); got != 0 {
		t.Fatalf("Entries=%d want 0 after unknown Remove", got)
	}
}

func TestRemove_registeredID_removedFromEntries(t *testing.T) {
	t.Parallel()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	id1, err := s.Every("0 * * * *", "job-a", nil)
	if err != nil {
		t.Fatalf("Every job-a: %v", err)
	}
	id2, err := s.Every("30 * * * *", "job-b", nil)
	if err != nil {
		t.Fatalf("Every job-b: %v", err)
	}
	s.Remove(id1)
	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("Entries=%d want 1 after Remove", len(entries))
	}
	if entries[0] != id2 {
		t.Fatalf("Entries=%v want [%v]", entries, id2)
	}
}

func TestEntries_listsAdded(t *testing.T) {
	t.Parallel()
	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	id1, err := s.Every("0 * * * *", "job-a", nil)
	if err != nil {
		t.Fatalf("Every job-a: %v", err)
	}
	id2, err := s.Every("30 * * * *", "job-b", nil)
	if err != nil {
		t.Fatalf("Every job-b: %v", err)
	}
	entries := s.Entries()
	if len(entries) != 2 {
		t.Fatalf("Entries=%d want 2", len(entries))
	}
	seen := map[EntryID]bool{}
	for _, e := range entries {
		seen[e] = true
	}
	if !seen[id1] || !seen[id2] {
		t.Fatalf("Entries=%v missing ids %v %v", entries, id1, id2)
	}
}

func TestFireWithSchedule_nilLocker_dispatches(t *testing.T) {
	fixed := time.Date(2026, time.January, 1, 0, 7, 0, 0, time.UTC)
	sq := &stubQueue{}
	s := newScheduleTestCtx(t, sq, &fakeCache{}, fixed)
	s.Locker = nil
	sched, err := s.cachedSchedule("0 * * * *")
	if err != nil {
		t.Fatalf("cachedSchedule: %v", err)
	}
	s.fireWithSchedule("sched-job", "arg", "0 * * * *", sched)
	if sq.pushes != 1 {
		t.Fatalf("pushes=%d want 1 with nil Locker", sq.pushes)
	}
}

func TestNewScheduler_nilDispatcher_EveryFails(t *testing.T) {
	t.Parallel()
	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("cachememory.New: %v", err)
	}
	defer c.Close(context.Background())
	s := NewScheduler(nil, NewUniqueLocker(c))
	id, err := s.Every("0 * * * *", "some-job", nil)
	if err == nil {
		t.Fatal("Every with nil Dispatcher want error")
	}
	if !strings.Contains(err.Error(), "dispatcher is nil") {
		t.Fatalf("err %q missing dispatcher is nil", err.Error())
	}
	if id != 0 {
		t.Fatalf("id=%v want 0 on error", id)
	}
	if got := len(s.Entries()); got != 0 {
		t.Fatalf("Entries=%d want 0 after failed Every", got)
	}
}

func TestScheduleFireNilDispatcherWarns(t *testing.T) {
	t.Parallel()
	s := &Scheduler{}
	sched, err := cron.ParseStandard("0 * * * *")
	if err != nil {
		t.Fatalf("ParseStandard: %v", err)
	}
	// Nil Locker skips the lock; nil Dispatcher must warn and return, not panic.
	s.fireWithSchedule("sched-job", nil, "0 * * * *", sched)
}
