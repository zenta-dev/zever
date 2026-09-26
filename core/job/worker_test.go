package job

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/adapters/queue/memory"
	"github.com/zenta-dev/zever/core/queue"
)

// eventually polls cond until true or timeout, failing the test on expiry.
func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("eventually timed out after %v: %s", timeout, msg)
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for %s", msg)
		case <-timer.C:
		}
	}
}

// workerStubQueue implements queue.Queue with injectable funcs.
type workerStubQueue struct {
	pushFn        func(context.Context, string, queue.Payload, queue.Headers) error
	pushDelayedFn func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error
	popFn         func(context.Context, string) (queue.Message, error)
	ackFn         func(context.Context, queue.Message) error
	nackFn        func(context.Context, queue.Message, bool) error
	lengthFn      func(context.Context, string) (int64, error)
	isEmptyFn     func(context.Context, string) (bool, error)
	closeFn       func() error
	nameFn        func() string
}

func (s *workerStubQueue) Push(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	if s.pushFn != nil {
		return s.pushFn(ctx, topic, payload, headers)
	}
	return nil
}

func (s *workerStubQueue) PushDelayed(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers, delay time.Duration) error {
	if s.pushDelayedFn != nil {
		return s.pushDelayedFn(ctx, topic, payload, headers, delay)
	}
	return nil
}

func (s *workerStubQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	if s.popFn != nil {
		return s.popFn(ctx, topic)
	}
	return queue.Message{}, queue.ErrEmpty
}

func (s *workerStubQueue) Ack(ctx context.Context, msg queue.Message) error {
	if s.ackFn != nil {
		return s.ackFn(ctx, msg)
	}
	return nil
}

func (s *workerStubQueue) Nack(ctx context.Context, msg queue.Message, requeue bool) error {
	if s.nackFn != nil {
		return s.nackFn(ctx, msg, requeue)
	}
	return nil
}

func (s *workerStubQueue) Length(ctx context.Context, topic string) (int64, error) {
	if s.lengthFn != nil {
		return s.lengthFn(ctx, topic)
	}
	return 0, nil
}

func (s *workerStubQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	if s.isEmptyFn != nil {
		return s.isEmptyFn(ctx, topic)
	}
	return true, nil
}

func (s *workerStubQueue) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

func (s *workerStubQueue) Name() string {
	if s.nameFn != nil {
		return s.nameFn()
	}
	return "stub"
}

var _ queue.Queue = (*workerStubQueue)(nil)

// fakeBatchStore implements BatchStore with counters and error injection.
type fakeBatchStore struct {
	mu sync.Mutex

	createCalls          int
	incrementCompleted   int
	incrementFailed      int
	incrementFailedBy    int
	getCalls             int
	lastIncrementBatchID string

	createErr             error
	incrementCompletedErr error
	incrementFailedErr    error
	incrementFailedByErr  error
	getErr                error
	getCompleted          int
	getFailed             int
	getTotal              int
}

func (f *fakeBatchStore) Create(_ context.Context, id string, total int) error { //nolint:revive // interface requires named params
	_ = id
	_ = total
	f.mu.Lock()
	f.createCalls++
	f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	return nil
}

func (f *fakeBatchStore) IncrementCompleted(_ context.Context, id string) (int, int, error) {
	f.mu.Lock()
	f.incrementCompleted++
	f.lastIncrementBatchID = id
	f.mu.Unlock()
	if f.incrementCompletedErr != nil {
		return 0, 0, f.incrementCompletedErr
	}
	return f.getCompleted, f.getTotal, nil
}

func (f *fakeBatchStore) IncrementFailed(_ context.Context, id string) error {
	f.mu.Lock()
	f.incrementFailed++
	f.lastIncrementBatchID = id
	f.mu.Unlock()
	if f.incrementFailedErr != nil {
		return f.incrementFailedErr
	}
	return nil
}

func (f *fakeBatchStore) IncrementFailedBy(_ context.Context, id string, n int) error { //nolint:revive
	_ = n
	f.mu.Lock()
	f.incrementFailedBy++
	f.lastIncrementBatchID = id
	f.mu.Unlock()
	if f.incrementFailedByErr != nil {
		return f.incrementFailedByErr
	}
	return nil
}

func (f *fakeBatchStore) Get(_ context.Context, id string) (int, int, int, error) { //nolint:revive
	_ = id
	f.mu.Lock()
	f.getCalls++
	f.mu.Unlock()
	if f.getErr != nil {
		return 0, 0, 0, f.getErr
	}
	return f.getCompleted, f.getFailed, f.getTotal, nil
}

func TestWorkerDrainTimeout(t *testing.T) {
	Reset()
	w := &Worker{DrainTimeout: 0}
	if got := w.drainTimeout(); got != 30*time.Second {
		t.Fatalf("drainTimeout 0 = %v want 30s", got)
	}
	w.DrainTimeout = 5 * time.Second
	if got := w.drainTimeout(); got != 5*time.Second {
		t.Fatalf("drainTimeout 5s = %v want 5s", got)
	}
	w.DrainTimeout = 20 * time.Millisecond
	if got := w.drainTimeout(); got != 20*time.Millisecond {
		t.Fatalf("drainTimeout 20ms = %v want 20ms", got)
	}
}

func TestWorkerWaitDrainDoneBranch(t *testing.T) {
	Reset()
	w := &Worker{DrainTimeout: 50 * time.Millisecond}
	var wg sync.WaitGroup
	start := time.Now()
	w.waitDrain(&wg)
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("waitDrain empty WaitGroup took %v want <20ms", elapsed)
	}
}

func TestWorkerWaitDrainTimerBranch(t *testing.T) {
	Reset()
	w := &Worker{DrainTimeout: 20 * time.Millisecond}
	var wg sync.WaitGroup
	wg.Add(1)
	start := time.Now()
	done := make(chan struct{})
	go func() {
		w.waitDrain(&wg)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("waitDrain timer branch did not return within 200ms")
	}
	elapsed := time.Since(start)
	if elapsed < 15*time.Millisecond {
		t.Fatalf("waitDrain timer branch elapsed %v want >=15ms", elapsed)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("waitDrain timer branch elapsed %v want <100ms", elapsed)
	}
	// cleanup: let wg be GC'd; keep Add without Done is intentional for test
}

