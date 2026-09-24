package job

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zenta-dev/zever/log/noop"
	"github.com/zenta-dev/zever/queue"
)

type batchFailReader struct{}

func (batchFailReader) Read(_ []byte) (int, error) {
	return 0, errors.New("rand boom")
}

func TestBatchTriggerJitterDefault(t *testing.T) {
	Reset()
	// Invoke the package-default triggerJitter closure directly so its body
	// stays covered; jitter-override tests below must not run before this.
	if triggerJitter == nil {
		t.Fatal("default triggerJitter is nil")
	}
	for range 5 {
		_ = triggerJitter()
	}
}

func TestBatchIDString(t *testing.T) {
	Reset()
	id := newBatchID()
	s := id.String()
	if len(s) != 36 {
		t.Fatalf("BatchID string len=%d want 36 (%q)", len(s), s)
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("uuid.Parse(%q): %v", s, err)
	}
	if uuid.UUID(id) != parsed {
		t.Fatalf("roundtrip mismatch %v vs %v", uuid.UUID(id), parsed)
	}
	other := newBatchID()
	if id == other {
		t.Fatal("newBatchID returned duplicate")
	}
}

func TestBatchSweepCalls(t *testing.T) {
	Reset()
	if got := SweepBatchCallbacksCalls(); got < 0 {
		t.Fatalf("SweepBatchCallbacksCalls=%d want >=0", got)
	}
}

func TestBatchNewBatchTimeouts(t *testing.T) {
	Reset()
	store := &fakeBatchStore{}
	before := time.Now()
	d := &Dispatcher{SettleTimeout: time.Minute, Q: &stubQueue{}}
	b := NewBatch(d, store)
	after := time.Now()
	if b.dispatcher != d {
		t.Fatal("NewBatch dispatcher not set")
	}
	if b.store == nil {
		t.Fatal("NewBatch store not set")
	}
	if b.deadline.Before(before.Add(time.Minute)) || b.deadline.After(after.Add(time.Minute)) {
		t.Fatalf("deadline=%v want now+1m", b.deadline)
	}
	d2 := &Dispatcher{Q: &stubQueue{}}
	b2 := NewBatch(d2, store)
	if b2.deadline.Before(before.Add(defaultSettleTimeout)) || b2.deadline.After(time.Now().Add(defaultSettleTimeout)) {
		t.Fatalf("default deadline=%v want now+5m", b2.deadline)
	}
	d3 := &Dispatcher{SettleTimeout: -time.Second, Q: &stubQueue{}}
	b3 := NewBatch(d3, store)
	if time.Until(b3.deadline) < 4*time.Minute {
		t.Fatalf("negative timeout deadline=%v want ~5m", b3.deadline)
	}
}

func TestBatchFluentChaining(t *testing.T) {
	Reset()
	d := &Dispatcher{Q: &stubQueue{}}
	b := NewBatch(d, &fakeBatchStore{})
	if got := b.Add("a", "x"); got != b {
		t.Fatal("Add did not return same batch")
	}
	b.Add("b", 1)
	if len(b.jobs) != 2 {
		t.Fatalf("jobs len=%d want 2", len(b.jobs))
	}
	fn := func(context.Context) {}
	if got := b.Then(fn); got != b {
		t.Fatal("Then did not return same batch")
	}
	if b.onComplete == nil {
		t.Fatal("Then did not set onComplete")
	}
	cfn := func(context.Context, string, error) {}
	if got := b.Catch(cfn); got != b {
		t.Fatal("Catch did not return same batch")
	}
	if b.onFailure == nil {
		t.Fatal("Catch did not set onFailure")
	}
}

func TestBatchDispatchMarshalError(t *testing.T) {
	Reset()
	store := &fakeBatchStore{}
	d := &Dispatcher{Q: &stubQueue{}, Logger: noop.New()}
	b := NewBatch(d, store)
	b.Add("anything", make(chan int))
	err := b.Dispatch(t.Context())
	if err == nil {
		t.Fatal("Dispatch chan args want error")
	}
	if store.createCalls != 0 {
		t.Fatalf("createCalls=%d want 0 on marshal error", store.createCalls)
	}
}