func TestWorkerEffectiveAttempt(t *testing.T) {
	Reset()
	cases := []struct {
		name string
		msg  queue.Message
		want int
	}{
		{"header numeric 3", queue.Message{Headers: queue.Headers{headerAttempt: "3"}, Attempt: 1}, 3},
		{"header non-numeric fallback", queue.Message{Headers: queue.Headers{headerAttempt: "x"}, Attempt: 7}, 7},
		{"nil headers fallback", queue.Message{Headers: nil, Attempt: 5}, 5},
		{"empty header map fallback", queue.Message{Headers: queue.Headers{}, Attempt: 9}, 9},
		{"empty header value fallback", queue.Message{Headers: queue.Headers{headerAttempt: ""}, Attempt: 4}, 4},
		{"header 0", queue.Message{Headers: queue.Headers{headerAttempt: "0"}, Attempt: 2}, 0},
	}
	for _, c := range cases {
		got := effectiveAttempt(c.msg)
		if got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestWorkerLog(t *testing.T) {
	Reset()
	w := &Worker{Logger: nil}
	if got := w.log(); got == nil {
		t.Fatal("log nil Logger = nil want noop")
	}
	if got := w.log(); got.Name() != "noop" {
		t.Fatalf("log nil Logger Name=%q want noop", got.Name())
	}
	n := noop.New()
	w2 := &Worker{Logger: n}
	if got := w2.log(); got != n {
		t.Fatal("log with Logger did not return same instance")
	}
}

// oldNextPollWait is the original mutated-duration doubling implementation
// kept here only so TestWorkerNextPollWait can prove the new attempt-based
// nextPollWait produces byte-identical values across the natural sequence a
// consecutive run of empty polls would generate.
func oldNextPollWait(cur, maxPollWait time.Duration) time.Duration {
	if cur == 0 {
		return time.Millisecond
	}

	if cur >= maxPollWait {
		return maxPollWait
	}

	next := cur * 2
	if next > maxPollWait || next < cur {
		return maxPollWait
	}

	return next
}

func TestWorkerNextPollWait(t *testing.T) {
	Reset()
	const maxPollWait = 5 * time.Second

	// Walk both implementations through the exact sequence a run of
	// consecutive empty polls produces: old starts at cur=0 and repeatedly
	// feeds its own output back in; new starts at attempt=1 (the first
	// non-zero wait; attempt=0/immediate-poll is handled directly by
	// handleEmpty, not by this function) and increments. old's step N
	// output (1-based, i.e. oldNextPollWait(0) is step 1) must equal new's
	// nextPollWait(N, maxPollWait).
	cur := time.Duration(0)
	for attempt := 1; attempt <= 20; attempt++ {
		oldWant := oldNextPollWait(cur, maxPollWait)
		got := nextPollWait(attempt, maxPollWait)
		if got != oldWant {
			t.Errorf("step %d: nextPollWait(%d,%v)=%v want %v (old sequence value)", attempt, attempt, maxPollWait, got, oldWant)
		}
		cur = oldWant
	}

	// Explicit pinned checks for the load-bearing transitions, matching the
	// original table (translated from cur-based to attempt-based inputs):
	// attempt 1 is the "cur 0 -> 1ms" first-step special case, and later
	// attempts must hit and stay at the maxPollWait cap.
	cases := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{"attempt 1 -> 1ms (0 -> 1ms first-step case)", 1, time.Millisecond},
		{"attempt 2 -> 2ms", 2, 2 * time.Millisecond},
		{"attempt 3 -> 4ms", 3, 4 * time.Millisecond},
		{"attempt reaching cap -> maxPollWait", 14, maxPollWait},
		{"attempt well past cap -> maxPollWait", 62, maxPollWait},
	}
	for _, c := range cases {
		got := nextPollWait(c.attempt, maxPollWait)
		if got != c.want {
			t.Errorf("%s: nextPollWait(%v,%v)=%v want %v", c.name, c.attempt, maxPollWait, got, c.want)
		}
	}
}

func TestWorkerPopNextAvailableQueuesNil(t *testing.T) {
	Reset()
	w := &Worker{Q: &workerStubQueue{}, Queues: nil}
	_, _, err := w.popNextAvailable(t.Context())
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("popNextAvailable nil Queues err=%v want ErrEmpty", err)
	}
	w.Queues = []string{}
	_, _, err = w.popNextAvailable(t.Context())
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("popNextAvailable empty Queues err=%v want ErrEmpty", err)
	}
}

func TestWorkerPopNextAvailableCtxDone(t *testing.T) {
	Reset()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	w := &Worker{Q: &workerStubQueue{}, Queues: []string{"a", "b"}}
	_, _, err := w.popNextAvailable(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("popNextAvailable ctx canceled err=%v want Canceled", err)
	}
}

func TestWorkerPopNextAvailablePerTopic(t *testing.T) {
	Reset()
	calls := 0
	q := &workerStubQueue{
		popFn: func(_ context.Context, topic string) (queue.Message, error) {
			calls++
			if topic == "a" {
				return queue.Message{}, queue.ErrEmpty
			}
			if topic == "b" {
				return queue.Message{Topic: "b", Payload: queue.Payload([]byte("hi"))}, nil
			}
			return queue.Message{}, queue.ErrEmpty
		},
	}
	w := &Worker{Q: q, Queues: []string{"a", "b"}}
	msg, topic, err := w.popNextAvailable(t.Context())
	if err != nil {
		t.Fatalf("popNextAvailable per-topic err=%v", err)
	}
	if topic != "b" {
		t.Fatalf("topic=%q want b", topic)
	}
	if string(msg.Payload) != "hi" {
		t.Fatalf("payload=%q want hi", string(msg.Payload))
	}
	if calls != 2 {
		t.Fatalf("pop calls=%d want 2", calls)
	}
}

func TestWorkerPopNextAvailableNonEmptyError(t *testing.T) {
	Reset()
	boom := errors.New("boom")
	q := &workerStubQueue{
		popFn: func(context.Context, string) (queue.Message, error) {
			return queue.Message{}, boom
		},
	}
	w := &Worker{Q: q, Queues: []string{"t"}}
	_, _, err := w.popNextAvailable(t.Context())
	if !errors.Is(err, boom) {
		t.Fatalf("popNextAvailable non-empty err=%v want boom", err)
	}
}

func TestWorkerPopNextAvailableAllEmpty(t *testing.T) {
	Reset()
	q := &workerStubQueue{
		popFn: func(context.Context, string) (queue.Message, error) {
			return queue.Message{}, queue.ErrEmpty
		},
	}
	w := &Worker{Q: q, Queues: []string{"x", "y"}}
	_, _, err := w.popNextAvailable(t.Context())
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("all empty err=%v want ErrEmpty", err)
	}
	// also test EmptyError wrapping
	q2 := &workerStubQueue{
		popFn: func(context.Context, string) (queue.Message, error) {
			return queue.Message{}, &queue.EmptyError{Topic: "x"}
		},
	}
	w.Q = q2
	_, _, err = w.popNextAvailable(t.Context())
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("EmptyError should be ErrEmpty, got %v", err)
	}
}

func TestWorkerHandleEmpty(t *testing.T) {
	Reset()
	w := &Worker{DrainTimeout: 10 * time.Millisecond}
	var wg sync.WaitGroup
	attempt := 0
	maxPollWait := 5 * time.Second

	// attempt 0 -> wait 0, time.After(0) fast returns false and increments attempt
	ctx := t.Context()
	got := w.handleEmpty(ctx, &wg, &attempt, maxPollWait)
	if got {
		t.Fatal("handleEmpty with attempt 0 and bg ctx = true want false")
	}
	if attempt != 1 {
		t.Fatalf("attempt after handleEmpty 0 = %v want 1", attempt)
	}

	// already canceled ctx -> returns true
	// NOTE: attempt must be >0 so the wait is not immediately ready; otherwise
	// the select between time.After(0) and ctx.Done() is racy.
	ctx2, cancel := context.WithCancel(t.Context())
	cancel()
	attempt2 := 5
	got = w.handleEmpty(ctx2, &wg, &attempt2, maxPollWait)
	if !got {
		t.Fatal("handleEmpty canceled ctx = false want true")
	}

	// capped min test: attempt far past the cap, small max, ensure the
	// capped wait is used; use canceled ctx to avoid waiting max time: should
	// still return true fast
	attempt3 := 20
	got = w.handleEmpty(ctx2, &wg, &attempt3, 2*time.Second)
	if !got {
		t.Fatal("handleEmpty capped canceled = false want true")
	}
}

func TestWorkerSweepBatches(t *testing.T) {
	Reset()
	orig := triggerJitter
	triggerJitter = func() bool { return false }
	defer func() { triggerJitter = orig }()

	before := SweepBatchCallbacksCalls()
	w := &Worker{}
	w.sweepBatches(t.Context())
	after := SweepBatchCallbacksCalls()
	if after != before+1 {
		t.Fatalf("sweepBatches calls before %d after %d want +1", before, after)
	}
}

func TestWorkerSweepLoop(t *testing.T) {
	Reset()
	orig := triggerJitter
	triggerJitter = func() bool { return false }
	defer func() { triggerJitter = orig }()

	ctx, cancel := context.WithCancel(t.Context())
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	w := &Worker{}
	before := SweepBatchCallbacksCalls()
	done := make(chan struct{})
	go func() {
		w.sweepLoop(ctx, ticker)
		close(done)
	}()
	// Poll for at least one tick to fire instead of a fixed sleep.
	eventually(t, 500*time.Millisecond, func() bool {
		return SweepBatchCallbacksCalls() > before
	}, "sweepLoop did not tick")
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sweepLoop did not return after cancel")
	}
	after := SweepBatchCallbacksCalls()
	if after <= before {
		t.Fatalf("sweepLoop did not call sweepBatches: before %d after %d", before, after)
	}
}

func TestWorkerInvokeHandler(t *testing.T) {
	Reset()
	w := &Worker{Logger: noop.New()}
	// normal success
	err := w.invokeHandler(t.Context(), func(context.Context, Payload) error { return nil }, []byte("x"), "jobA")
	if err != nil {
		t.Fatalf("invokeHandler success err=%v want nil", err)
	}
	// error return
	sentinel := errors.New("handler fail")
	err = w.invokeHandler(t.Context(), func(context.Context, Payload) error { return sentinel }, nil, "jobB")
	if !errors.Is(err, sentinel) {
		t.Fatalf("invokeHandler err=%v want sentinel", err)
	}
	// panic -> ErrHandlerPanic
	err = w.invokeHandler(t.Context(), func(context.Context, Payload) error { panic("boom") }, nil, "panicky")
	if !errors.Is(err, ErrHandlerPanic) {
		t.Fatalf("invokeHandler panic err=%v want ErrHandlerPanic", err)
	}
	if err.Error() == "" {
		t.Fatal("invokeHandler panic error empty")
	}
}

func TestWorkerWrapHandler(t *testing.T) {
	Reset()
	var order []int
	m1 := func(next Handler) Handler {
		return func(ctx context.Context, p Payload) error {
			order = append(order, 1)
			return next(ctx, p)
		}
	}
	m2 := func(next Handler) Handler {
		return func(ctx context.Context, p Payload) error {
			order = append(order, 2)
			return next(ctx, p)
		}
	}
	if err := Use(m1); err != nil {
		t.Fatalf("Use m1: %v", err)
	}
	if err := Use(m2); err != nil {
		t.Fatalf("Use m2: %v", err)
	}
	w := &Worker{}
	base := func(context.Context, Payload) error {
		order = append(order, 99)
		return nil
	}
	h := w.wrapHandler(base)
	order = nil
	if err := h(t.Context(), nil); err != nil {
		t.Fatalf("wrapHandler invoke: %v", err)
	}
	// slices.Backward means m1 outermost, then m2, then base
	// So order should be 1,2,99
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 99 {
		t.Fatalf("wrapHandler order=%v want [1 2 99]", order)
	}
}