func TestBatchDispatchUnknownJob(t *testing.T) {
	Reset()
	store := &fakeBatchStore{}
	d := &Dispatcher{Q: &stubQueue{}, Logger: noop.New()}
	b := NewBatch(d, store)
	b.Add("no-such-job", "arg")
	err := b.Dispatch(t.Context())
	if err == nil {
		t.Fatal("Dispatch unknown want error")
	}
	if !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("err=%v want ErrUnknownJob", err)
	}
	if store.createCalls != 0 {
		t.Fatalf("createCalls=%d want 0 on unknown job", store.createCalls)
	}
}

func TestBatchDispatchCreateError(t *testing.T) {
	Reset()
	if err := Register("batch-create", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	store := &fakeBatchStore{createErr: errors.New("create boom")}
	d := &Dispatcher{Q: &stubQueue{}, Logger: noop.New()}
	b := NewBatch(d, store)
	b.Add("batch-create", "arg")
	err := b.Dispatch(t.Context())
	if err == nil {
		t.Fatal("Dispatch create err want error")
	}
	if !errors.Is(err, store.createErr) {
		t.Fatalf("err=%v want create boom", err)
	}
}

func TestBatchDispatchPushFailureOnFailure(t *testing.T) {
	Reset()
	if err := Register("batch-pushfail", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	pushErr := errors.New("push boom")
	sq := &stubQueue{
		pushFn: func(context.Context, string, queue.Payload, queue.Headers) error { return pushErr },
	}
	store := &fakeBatchStore{getFailed: 1, getTotal: 1}
	d := &Dispatcher{Q: sq, Logger: noop.New()}
	b := NewBatch(d, store)
	var mu sync.Mutex
	var gotNames []string
	b.Add("batch-pushfail", "a").Catch(func(_ context.Context, name string, _ error) {
		mu.Lock()
		gotNames = append(gotNames, name)
		mu.Unlock()
	})
	err := b.Dispatch(t.Context())
	if err == nil {
		t.Fatal("Dispatch push failure want error")
	}
	if !errors.Is(err, pushErr) {
		t.Fatalf("err=%v want push boom", err)
	}
	if store.incrementFailed != 1 {
		t.Fatalf("incrementFailed=%d want 1", store.incrementFailed)
	}
	mu.Lock()
	n := len(gotNames)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("onFailure calls=%d want 1", n)
	}
	batchCallbacks.Lock()
	_, still := batchCallbacks.m[b.ID.String()]
	batchCallbacks.Unlock()
	if still {
		t.Fatal("settled batch should be deleted")
	}
	_ = gotNames
}

func TestBatchDispatchPushFailureIncrementError(t *testing.T) {
	Reset()
	if err := Register("batch-pushfail2", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	pushErr := errors.New("push boom")
	sq := &stubQueue{
		pushFn: func(context.Context, string, queue.Payload, queue.Headers) error { return pushErr },
	}
	store := &fakeBatchStore{incrementFailedErr: errors.New("inc boom"), getTotal: 5}
	d := &Dispatcher{Q: sq, Logger: noop.New()}
	b := NewBatch(d, store)
	called := 0
	b.Add("batch-pushfail2", "a").Catch(func(context.Context, string, error) { called++ })
	err := b.Dispatch(t.Context())
	if err == nil {
		t.Fatal("Dispatch want error")
	}
	if called != 0 {
		t.Fatalf("onFailure calls=%d want 0 when increment fails", called)
	}
	if store.incrementFailed != 1 {
		t.Fatalf("incrementFailed=%d want 1", store.incrementFailed)
	}
}

func TestBatchDispatchAllPushesOK(t *testing.T) {
	Reset()
	if err := Register("batch-ok", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sq := &stubQueue{}
	store := &fakeBatchStore{}
	d := &Dispatcher{Q: sq, Logger: noop.New()}
	b := NewBatch(d, store)
	b.Add("batch-ok", "a").Add("batch-ok", "b")
	if err := b.Dispatch(t.Context()); err != nil {
		t.Fatalf("Dispatch ok err=%v", err)
	}
	if sq.pushes != 2 {
		t.Fatalf("pushes=%d want 2", sq.pushes)
	}
	if store.createCalls != 1 {
		t.Fatalf("createCalls=%d want 1", store.createCalls)
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m[b.ID.String()]
	batchCallbacks.Unlock()
	if !ok {
		t.Fatal("successful batch callback should remain registered")
	}
}

func TestBatchDispatchEmptyGetError(t *testing.T) {
	Reset()
	store := &fakeBatchStore{getErr: errors.New("get boom")}
	d := &Dispatcher{Q: &stubQueue{}, Logger: noop.New()}
	b := NewBatch(d, store)
	completed := false
	b.Then(func(context.Context) { completed = true })
	if err := b.Dispatch(t.Context()); err != nil {
		t.Fatalf("empty Dispatch err=%v", err)
	}
	if completed {
		t.Fatal("onComplete should not run on Get error")
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m[b.ID.String()]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("empty batch entry should be deleted")
	}
}

func TestBatchDispatchEmptyFailedPositive(t *testing.T) {
	Reset()
	store := &fakeBatchStore{getFailed: 1, getTotal: 1}
	d := &Dispatcher{Q: &stubQueue{}, Logger: noop.New()}
	b := NewBatch(d, store)
	completed := false
	b.Then(func(context.Context) { completed = true })
	if err := b.Dispatch(t.Context()); err != nil {
		t.Fatalf("empty Dispatch err=%v", err)
	}
	if completed {
		t.Fatal("onComplete should not run when failed>0")
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m[b.ID.String()]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("empty batch entry should be deleted")
	}
}

func TestBatchDispatchEmptyComplete(t *testing.T) {
	Reset()
	store := &fakeBatchStore{getCompleted: 0, getFailed: 0, getTotal: 0}
	d := &Dispatcher{Q: &stubQueue{}, Logger: noop.New()}
	b := NewBatch(d, store)
	calls := 0
	b.Then(func(context.Context) { calls++ })
	if err := b.Dispatch(t.Context()); err != nil {
		t.Fatalf("empty Dispatch err=%v", err)
	}
	if calls != 1 {
		t.Fatalf("onComplete calls=%d want 1", calls)
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m[b.ID.String()]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("empty batch entry should be deleted")
	}
}

func TestBatchReportResult(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getTotal: 5}
	reportBatchResult(t.Context(), fb, "bid-nil", "j", nil)
	if fb.incrementCompleted != 1 {
		t.Fatalf("nil err incrementCompleted=%d want 1", fb.incrementCompleted)
	}
	fb2 := &fakeBatchStore{getTotal: 5}
	sentinel := errors.New("job fail")
	reportBatchResult(t.Context(), fb2, "bid-err", "j", sentinel)
	if fb2.incrementFailed != 1 {
		t.Fatalf("non-nil err incrementFailed=%d want 1", fb2.incrementFailed)
	}
}

func TestBatchReportFailureIncrementError(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{incrementFailedErr: errors.New("inc boom")}
	reportBatchFailure(t.Context(), fb, "bid-inc", "j", errors.New("x"))
	if fb.incrementFailed != 1 {
		t.Fatalf("incrementFailed=%d want 1", fb.incrementFailed)
	}
}

func TestBatchReportFailureSettlingDefers(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getTotal: 5}
	cb := &batchCallback{store: fb, completeOnce: new(sync.Once), logger: noop.New()}
	cb.settling.Store(true)
	batchCallbacks.Lock()
	batchCallbacks.m["bid-settling"] = cb
	batchCallbacks.Unlock()
	called := false
	cb.onFailure = func(context.Context, string, error) { called = true }
	reportBatchFailure(t.Context(), fb, "bid-settling", "j", errors.New("x"))
	if called {
		t.Fatal("onFailure should not run while settling")
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["bid-settling"]
	batchCallbacks.Unlock()
	if !ok {
		t.Fatal("settling entry should remain")
	}
}

func TestBatchReportFailureInvokesAndDeletes(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getFailed: 1, getTotal: 1}
	cb := &batchCallback{store: fb, completeOnce: new(sync.Once), logger: noop.New()}
	var gotName string
	var gotErr error
	sentinel := errors.New("member fail")
	cb.onFailure = func(_ context.Context, name string, err error) {
		gotName = name
		gotErr = err
	}
	batchCallbacks.Lock()
	batchCallbacks.m["bid-del"] = cb
	batchCallbacks.Unlock()
	reportBatchFailure(t.Context(), fb, "bid-del", "myjob", sentinel)
	if gotName != "myjob" || !errors.Is(gotErr, sentinel) {
		t.Fatalf("onFailure name=%q err=%v want myjob/sentinel", gotName, gotErr)
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["bid-del"]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("settled entry should be deleted")
	}
}

func TestBatchReportFailureNotSettledKeeps(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getFailed: 0, getTotal: 5}
	cb := &batchCallback{store: fb, completeOnce: new(sync.Once), logger: noop.New(), onFailure: func(context.Context, string, error) {}}
	batchCallbacks.Lock()
	batchCallbacks.m["bid-keep"] = cb
	batchCallbacks.Unlock()
	reportBatchFailure(t.Context(), fb, "bid-keep", "j", errors.New("x"))
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["bid-keep"]
	batchCallbacks.Unlock()
	if !ok {
		t.Fatal("unsettled entry should remain")
	}
}

func TestBatchReportFailureNoEntry(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getFailed: 1, getTotal: 1}
	reportBatchFailure(t.Context(), fb, "bid-missing", "j", errors.New("x"))
	if fb.incrementFailed != 1 {
		t.Fatalf("incrementFailed=%d want 1", fb.incrementFailed)
	}
}