func TestWorkerProcessUnknownJob(t *testing.T) {
	Reset()
	ackCalled := false
	payload := queue.Payload([]byte("data"))
	msg := queue.Message{
		Payload: payload,
		Headers: queue.Headers{headerJobName: "unknown-job"},
		Attempt: 1,
	}
	q := &workerStubQueue{
		ackFn: func(_ context.Context, _ queue.Message) error {
			ackCalled = true
			return nil
		},
	}
	var deadLetterCalled bool
	var dlJob string
	var dlErr error
	var dlAttempts int
	w := &Worker{
		Q:      q,
		Logger: noop.New(),
		OnDeadLetter: func(_ context.Context, jobName string, _ []byte, err error, attempts int) {
			deadLetterCalled = true
			dlJob = jobName
			dlErr = err
			dlAttempts = attempts
		},
	}
	w.process(t.Context(), msg, "low")
	if !ackCalled {
		t.Fatal("process unknown job did not ack")
	}
	if !deadLetterCalled {
		t.Fatal("process unknown job did not call OnDeadLetter")
	}
	if dlJob != "unknown-job" {
		t.Fatalf("deadLetter job=%q want unknown-job", dlJob)
	}
	if !errors.Is(dlErr, ErrUnknownJob) {
		t.Fatalf("deadLetter err=%v want ErrUnknownJob", dlErr)
	}
	if dlAttempts != 1 {
		t.Fatalf("deadLetter attempts=%d want 1", dlAttempts)
	}
}

func TestWorkerProcessUnknownJobWithBatch(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getTotal: 1, getCompleted: 0, getFailed: 0}
	// pre-populate batchCallbacks to avoid nil logger path warning
	// reportBatchFailure will call store.IncrementFailed and batchSettle; that's fine
	msg := queue.Message{
		Payload: queue.Payload([]byte("x")),
		Headers: queue.Headers{headerJobName: "nope", "batch_id": "bid123"},
		Attempt: 2,
	}
	ackCalled := false
	q := &workerStubQueue{
		ackFn: func(context.Context, queue.Message) error { ackCalled = true; return nil },
	}
	w := &Worker{Q: q, BatchStore: fb, Logger: noop.New()}
	w.process(t.Context(), msg, "low")
	if !ackCalled {
		t.Fatal("unknown batch job ack not called")
	}
	if fb.incrementFailed != 1 {
		t.Fatalf("reportBatchResult failed calls=%d want 1", fb.incrementFailed)
	}
}

func TestWorkerProcessKnownSuccess(t *testing.T) {
	Reset()
	var handlerCalled bool
	if err := Register("known-success", func(_ context.Context, _ string) error {
		handlerCalled = true
		return nil
	}); err != nil {
		t.Fatalf("Register known-success: %v", err)
	}
	payload, err := json.Marshal("hello")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	msg := queue.Message{
		Payload: queue.Payload(payload),
		Headers: queue.Headers{headerJobName: "known-success"},
		Attempt: 1,
	}
	ackCalled := false
	q := &workerStubQueue{
		ackFn: func(context.Context, queue.Message) error { ackCalled = true; return nil },
	}
	w := &Worker{Q: q, Logger: noop.New()}
	w.process(t.Context(), msg, "low")
	if !handlerCalled {
		t.Fatal("known success handler not called")
	}
	if !ackCalled {
		t.Fatal("known success ack not called")
	}
}

func TestWorkerProcessMiddlewareOrder(t *testing.T) {
	Reset()
	var order []string
	m1 := func(next Handler) Handler {
		return func(ctx context.Context, p Payload) error {
			order = append(order, "m1-before")
			err := next(ctx, p)
			order = append(order, "m1-after")
			return err
		}
	}
	m2 := func(next Handler) Handler {
		return func(ctx context.Context, p Payload) error {
			order = append(order, "m2-before")
			err := next(ctx, p)
			order = append(order, "m2-after")
			return err
		}
	}
	if err := Use(m1); err != nil {
		t.Fatalf("Use m1: %v", err)
	}
	if err := Use(m2); err != nil {
		t.Fatalf("Use m2: %v", err)
	}
	if err := Register("mw-job", func(context.Context, string) error {
		order = append(order, "handler")
		return nil
	}); err != nil {
		t.Fatalf("Register mw-job: %v", err)
	}
	payload, _ := json.Marshal("x")
	msg := queue.Message{
		Payload: queue.Payload(payload),
		Headers: queue.Headers{headerJobName: "mw-job"},
		Attempt: 1,
	}
	q := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w := &Worker{Q: q, Logger: noop.New()}
	w.process(t.Context(), msg, "low")
	want := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(want) {
		t.Fatalf("middleware order %v want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order[%d]=%q want %q full %v", i, order[i], want[i], order)
		}
	}
}

func TestWorkerProcessNilHeaders(t *testing.T) {
	Reset()
	if err := Register("nilhdr-job", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register nilhdr-job: %v", err)
	}
	payload, _ := json.Marshal("hi")
	msg := queue.Message{
		Payload: queue.Payload(payload),
		Headers: nil,
		Attempt: 1,
	}
	q := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w := &Worker{Q: q, Logger: noop.New()}
	// should not panic; jobName will be "" -> unknown job path
	w.process(t.Context(), msg, "low")
	// ack should have been called via deadletter path (unknown job "")
	// We use separate stub to verify; this just checks no panic
}

func TestWorkerProcessPanicHandler(t *testing.T) {
	Reset()
	if err := Register("panic-job", func(context.Context, string) error { panic("boom-panic") }); err != nil {
		t.Fatalf("Register panic-job: %v", err)
	}
	payload, _ := json.Marshal("x")
	msg := queue.Message{
		Payload: queue.Payload(payload),
		Headers: queue.Headers{headerJobName: "panic-job"},
		Attempt: 1,
	}
	// panic leads to retry: PushDelayed + Ack
	var pushDelayedCalled bool
	var ackCalled bool
	q := &workerStubQueue{
		pushDelayedFn: func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error {
			pushDelayedCalled = true
			return nil
		},
		ackFn: func(context.Context, queue.Message) error { ackCalled = true; return nil },
	}
	w := &Worker{Q: q, Logger: noop.New()}
	w.process(t.Context(), msg, "low")
	if !pushDelayedCalled {
		t.Fatal("panic handler should trigger retry PushDelayed")
	}
	if !ackCalled {
		t.Fatal("panic handler retry should ack")
	}
}

func TestWorkerHandleResultBranches(t *testing.T) {
	Reset()
	// nil handlerErr + non-batch -> ack
	q1 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w1 := &Worker{Q: q1, Logger: noop.New()}
	msg := queue.Message{Headers: queue.Headers{}, Payload: queue.Payload([]byte("x"))}
	def := Definition{policy: RetryPolicy{MaxAttempts: 5, BaseDelay: time.Second}}
	w1.handleResult(t.Context(), msg, "low", "job1", def, 1, nil)
	// nil + isBatch true -> handleSuccess with batch
	fb := &fakeBatchStore{getTotal: 1, getCompleted: 1, getFailed: 0}
	q2 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w2 := &Worker{Q: q2, BatchStore: fb, Logger: noop.New()}
	msg2 := queue.Message{Headers: queue.Headers{"batch_id": "bid"}, Payload: queue.Payload([]byte("x"))}
	w2.handleResult(t.Context(), msg2, "low", "job1", def, 1, nil)
	if fb.incrementCompleted != 1 {
		t.Fatalf("handleResult nil err batch incrementCompleted=%d want 1", fb.incrementCompleted)
	}
	// attempt >= MaxAttempts -> deadletter
	var dlCalled bool
	q3 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w3 := &Worker{Q: q3, Logger: noop.New(), OnDeadLetter: func(context.Context, string, []byte, error, int) { dlCalled = true }}
	sentinel := errors.New("fail")
	w3.handleResult(t.Context(), msg, "low", "job1", def, 5, sentinel)
	if !dlCalled {
		t.Fatal("handleResult attempt>=MaxAttempts should deadletter")
	}
	// else retry
	var pushDelayedCalled bool
	q4 := &workerStubQueue{
		pushDelayedFn: func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error {
			pushDelayedCalled = true
			return nil
		},
		ackFn: func(context.Context, queue.Message) error { return nil },
	}
	w4 := &Worker{Q: q4, Logger: noop.New()}
	w4.handleResult(t.Context(), msg, "low", "job1", def, 1, sentinel)
	if !pushDelayedCalled {
		t.Fatal("handleResult retry should PushDelayed")
	}
}

func TestWorkerHandleSuccess(t *testing.T) {
	Reset()
	// Ack err -> warn return (no batch reporting)
	ackErr := errors.New("ack boom")
	q1 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return ackErr }}
	fb1 := &fakeBatchStore{getTotal: 1}
	w1 := &Worker{Q: q1, BatchStore: fb1, Logger: noop.New()}
	msg := queue.Message{Headers: queue.Headers{"batch_id": "bid"}, Payload: queue.Payload([]byte("x"))}
	w1.handleSuccess(t.Context(), msg, "bid", "job1", true)
	if fb1.incrementCompleted != 0 {
		t.Fatalf("handleSuccess Ack err should not increment, got %d", fb1.incrementCompleted)
	}
	// Ack ok + isBatch true -> reportBatchResult increments
	fb2 := &fakeBatchStore{getTotal: 1, getCompleted: 1}
	q2 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w2 := &Worker{Q: q2, BatchStore: fb2, Logger: noop.New()}
	w2.handleSuccess(t.Context(), msg, "bid", "job1", true)
	if fb2.incrementCompleted != 1 {
		t.Fatalf("handleSuccess ack ok batch increment=%d want 1", fb2.incrementCompleted)
	}
	// Ack ok + non-batch -> no increment
	fb3 := &fakeBatchStore{}
	q3 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w3 := &Worker{Q: q3, BatchStore: fb3, Logger: noop.New()}
	msgNoBatch := queue.Message{Headers: queue.Headers{}, Payload: queue.Payload([]byte("x"))}
	w3.handleSuccess(t.Context(), msgNoBatch, "", "job1", false)
	if fb3.incrementCompleted != 0 {
		t.Fatalf("non-batch handleSuccess should not increment, got %d", fb3.incrementCompleted)
	}
	// Ack ok + isBatch false but batchID set without store -> also no increment (handled via isBatch false)
	w3.handleSuccess(t.Context(), msg, "bid", "job1", false)
	if fb3.incrementCompleted != 0 {
		t.Fatalf("isBatch false should not increment even with batchID")
	}
}

func TestWorkerHandleDeadLetter(t *testing.T) {
	Reset()
	// Ack err -> warn, no deadletter, no batch
	ackErr := errors.New("ack fail")
	q1 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return ackErr }}
	var dlCalled bool
	fb1 := &fakeBatchStore{}
	w1 := &Worker{Q: q1, BatchStore: fb1, Logger: noop.New(), OnDeadLetter: func(context.Context, string, []byte, error, int) { dlCalled = true }}
	msg := queue.Message{Headers: queue.Headers{"batch_id": "bid"}, Payload: queue.Payload([]byte("payload"))}
	w1.handleDeadLetter(t.Context(), msg, "bid", "jobX", errors.New("handler err"), 3, true)
	if dlCalled {
		t.Fatal("Ack err should not call OnDeadLetter")
	}
	if fb1.incrementFailed != 0 {
		t.Fatalf("Ack err should not incrementFailed, got %d", fb1.incrementFailed)
	}
	// Ack ok + isBatch true -> report
	fb2 := &fakeBatchStore{getTotal: 1}
	q2 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w2 := &Worker{Q: q2, BatchStore: fb2, Logger: noop.New()}
	w2.handleDeadLetter(t.Context(), msg, "bid", "jobX", errors.New("herr"), 2, true)
	if fb2.incrementFailed != 1 {
		t.Fatalf("deadletter batch incrementFailed=%d want 1", fb2.incrementFailed)
	}
	// Ack ok + OnDeadLetter called with correct args
	var gotJob string
	var gotPayload string
	var gotAttempts int
	var gotErr error
	q3 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w3 := &Worker{
		Q:      q3,
		Logger: noop.New(),
		OnDeadLetter: func(_ context.Context, jobName string, payload []byte, err error, attempts int) {
			gotJob = jobName
			gotPayload = string(payload)
			gotErr = err
			gotAttempts = attempts
		},
	}
	handlerErr := errors.New("handler boom")
	w3.handleDeadLetter(t.Context(), queue.Message{Payload: queue.Payload([]byte("mydata"))}, "", "jobY", handlerErr, 4, false)
	if gotJob != "jobY" || gotPayload != "mydata" || gotAttempts != 4 || !errors.Is(gotErr, handlerErr) {
		t.Fatalf("OnDeadLetter args job=%q payload=%q attempts=%d err=%v", gotJob, gotPayload, gotAttempts, gotErr)
	}
	// OnDeadLetter nil -> no panic
	q4 := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w4 := &Worker{Q: q4, Logger: noop.New(), OnDeadLetter: nil}
	w4.handleDeadLetter(t.Context(), msg, "", "jobZ", errors.New("e"), 1, false)
}