func TestBatchReportSuccessIncrementError(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{incrementCompletedErr: errors.New("inc boom")}
	reportBatchSuccess(t.Context(), fb, "bid-ok-err")
	if fb.incrementCompleted != 1 {
		t.Fatalf("incrementCompleted=%d want 1", fb.incrementCompleted)
	}
}

func TestBatchReportSuccessSettlingDefers(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getTotal: 5}
	cb := &batchCallback{store: fb, completeOnce: new(sync.Once), logger: noop.New()}
	cb.settling.Store(true)
	batchCallbacks.Lock()
	batchCallbacks.m["bid-ssettle"] = cb
	batchCallbacks.Unlock()
	reportBatchSuccess(t.Context(), fb, "bid-ssettle")
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["bid-ssettle"]
	batchCallbacks.Unlock()
	if !ok {
		t.Fatal("settling entry should remain")
	}
}

func TestBatchReportSuccessNotSettled(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 5}
	cb := &batchCallback{store: fb, completeOnce: new(sync.Once), logger: noop.New(), onComplete: func(context.Context) {}}
	batchCallbacks.Lock()
	batchCallbacks.m["bid-nset"] = cb
	batchCallbacks.Unlock()
	reportBatchSuccess(t.Context(), fb, "bid-nset")
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["bid-nset"]
	batchCallbacks.Unlock()
	if !ok {
		t.Fatal("unsettled entry should remain")
	}
}