func TestWorkerHandleRetry(t *testing.T) {
	Reset()
	msg := queue.Message{
		Headers: queue.Headers{headerAttempt: "1", headerJobName: "retryJob"},
		Payload: queue.Payload([]byte("body")),
	}
	policy := RetryPolicy{MaxAttempts: 5, BaseDelay: time.Second}
	// PushDelayed err -> warn, no ack
	var ackCalled bool
	q1 := &workerStubQueue{
		pushDelayedFn: func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error {
			return errors.New("push delayed boom")
		},
		ackFn: func(context.Context, queue.Message) error { ackCalled = true; return nil },
	}
	w1 := &Worker{Q: q1, Logger: noop.New()}
	w1.handleRetry(t.Context(), msg, "low", "retryJob", 1, policy)
	if ackCalled {
		t.Fatal("PushDelayed err should not Ack")
	}
	// PushDelayed ok + Ack err -> warn
	var pushCalled bool
	q2 := &workerStubQueue{
		pushDelayedFn: func(_ context.Context, _ string, _ queue.Payload, headers queue.Headers, _ time.Duration) error {
			pushCalled = true
			if headers[headerAttempt] != "2" {
				t.Errorf("retry header attempt=%q want 2", headers[headerAttempt])
			}
			return nil
		},
		ackFn: func(context.Context, queue.Message) error { return errors.New("ack fail") },
	}
	w2 := &Worker{Q: q2, Logger: noop.New()}
	w2.handleRetry(t.Context(), msg, "low", "retryJob", 1, policy)
	if !pushCalled {
		t.Fatal("retry PushDelayed not called")
	}
	// PushDelayed ok + Ack ok
	ackOk := false
	q3 := &workerStubQueue{
		pushDelayedFn: func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error { return nil },
		ackFn:         func(context.Context, queue.Message) error { ackOk = true; return nil },
	}
	w3 := &Worker{Q: q3, Logger: noop.New()}
	w3.handleRetry(t.Context(), msg, "low", "retryJob", 1, policy)
	if !ackOk {
		t.Fatal("retry Ack not called on success")
	}
	// verify headers clone not mutating original
	origAttempt := msg.Headers[headerAttempt]
	w3.handleRetry(t.Context(), msg, "low", "retryJob", 9, policy)
	if msg.Headers[headerAttempt] != origAttempt {
		t.Fatalf("original headers mutated: got %q want %q", msg.Headers[headerAttempt], origAttempt)
	}
}

func TestWorkerRunLoopBranches(t *testing.T) {
	Reset()
	// Branch: ctx.Done at sem select -> waitDrain -> nil
	t.Run("CancelAtSemSelect", func(t *testing.T) {
		Reset()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		sem := make(chan struct{}, 1)
		sem <- struct{}{} // fill so next send blocks
		var wg sync.WaitGroup
		w := &Worker{DrainTimeout: 10 * time.Millisecond, Q: &workerStubQueue{}, Queues: []string{"t"}, Logger: noop.New()}
		err := w.runLoop(ctx, sem, &wg)
		if err != nil {
			t.Fatalf("runLoop cancel at sem err=%v want nil", err)
		}
		<-sem // drain to avoid leak for next tests
	})

	// Branch: sem acquire + ErrEmpty -> handleEmpty continue (pollWait 0 fast) then cancel via ctx
	t.Run("EmptyPopContinue", func(t *testing.T) {
		Reset()
		// First pop returns ErrEmpty, second pop returns ctx.Canceled? But handleEmpty with bg ctx will sleep 0 then return false and continue.
		// We need to let loop iterate once with empty, then cancel ctx on next sem acquire.
		// Use context that cancels after short delay, and stub that always returns ErrEmpty.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var pops atomic.Int64
		q := &workerStubQueue{
			popFn: func(context.Context, string) (queue.Message, error) {
				pops.Add(1)
				return queue.Message{}, queue.ErrEmpty
			},
		}
		w := &Worker{Q: q, Queues: []string{"t"}, DrainTimeout: 10 * time.Millisecond, Logger: noop.New()}
		sem := make(chan struct{}, 1)
		var wg sync.WaitGroup
		// run runLoop in goroutine and cancel once it has polled
		errCh := make(chan error, 1)
		go func() { errCh <- w.runLoop(ctx, sem, &wg) }()
		eventually(t, 500*time.Millisecond, func() bool {
			return pops.Load() >= 2
		}, "runLoop did not poll empty queue")
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("EmptyPopContinue err=%v want nil", err)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatal("EmptyPopContinue timed out")
		}
	})

	// Branch: pop err non-empty -> return err
	t.Run("PopNonEmptyError", func(t *testing.T) {
		Reset()
		boom := errors.New("boom")
		q := &workerStubQueue{popFn: func(context.Context, string) (queue.Message, error) { return queue.Message{}, boom }}
		w := &Worker{Q: q, Queues: []string{"t"}, DrainTimeout: 10 * time.Millisecond, Logger: noop.New()}
		sem := make(chan struct{}, 1)
		var wg sync.WaitGroup
		err := w.runLoop(t.Context(), sem, &wg)
		if !errors.Is(err, boom) {
			t.Fatalf("PopNonEmptyError err=%v want boom", err)
		}
	})

	// Branch: pop err context.Canceled -> return nil
	t.Run("PopCanceled", func(t *testing.T) {
		Reset()
		q := &workerStubQueue{popFn: func(context.Context, string) (queue.Message, error) { return queue.Message{}, context.Canceled }}
		w := &Worker{Q: q, Queues: []string{"t"}, DrainTimeout: 10 * time.Millisecond, Logger: noop.New()}
		sem := make(chan struct{}, 1)
		var wg sync.WaitGroup
		err := w.runLoop(t.Context(), sem, &wg)
		if err != nil {
			t.Fatalf("PopCanceled err=%v want nil", err)
		}
	})

	// Branch: pop err non-canceled but ctx.Err()!=nil -> return nil
	t.Run("PopErrorWithCtxErr", func(t *testing.T) {
		Reset()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		boom := errors.New("other")
		q := &workerStubQueue{popFn: func(context.Context, string) (queue.Message, error) { return queue.Message{}, boom }}
		w := &Worker{Q: q, Queues: []string{"t"}, DrainTimeout: 10 * time.Millisecond, Logger: noop.New()}
		sem := make(chan struct{}, 1)
		var wg sync.WaitGroup
		err := w.runLoop(ctx, sem, &wg)
		if err != nil {
			t.Fatalf("PopErrorWithCtxErr err=%v want nil", err)
		}
	})

	// Branch: pop ok -> pollWait reset -> launchHandler
	t.Run("PopSuccessLaunch", func(t *testing.T) {
		Reset()
		if err := Register("runloop-success", func(context.Context, string) error { return nil }); err != nil {
			t.Fatalf("Register runloop-success: %v", err)
		}
		payload, _ := json.Marshal("hello")
		msg := queue.Message{
			Payload: queue.Payload(payload),
			Headers: queue.Headers{headerJobName: "runloop-success"},
			Attempt: 1,
			Topic:   "t",
		}
		var pops atomic.Int64
		var acks atomic.Int64
		q := &workerStubQueue{
			popFn: func(_ context.Context, _ string) (queue.Message, error) {
				if pops.Add(1) == 1 {
					return msg, nil
				}
				// second call: context canceled to exit loop while waiting for sem or handleEmpty
				// return ErrEmpty to trigger handleEmpty; context will be canceled externally
				return queue.Message{}, queue.ErrEmpty
			},
			ackFn: func(context.Context, queue.Message) error { acks.Add(1); return nil },
		}
		w := &Worker{Q: q, Queues: []string{"t"}, DrainTimeout: 20 * time.Millisecond, Logger: noop.New()}
		sem := make(chan struct{}, 2)
		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(t.Context())
		errCh := make(chan error, 1)
		go func() { errCh <- w.runLoop(ctx, sem, &wg) }()
		eventually(t, 500*time.Millisecond, func() bool {
			return acks.Load() >= 1
		}, "runLoop did not process popped message")
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("PopSuccessLaunch err=%v want nil", err)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("PopSuccessLaunch timed out")
		}
		if pops.Load() < 1 {
			t.Fatal("pop not called")
		}
	})
}

func TestWorkerHandleEmptyPollWaitUpdate(t *testing.T) {
	Reset()
	w := &Worker{DrainTimeout: 10 * time.Millisecond}
	var wg sync.WaitGroup
	// attempt 1 -> handleEmpty waits nextPollWait(1, max) = 1ms, then increments to 2
	attempt := 1
	maxPollWait := 5 * time.Second
	err := w.handleEmpty(t.Context(), &wg, &attempt, maxPollWait)
	if err {
		t.Fatal("handleEmpty bg ctx should be false")
	}
	if attempt != 2 {
		t.Fatalf("attempt after handleEmpty from 1 = %v want 2", attempt)
	}
}

func TestWorkerLaunchHandlerDrain(t *testing.T) {
	Reset()
	if err := Register("launch-job", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register launch-job: %v", err)
	}
	payload, _ := json.Marshal("x")
	msg := queue.Message{
		Payload: queue.Payload(payload),
		Headers: queue.Headers{headerJobName: "launch-job"},
		Attempt: 1,
	}
	q := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w := &Worker{Q: q, DrainTimeout: 30 * time.Millisecond, Logger: noop.New()}
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	var wg sync.WaitGroup
	w.launchHandler(t.Context(), msg, "low", sem, &wg)
	// wait for handler to finish via waitDrain
	done := make(chan struct{})
	go func() {
		w.waitDrain(&wg)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("launchHandler didn't complete")
	}
	if len(sem) != 0 {
		t.Fatalf("sem len=%d want 0 after launchHandler", len(sem))
	}
}

func TestWorkerRunWithMemoryQueue(t *testing.T) {
	Reset()
	// Register job that signals completion
	var mu sync.Mutex
	called := 0
	doneCh := make(chan struct{}, 10)
	if err := Register("worker-run-mem", func(_ context.Context, _ string) error {
		mu.Lock()
		called++
		mu.Unlock()
		select {
		case doneCh <- struct{}{}:
		default:
		}
		return nil
	}); err != nil {
		t.Fatalf("Register worker-run-mem: %v", err)
	}
	q, err := memory.New(queue.Options{})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	defer q.Close()
	payload, _ := json.Marshal("hello")
	headers := queue.Headers{headerJobName: "worker-run-mem"}
	if err := q.Push(t.Context(), "low", queue.Payload(payload), headers); err != nil {
		t.Fatalf("Push: %v", err)
	}
	w := &Worker{
		Q:            q,
		Queues:       []string{"low"},
		Concurrency:  0, // should default to 10
		DrainTimeout: 50 * time.Millisecond,
		Logger:       noop.New(),
	}
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()
	// wait for handler
	select {
	case <-doneCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not process message in time")
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run err=%v want nil", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after cancel")
	}
	mu.Lock()
	if called == 0 {
		mu.Unlock()
		t.Fatal("handler not called via Run")
	}
	mu.Unlock()
}