func TestBatchReportSuccessCompletes(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getCompleted: 2, getFailed: 0, getTotal: 2}
	cb := &batchCallback{store: fb, completeOnce: new(sync.Once), logger: noop.New()}
	calls := 0
	cb.onComplete = func(context.Context) { calls++ }
	batchCallbacks.Lock()
	batchCallbacks.m["bid-done"] = cb
	batchCallbacks.Unlock()
	reportBatchSuccess(t.Context(), fb, "bid-done")
	reportBatchSuccess(t.Context(), fb, "bid-done")
	if calls != 1 {
		t.Fatalf("onComplete calls=%d want 1 (once)", calls)
	}
}

func TestBatchReportSuccessFailedPositive(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getCompleted: 1, getFailed: 1, getTotal: 2}
	cb := &batchCallback{store: fb, completeOnce: new(sync.Once), logger: noop.New()}
	called := false
	cb.onComplete = func(context.Context) { called = true }
	batchCallbacks.Lock()
	batchCallbacks.m["bid-fail"] = cb
	batchCallbacks.Unlock()
	reportBatchSuccess(t.Context(), fb, "bid-fail")
	if called {
		t.Fatal("onComplete should not run when failed>0")
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["bid-fail"]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("settled failed entry should be deleted")
	}
}

func TestBatchReportSuccessNoEntry(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 1}
	reportBatchSuccess(t.Context(), fb, "bid-absent")
	if fb.incrementCompleted != 1 {
		t.Fatalf("incrementCompleted=%d want 1", fb.incrementCompleted)
	}
}

func TestBatchReportSuccessNilComplete(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 1}
	cb := &batchCallback{store: fb, completeOnce: new(sync.Once), logger: noop.New()}
	batchCallbacks.Lock()
	batchCallbacks.m["bid-nilcb"] = cb
	batchCallbacks.Unlock()
	reportBatchSuccess(t.Context(), fb, "bid-nilcb")
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["bid-nilcb"]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("settled entry should be deleted even with nil onComplete")
	}
}

func TestBatchLogger(t *testing.T) {
	Reset()
	n := noop.New()
	cb := &batchCallback{store: &fakeBatchStore{}, completeOnce: new(sync.Once), logger: n}
	batchCallbacks.Lock()
	batchCallbacks.m["bid-log"] = cb
	batchCallbacks.Unlock()
	if got := batchLogger("bid-log"); got != n {
		t.Fatal("batchLogger should return registered logger")
	}
	if got := batchLogger("bid-unknown"); got == nil {
		t.Fatal("batchLogger unknown should return noop, got nil")
	} else if got.Name() != "noop" {
		t.Fatalf("batchLogger unknown Name=%q want noop", got.Name())
	}
}

func TestBatchSettle(t *testing.T) {
	Reset()
	fb := &fakeBatchStore{getErr: errors.New("get boom")}
	failed, settled := batchSettle(t.Context(), fb, "x")
	if settled || failed != 0 {
		t.Fatalf("Get error got (%d,%v) want (0,false)", failed, settled)
	}
	fb2 := &fakeBatchStore{getCompleted: 2, getFailed: 1, getTotal: 3}
	failed, settled = batchSettle(t.Context(), fb2, "x")
	if !settled || failed != 1 {
		t.Fatalf("settled got (%d,%v) want (1,true)", failed, settled)
	}
	fb3 := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 5}
	failed, settled = batchSettle(t.Context(), fb3, "x")
	if settled || failed != 0 {
		t.Fatalf("unsettled got (%d,%v) want (0,false)", failed, settled)
	}
}

func TestBatchCryptoRand(t *testing.T) {
	Reset()
	if got := cryptoRandInt64n(0); got != 0 {
		t.Fatalf("n=0 got %d want 0", got)
	}
	if got := cryptoRandInt64n(-5); got != 0 {
		t.Fatalf("n<0 got %d want 0", got)
	}
	old := randReader
	randReader = batchFailReader{}
	defer func() { randReader = old }()
	if got := cryptoRandInt64n(10); got != 0 {
		t.Fatalf("reader error got %d want 0", got)
	}
	randReader = old
	for i := 0; i < 50; i++ {
		got := cryptoRandInt64n(10)
		if got < 0 || got >= 10 {
			t.Fatalf("cryptoRandInt64n(10)=%d out of range", got)
		}
	}
}

func TestBatchJitterDuration(t *testing.T) {
	Reset()
	d := 50 * time.Millisecond
	for i := 0; i < 20; i++ {
		got := jitterDuration(d)
		if got < 0 || got >= d {
			t.Fatalf("jitterDuration=%v out of [0,%v)", got, d)
		}
	}
	if got := jitterDuration(0); got != 0 {
		t.Fatalf("jitterDuration(0)=%v want 0", got)
	}
}