func TestWorkerRunConcurrencyExplicit(t *testing.T) {
	Reset()
	if err := Register("worker-run-explicit", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register worker-run-explicit: %v", err)
	}
	q, err := memory.New(queue.Options{})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	defer q.Close()
	w := &Worker{
		Q:            q,
		Queues:       []string{"low"},
		Concurrency:  2,
		DrainTimeout: 20 * time.Millisecond,
		Logger:       noop.New(),
	}
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()
	// Idle run has no job completion to observe; cancel directly instead of
	// a fixed sleep. Run blocks on ctx.Done so an immediate cancel is
	// deterministic and still covers start/stop.
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run explicit err=%v want nil", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Run explicit timeout")
	}
}

func TestWorkerRunPanicHandler(t *testing.T) {
	Reset()
	var ackCount atomic.Int64
	var delayedCount atomic.Int64
	// Use stub queue instead of memory to track retry PushDelayed + Ack
	// But also test panic via process that goes to handleRetry
	if err := Register("worker-panic", func(context.Context, string) error { panic("panic in run") }); err != nil {
		t.Fatalf("Register worker-panic: %v", err)
	}
	payload, _ := json.Marshal("x")
	headers := queue.Headers{headerJobName: "worker-panic"}
	// stub that returns message once then empty
	var popped atomic.Bool
	q := &workerStubQueue{
		popFn: func(ctx context.Context, _ string) (queue.Message, error) {
			select {
			case <-ctx.Done():
				return queue.Message{}, ctx.Err()
			default:
			}
			if popped.CompareAndSwap(false, true) {
				return queue.Message{
					Payload: queue.Payload(payload),
					Headers: headers,
					Attempt: 1,
					Topic:   "low",
				}, nil
			}
			return queue.Message{}, queue.ErrEmpty
		},
		ackFn: func(context.Context, queue.Message) error {
			ackCount.Add(1)
			return nil
		},
		pushDelayedFn: func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error {
			delayedCount.Add(1)
			return nil
		},
	}
	w := &Worker{
		Q:            q,
		Queues:       []string{"low"},
		Concurrency:  1,
		DrainTimeout: 30 * time.Millisecond,
		Logger:       noop.New(),
	}
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()
	eventually(t, 2*time.Second, func() bool {
		return delayedCount.Load() >= 1 && ackCount.Load() >= 1
	}, "panic handler did not retry via PushDelayed+Ack")
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run panic handler err=%v want nil", err)
		}
	case <-time.After(400 * time.Millisecond):
		t.Fatal("Run panic handler timeout")
	}
}

func TestWorkerHandleResultDeadLetterBatch(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getTotal: 1, getCompleted: 0, getFailed: 0}
	q := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	var dlJob string
	w := &Worker{
		Q:          q,
		BatchStore: fb,
		Logger:     noop.New(),
		OnDeadLetter: func(_ context.Context, jobName string, _ []byte, _ error, _ int) {
			dlJob = jobName
		},
	}
	msg := queue.Message{Headers: queue.Headers{"batch_id": "bid2"}, Payload: queue.Payload([]byte("x"))}
	def := Definition{policy: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second}}
	sentinel := errors.New("fail")
	w.handleResult(t.Context(), msg, "low", "j", def, 3, sentinel)
	if dlJob != "j" {
		t.Fatalf("deadletter job=%q want j", dlJob)
	}
	if fb.incrementFailed != 1 {
		t.Fatalf("deadletter batch incrementFailed=%d want 1", fb.incrementFailed)
	}
}

func TestWorkerServiceNameAndClose(t *testing.T) {
	Reset()
	q, err := memory.New(queue.Options{})
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	if q.Name() != "memory" {
		t.Fatalf("queue Name=%q want memory", q.Name())
	}
	if err := q.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// Ensure stubQueue Length/IsEmpty/Close/Nack/Push behave without panic.
func TestWorkerStubQueueBasics(t *testing.T) {
	Reset()
	q := &workerStubQueue{}
	if _, err := q.Length(t.Context(), "t"); err != nil {
		t.Fatalf("Length: %v", err)
	}
	if _, err := q.IsEmpty(t.Context(), "t"); err != nil {
		t.Fatalf("IsEmpty: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := q.Nack(t.Context(), queue.Message{}, true); err != nil {
		t.Fatalf("Nack: %v", err)
	}
	if err := q.Push(t.Context(), "t", nil, nil); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := q.PushDelayed(t.Context(), "t", nil, nil, time.Second); err != nil {
		t.Fatalf("PushDelayed: %v", err)
	}
	if q.Name() != "stub" {
		t.Fatalf("Name=%q want stub", q.Name())
	}
}

// Verify fmt import used via error formatting in invokeHandler panic path.
func TestWorkerInvokeHandlerErrorFormat(t *testing.T) {
	Reset()
	_ = fmt.Sprintf("fmt-used-%d", 1)
	w := &Worker{Logger: noop.New()}
	err := w.invokeHandler(t.Context(), func(context.Context, Payload) error { panic(errors.New("wrapped panic")) }, nil, "fmtJob")
	if !errors.Is(err, ErrHandlerPanic) {
		t.Fatalf("want ErrHandlerPanic, got %v", err)
	}
}

// Verify atomic usage via SweepBatchCallbacksCalls increment.
func TestWorkerSweepBatchesAtomic(t *testing.T) {
	Reset()
	orig := triggerJitter
	triggerJitter = func() bool { return false }
	defer func() { triggerJitter = orig }()
	beforeCalls := SweepBatchCallbacksCalls()
	var wg sync.WaitGroup
	var total atomic.Int64
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := &Worker{}
			w.sweepBatches(t.Context())
			total.Add(1)
		}()
	}
	wg.Wait()
	after := SweepBatchCallbacksCalls()
	if after != beforeCalls+total.Load() {
		t.Fatalf("sweepBatches atomic before %d after %d total %d", beforeCalls, after, total.Load())
	}
}