func TestBatchSleepWithContext(t *testing.T) {
	Reset()
	if !sleepWithContext(t.Context(), 0) {
		t.Fatal("d<=0 live ctx want true")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if sleepWithContext(canceled, 0) {
		t.Fatal("d<=0 canceled ctx want false")
	}
	if !sleepWithContext(t.Context(), 5*time.Millisecond) {
		t.Fatal("d>0 live ctx want true")
	}
	canceled2, cancel2 := context.WithCancel(t.Context())
	cancel2()
	if sleepWithContext(canceled2, 50*time.Millisecond) {
		t.Fatal("d>0 canceled ctx want false")
	}
}

func TestBatchSettleDueBatchesNoJitter(t *testing.T) {
	Reset()
	old := triggerJitter
	triggerJitter = func() bool { return false }
	defer func() { triggerJitter = old }()
	before := SweepBatchCallbacksCalls()
	settleDueBatches(t.Context())
	after := SweepBatchCallbacksCalls()
	if after != before+1 {
		t.Fatalf("calls before %d after %d want +1", before, after)
	}
}

func TestBatchSettleDueBatchesWithDeadline(t *testing.T) {
	Reset()
	old := triggerJitter
	triggerJitter = func() bool { return false }
	defer func() { triggerJitter = old }()
	futureCb := &batchCallback{store: &fakeBatchStore{}, completeOnce: new(sync.Once), logger: noop.New(), deadline: time.Now().Add(time.Hour)}
	zeroCb := &batchCallback{store: &fakeBatchStore{}, completeOnce: new(sync.Once), logger: noop.New()}
	batchCallbacks.Lock()
	batchCallbacks.m["future"] = futureCb
	batchCallbacks.m["zero"] = zeroCb
	batchCallbacks.m["nilcb"] = nil
	batchCallbacks.Unlock()
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Minute))
	defer cancel()
	before := SweepBatchCallbacksCalls()
	settleDueBatches(ctx)
	after := SweepBatchCallbacksCalls()
	if after != before+1 {
		t.Fatalf("calls before %d after %d want +1", before, after)
	}
	batchCallbacks.Lock()
	_, okF := batchCallbacks.m["future"]
	_, okZ := batchCallbacks.m["zero"]
	batchCallbacks.Unlock()
	if !okF || !okZ {
		t.Fatal("non-expired entries should remain")
	}
}

func TestBatchSettleDueBatchesJitterExpired(t *testing.T) {
	Reset()
	old := triggerJitter
	triggerJitter = func() bool { return true }
	defer func() { triggerJitter = old }()
	store := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 1}
	cb := &batchCallback{store: store, completeOnce: new(sync.Once), logger: noop.New(), deadline: time.Now().Add(-time.Second)}
	calls := 0
	cb.onComplete = func(context.Context) { calls++ }
	batchCallbacks.Lock()
	batchCallbacks.m["expired-j"] = cb
	batchCallbacks.Unlock()
	before := SweepBatchCallbacksCalls()
	settleDueBatches(t.Context())
	after := SweepBatchCallbacksCalls()
	if after != before+1 {
		t.Fatalf("calls before %d after %d want +1", before, after)
	}
	if calls != 1 {
		t.Fatalf("expired onComplete calls=%d want 1", calls)
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["expired-j"]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("expired entry should be deleted")
	}
}

func TestBatchSettleDueBatchesJitterCanceled(t *testing.T) {
	Reset()
	old := triggerJitter
	triggerJitter = func() bool { return true }
	defer func() { triggerJitter = old }()
	store := &fakeBatchStore{getCompleted: 1, getTotal: 1}
	cb := &batchCallback{store: store, completeOnce: new(sync.Once), logger: noop.New(), deadline: time.Now().Add(-time.Second)}
	batchCallbacks.Lock()
	batchCallbacks.m["expired-c"] = cb
	batchCallbacks.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := SweepBatchCallbacksCalls()
	settleDueBatches(ctx)
	after := SweepBatchCallbacksCalls()
	if after != before+1 {
		t.Fatalf("calls before %d after %d want +1", before, after)
	}
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["expired-c"]
	batchCallbacks.Unlock()
	if !ok {
		t.Fatal("canceled sweep should not settle expired entry")
	}
}

func TestBatchSettleExpiredBranches(t *testing.T) {
	Reset()
	ctx := t.Context()
	settleExpired(ctx, "x", nil)
	settleExpired(ctx, "x", &batchCallback{store: nil, completeOnce: new(sync.Once), logger: noop.New()})
	other := &batchCallback{store: &fakeBatchStore{}, completeOnce: new(sync.Once), logger: noop.New()}
	settleExpired(ctx, "not-in-map", other)
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["not-in-map"]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("acquire-false should not insert")
	}
	getStore := &fakeBatchStore{getErr: errors.New("get boom")}
	getCb := &batchCallback{store: getStore, completeOnce: new(sync.Once), logger: noop.New()}
	batchCallbacks.Lock()
	batchCallbacks.m["bid-loadfail"] = getCb
	batchCallbacks.Unlock()
	settleExpired(ctx, "bid-loadfail", getCb)
	if getCb.settling.Load() {
		t.Fatal("settling should be reset after load fail")
	}
	fullStore := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 1}
	fullCb := &batchCallback{store: fullStore, completeOnce: new(sync.Once), logger: noop.New(), deadline: time.Now().Add(-time.Second)}
	fullCb.onComplete = func(context.Context) {}
	batchCallbacks.Lock()
	batchCallbacks.m["bid-full"] = fullCb
	batchCallbacks.Unlock()
	settleExpired(ctx, "bid-full", fullCb)
	batchCallbacks.Lock()
	_, ok = batchCallbacks.m["bid-full"]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("full path should delete entry")
	}
	if fullCb.settling.Load() {
		t.Fatal("settling should be reset after full path")
	}
}

func TestBatchAcquireSettling(t *testing.T) {
	Reset()
	cb := &batchCallback{completeOnce: new(sync.Once), logger: noop.New()}
	if acquireSettling("missing", cb) {
		t.Fatal("missing id want false")
	}
	cb1 := &batchCallback{completeOnce: new(sync.Once), logger: noop.New()}
	cb2 := &batchCallback{completeOnce: new(sync.Once), logger: noop.New()}
	batchCallbacks.Lock()
	batchCallbacks.m["bid-acq"] = cb1
	batchCallbacks.Unlock()
	if acquireSettling("bid-acq", cb2) {
		t.Fatal("cur!=cb want false")
	}
	if !acquireSettling("bid-acq", cb1) {
		t.Fatal("CAS success want true")
	}
	cb1.settling.Store(false)
	cb1.settling.Store(true)
	if acquireSettling("bid-acq", cb1) {
		t.Fatal("CAS fail want false")
	}
	cb1.settling.Store(false)
}

func TestBatchLoadAndFailLost(t *testing.T) {
	Reset()
	ctx := t.Context()
	errStore := &fakeBatchStore{getErr: errors.New("get boom")}
	errCb := &batchCallback{store: errStore, completeOnce: new(sync.Once), logger: noop.New()}
	if loadAndFailLost(ctx, "x", errCb) {
		t.Fatal("Get error want false")
	}
	evenStore := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 1}
	evenCb := &batchCallback{store: evenStore, completeOnce: new(sync.Once), logger: noop.New()}
	if !loadAndFailLost(ctx, "x", evenCb) {
		t.Fatal("lost<=0 want true")
	}
	if evenStore.incrementFailedBy != 0 {
		t.Fatalf("lost<=0 incrementFailedBy=%d want 0", evenStore.incrementFailedBy)
	}
	incStore := &fakeBatchStore{getCompleted: 0, getFailed: 0, getTotal: 2, incrementFailedByErr: errors.New("inc boom")}
	incCb := &batchCallback{store: incStore, completeOnce: new(sync.Once), logger: noop.New(), onFailure: func(context.Context, string, error) {}}
	if !loadAndFailLost(ctx, "x", incCb) {
		t.Fatal("IncrementFailedBy error want true")
	}
	okStore := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 3}
	okCb := &batchCallback{store: okStore, completeOnce: new(sync.Once), logger: noop.New()}
	count := 0
	okCb.onFailure = func(_ context.Context, name string, err error) {
		count++
		if name != "" {
			t.Errorf("lost onFailure name=%q want empty", name)
		}
		if !errors.Is(err, ErrMemberLost) {
			t.Errorf("lost onFailure err=%v want ErrMemberLost", err)
		}
	}
	if !loadAndFailLost(ctx, "x", okCb) {
		t.Fatal("ok path want true")
	}
	if count != 2 {
		t.Fatalf("onFailure calls=%d want 2", count)
	}
	nilStore := &fakeBatchStore{getCompleted: 0, getFailed: 0, getTotal: 1}
	nilCb := &batchCallback{store: nilStore, completeOnce: new(sync.Once), logger: noop.New()}
	if !loadAndFailLost(ctx, "x", nilCb) {
		t.Fatal("nil onFailure want true")
	}
	if nilStore.incrementFailedBy != 1 {
		t.Fatalf("incrementFailedBy=%d want 1", nilStore.incrementFailedBy)
	}
}

func TestBatchFinalizeExpired(t *testing.T) {
	Reset()
	ctx := t.Context()
	errStore := &fakeBatchStore{getErr: errors.New("get boom")}
	errCb := &batchCallback{store: errStore, completeOnce: new(sync.Once), logger: noop.New()}
	batchCallbacks.Lock()
	batchCallbacks.m["fin-err"] = errCb
	batchCallbacks.Unlock()
	finalizeExpired(ctx, "fin-err", errCb)
	batchCallbacks.Lock()
	_, ok := batchCallbacks.m["fin-err"]
	batchCallbacks.Unlock()
	if !ok {
		t.Fatal("Get error should keep entry")
	}
	incStore := &fakeBatchStore{getCompleted: 0, getFailed: 0, getTotal: 3}
	incCb := &batchCallback{store: incStore, completeOnce: new(sync.Once), logger: noop.New()}
	batchCallbacks.Lock()
	batchCallbacks.m["fin-inc"] = incCb
	batchCallbacks.Unlock()
	finalizeExpired(ctx, "fin-inc", incCb)
	batchCallbacks.Lock()
	_, ok = batchCallbacks.m["fin-inc"]
	batchCallbacks.Unlock()
	if !ok {
		t.Fatal("incomplete should keep entry")
	}
	doneStore := &fakeBatchStore{getCompleted: 2, getFailed: 0, getTotal: 2}
	doneCb := &batchCallback{store: doneStore, completeOnce: new(sync.Once), logger: noop.New()}
	calls := 0
	doneCb.onComplete = func(context.Context) { calls++ }
	batchCallbacks.Lock()
	batchCallbacks.m["fin-done"] = doneCb
	batchCallbacks.Unlock()
	finalizeExpired(ctx, "fin-done", doneCb)
	if calls != 1 {
		t.Fatalf("onComplete calls=%d want 1", calls)
	}
	batchCallbacks.Lock()
	_, ok = batchCallbacks.m["fin-done"]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("completed entry should be deleted")
	}
	failStore := &fakeBatchStore{getCompleted: 1, getFailed: 1, getTotal: 2}
	failCb := &batchCallback{store: failStore, completeOnce: new(sync.Once), logger: noop.New()}
	called := false
	failCb.onComplete = func(context.Context) { called = true }
	batchCallbacks.Lock()
	batchCallbacks.m["fin-fail"] = failCb
	batchCallbacks.Unlock()
	finalizeExpired(ctx, "fin-fail", failCb)
	if called {
		t.Fatal("onComplete should not run when failed>0")
	}
	batchCallbacks.Lock()
	_, ok = batchCallbacks.m["fin-fail"]
	batchCallbacks.Unlock()
	if ok {
		t.Fatal("failed entry should be deleted")
	}
	otherStore := &fakeBatchStore{getCompleted: 1, getFailed: 0, getTotal: 1}
	otherCb := &batchCallback{store: otherStore, completeOnce: new(sync.Once), logger: noop.New()}
	otherCb.onComplete = func(context.Context) {}
	replacement := &batchCallback{store: &fakeBatchStore{}, completeOnce: new(sync.Once), logger: noop.New()}
	batchCallbacks.Lock()
	batchCallbacks.m["fin-other"] = replacement
	batchCallbacks.Unlock()
	finalizeExpired(ctx, "fin-other", otherCb)
	batchCallbacks.Lock()
	cur, ok := batchCallbacks.m["fin-other"]
	batchCallbacks.Unlock()
	if !ok || cur != replacement {
		t.Fatal("cur!=cb should keep replacement entry")
	}
}

func TestBatchSweepDelegates(t *testing.T) {
	Reset()
	old := triggerJitter
	triggerJitter = func() bool { return false }
	defer func() { triggerJitter = old }()
	before := SweepBatchCallbacksCalls()
	SweepBatchCallbacks(t.Context())
	after := SweepBatchCallbacksCalls()
	if after != before+1 {
		t.Fatalf("Sweep calls before %d after %d want +1", before, after)
	}
}
