package container

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/cache"
	cachememory "github.com/zenta-dev/zever/cache/memory"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	"github.com/zenta-dev/zever/observability"
	observabilitynoop "github.com/zenta-dev/zever/observability/noop"
	"github.com/zenta-dev/zever/queue"
	queuememory "github.com/zenta-dev/zever/queue/memory"
	"github.com/zenta-dev/zever/scheduler"
	schedulerembedded "github.com/zenta-dev/zever/scheduler/embedded"
)

// errFakeUnimplemented marks fakeCtx methods that tests never invoke.
var errFakeUnimplemented = errors.New("fakeCtx: unimplemented")

// registerTestAdapters wires the zero-infrastructure factories once.
// Adapter registries are global; duplicates from parallel helpers are ignored.
var registerAdaptersOnce sync.Once

func registerTestAdapters() {
	registerAdaptersOnce.Do(func() {
		_ = cache.Register(cache.Memory, cachememory.New)
		_ = db.Register(db.SQLite, sqlite.New)
		_ = queue.Register(queue.Memory, queuememory.New)
		_ = scheduler.Register(scheduler.Embedded, schedulerembedded.New)
	})
}

// fakeCtx implements cache.Cache and db.DB through a shared Close(ctx).
// One pointer can back two lazy fields, which exercises pointer dedup.
type fakeCtx struct {
	name         string
	order        *[]string
	mu           *sync.Mutex
	calls        atomic.Int32
	started      atomic.Bool
	block        time.Duration
	panicOnClose bool
	closeErr     error
	ignoreCtx    bool
}

func (m *fakeCtx) Close(ctx context.Context) error {
	m.started.Store(true)
	if m.panicOnClose {
		panic("fakeCtx: simulated panic in service Close")
	}
	if m.block > 0 {
		if m.ignoreCtx {
			time.Sleep(m.block)
		} else {
			select {
			case <-time.After(m.block):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	if m.closeErr != nil {
		return m.closeErr
	}
	m.calls.Add(1)
	if m.order != nil && m.mu != nil {
		m.mu.Lock()
		*m.order = append(*m.order, m.name)
		m.mu.Unlock()
	}
	return nil
}

func (m *fakeCtx) Get(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (m *fakeCtx) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}
func (m *fakeCtx) SetIfAbsent(_ context.Context, _ string, _ []byte, _ time.Duration) (bool, error) {
	return false, nil
}
func (m *fakeCtx) Delete(_ context.Context, _ string) error         { return nil }
func (m *fakeCtx) Increment(_ context.Context, _ string) error      { return nil }
func (m *fakeCtx) Decrement(_ context.Context, _ string) error      { return nil }
func (m *fakeCtx) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (m *fakeCtx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nil, errFakeUnimplemented
}
func (m *fakeCtx) Exec(_ context.Context, _ string, _ ...any) (int64, error) {
	return 0, nil
}
func (m *fakeCtx) Ping(_ context.Context) error { return nil }
func (m *fakeCtx) Dialect() string              { return "fake" }

// fakeQ implements queue.Queue (Close() error, no context).
type fakeQ struct {
	name  string
	order *[]string
	mu    *sync.Mutex
	calls atomic.Int32
	block time.Duration
}

func (m *fakeQ) Close() error {
	if m.block > 0 {
		time.Sleep(m.block)
	}
	m.calls.Add(1)
	if m.order != nil && m.mu != nil {
		m.mu.Lock()
		*m.order = append(*m.order, m.name)
		m.mu.Unlock()
	}
	return nil
}

func (m *fakeQ) Push(_ context.Context, _ string, _ queue.Payload, _ queue.Headers) error {
	return nil
}
func (m *fakeQ) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}
func (m *fakeQ) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, nil
}
func (m *fakeQ) Ack(_ context.Context, _ queue.Message) error { return nil }
func (m *fakeQ) Nack(_ context.Context, _ queue.Message, _ bool) error {
	return nil
}
func (m *fakeQ) Length(_ context.Context, _ string) (int64, error) { return 0, nil }
func (m *fakeQ) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }
func (m *fakeQ) Name() string                                      { return "fake" }

// fakeSched implements scheduler.Scheduler (Stop() error probe).
type fakeSched struct {
	name  string
	order *[]string
	mu    *sync.Mutex
	calls atomic.Int32
}

func (m *fakeSched) Schedule(_ context.Context, _, _ string, _ any) (scheduler.EntryID, error) {
	return 0, nil
}
func (m *fakeSched) Remove(_ scheduler.EntryID) error { return nil }
func (m *fakeSched) Entries() []scheduler.EntryID     { return nil }
func (m *fakeSched) Start() error                     { return nil }
func (m *fakeSched) Stop() error {
	m.calls.Add(1)
	if m.order != nil && m.mu != nil {
		m.mu.Lock()
		*m.order = append(*m.order, m.name)
		m.mu.Unlock()
	}
	return nil
}
func (m *fakeSched) Name() string { return "fake" }

// waitForCloseStarted polls until a fakeCtx Close has been entered, so tests
// cancel only once Close is in flight instead of sleeping a fixed delay.
func waitForCloseStarted(t *testing.T, m *fakeCtx, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !m.started.Load() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s Close to start", m.name)
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for %s Close to start", m.name)
		case <-timer.C:
		}
	}
}

// waitForCloseCalls polls until the ignored-ctx goroutine finishes its block
// (calls incremented at the end of fakeCtx.Close), replacing the fixed
// leak-drain sleep so goleak sees no leftover goroutine without wall-clock waits.
func waitForCloseCalls(t *testing.T, m *fakeCtx, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for m.calls.Load() < 1 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s goroutine to finish", m.name)
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for %s goroutine to finish", m.name)
		case <-timer.C:
		}
	}
}

type noCloser struct{}

func TestContainer_Snapshots_CoversAllServices(t *testing.T) {
	c := New(config.Default())
	typ := reflect.TypeOf(c).Elem()
	totalLazy := 0
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Name == "cfg" {
			continue
		}
		totalLazy++
	}
	const orderedExclusions = 5 // cache, queue, scheduler, job, grpcServer
	snaps := c.snapshots()
	if got := len(snaps) + orderedExclusions; got != totalLazy {
		t.Fatalf("snapshots drift: len(snapshots)=%d + ordered %d = %d, want %d lazy fields", len(snaps), orderedExclusions, got, totalLazy)
	}
	if got := len(snapshotServiceNames()); got != len(snaps) {
		t.Fatalf("service names drift: names=%d snapshots=%d", got, len(snaps))
	}
}

func TestContainer_Close_OnlyResolvedClosed(t *testing.T) {
	registerTestAdapters()
	cfg := config.Default()
	cfg.DB.Options.Path = ":memory:"
	cfg.Payment.Adapter = "no-such-adapter-zzz"
	c := New(cfg)

	if _, err := c.DB(); err != nil {
		t.Fatalf("DB: %v", err)
	}
	if _, err := c.Cache(); err != nil {
		t.Fatalf("Cache: %v", err)
	}
	if _, err := c.Queue(); err != nil {
		t.Fatalf("Queue: %v", err)
	}
	if _, err := c.Scheduler(); err != nil {
		t.Fatalf("Scheduler: %v", err)
	}
	if _, err := c.Job(); err != nil {
		t.Fatalf("Job: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, ok := c.payment.getIfResolved(); ok {
		t.Fatal("untouched payment must stay unresolved after Close")
	}
}

func TestContainer_Close_NeverOpensUntouched(t *testing.T) {
	registerTestAdapters()
	cfg := config.Default()
	cfg.DB.Options.Path = ":memory:"
	cfg.Auth.Adapter = "bogus-adapter-never-opened"
	c := New(cfg)

	if _, err := c.DB(); err != nil {
		t.Fatalf("DB: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close must not open untouched services, got: %v", err)
	}
	if _, ok := c.auth.getIfResolved(); ok {
		t.Fatal("untouched auth must stay unresolved")
	}
}

func TestContainer_Close_EmptyContainer(t *testing.T) {
	c := New(config.Default())
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close on untouched container: %v", err)
	}
}

func TestContainer_Close_NilConfigEmpty(t *testing.T) {
	c := New(nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close on nil-config container: %v", err)
	}
}

func TestContainer_Close_Ordering(t *testing.T) {
	c := New(config.Default())
	var order []string
	var mu sync.Mutex
	sched := &fakeSched{name: "scheduler", order: &order, mu: &mu}
	cacheMock := &fakeCtx{name: "cache", order: &order, mu: &mu}
	queueMock := &fakeQ{name: "queue", order: &order, mu: &mu}
	other := &fakeCtx{name: "other", order: &order, mu: &mu}

	c.scheduler.val = sched
	c.scheduler.done = true
	c.scheduler.ready.Store(true)
	c.cache.val = cacheMock
	c.cache.done = true
	c.cache.ready.Store(true)
	c.queue.val = queueMock
	c.queue.done = true
	c.queue.ready.Store(true)
	c.db.val = other
	c.db.done = true
	c.db.ready.Store(true)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	pos := func(name string) int {
		for i, n := range order {
			if n == name {
				return i
			}
		}
		return -1
	}
	sPos, oPos, cPos, qPos := pos("scheduler"), pos("other"), pos("cache"), pos("queue")
	if sPos == -1 || oPos == -1 || cPos == -1 || qPos == -1 {
		t.Fatalf("missing closes in order %v", order)
	}
	if sPos >= oPos {
		t.Fatalf("scheduler should close before snapshots (other): order %v", order)
	}
	if oPos >= cPos || oPos >= qPos {
		t.Fatalf("snapshots (other) should close before cache/queue: order %v", order)
	}
}

func TestContainer_Close_DedupKeyKinds(t *testing.T) {
	if _, ok := dedupKey(nil); ok {
		t.Fatal("dedupKey(nil) should be false")
	}
	ch := make(chan struct{})
	if _, ok := dedupKey(ch); !ok {
		t.Fatal("dedupKey(chan) should be ok")
	}
	var nilCh chan struct{}
	if _, ok := dedupKey(nilCh); ok {
		t.Fatal("dedupKey(nil chan) should be false")
	}
	m := map[string]string{"k": "v"}
	if _, ok := dedupKey(m); !ok {
		t.Fatal("dedupKey(map) should be ok")
	}
	var nilMap map[string]string
	if _, ok := dedupKey(nilMap); ok {
		t.Fatal("dedupKey(nil map) should be false")
	}
	s := []byte{1, 2, 3}
	if _, ok := dedupKey(s); !ok {
		t.Fatal("dedupKey(slice) should be ok")
	}
	var nilSlice []byte
	if _, ok := dedupKey(nilSlice); ok {
		t.Fatal("dedupKey(nil slice) should be false")
	}
	fn := func() {}
	if _, ok := dedupKey(fn); !ok {
		t.Fatal("dedupKey(func) should be ok")
	}
	var nilFn func()
	if _, ok := dedupKey(nilFn); ok {
		t.Fatal("dedupKey(nil func) should be false")
	}
	p := &fakeCtx{}
	if _, ok := dedupKey(p); !ok {
		t.Fatal("dedupKey(pointer) should be ok")
	}
	var nilP *fakeCtx
	if _, ok := dedupKey(nilP); ok {
		t.Fatal("dedupKey(nil pointer) should be false")
	}
	if _, ok := dedupKey(42); ok {
		t.Fatal("dedupKey(int) should be false")
	}
	if _, ok := dedupKey("x"); ok {
		t.Fatal("dedupKey(string) should be false")
	}
}

func TestContainer_Close_FindPointerFields(t *testing.T) {
	type mixed struct {
		N   int
		S   string
		Nil *fakeCtx
		Ch  chan struct{}
		M   map[string]string
		Sl  []byte
		Fn  func()
		P   *fakeCtx
	}
	v := mixed{Ch: make(chan struct{}), M: map[string]string{}, Sl: []byte{1}, Fn: func() {}, P: &fakeCtx{}}
	if _, _, ok := findPointerInValue(reflect.ValueOf(v)); !ok {
		t.Fatal("mixed struct should dedup")
	}
	type ifaceNonPtr struct {
		I any
	}
	if _, _, ok := findPointerInValue(reflect.ValueOf(ifaceNonPtr{I: 42})); ok {
		t.Fatal("interface holding int should not dedup")
	}
	var nilIface ifaceNonPtr
	if _, _, ok := findPointerInValue(reflect.ValueOf(nilIface)); ok {
		t.Fatal("nil interface field should not dedup")
	}
	type nestedMiss struct {
		In ifaceNonPtr
		N  int
	}
	if _, _, ok := findPointerInValue(reflect.ValueOf(nestedMiss{})); ok {
		t.Fatal("nested miss should not dedup")
	}
	type structIface struct {
		I any
	}
	if _, _, ok := findPointerInValue(reflect.ValueOf(structIface{I: struct{ N int }{N: 1}})); ok {
		t.Fatal("interface holding plain struct should not dedup")
	}
	var nilPtr *fakeCtx
	if _, _, ok := findPointerInValue(reflect.ValueOf(structIface{I: nilPtr})); ok {
		t.Fatal("interface holding nil pointer should not dedup")
	}
	type wrap struct {
		P *fakeCtx
	}
	sharedIface := &fakeCtx{}
	if _, _, ok := findPointerInValue(reflect.ValueOf(structIface{I: wrap{P: sharedIface}})); !ok {
		t.Fatal("interface holding pointer-wrapping struct should dedup")
	}
}

func TestContainer_Close_NilValueSkipped(t *testing.T) {
	c := New(config.Default())
	c.db.val = nil
	c.db.done = true
	c.db.ready.Store(true)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("nil resolved value must be skipped, got: %v", err)
	}
}

func TestContainer_Close_DedupSharedPointer(t *testing.T) {
	c := New(config.Default())
	shared := &fakeCtx{name: "shared"}
	c.cache.val = shared
	c.cache.done = true
	c.cache.ready.Store(true)
	c.db.val = shared
	c.db.done = true
	c.db.ready.Store(true)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := shared.calls.Load(); got != 1 {
		t.Fatalf("shared instance closed %d times, want 1", got)
	}
}

func TestContainer_Close_DedupStructWrappers(t *testing.T) {
	shared := &fakeCtx{name: "shared"}
	type wrapper struct {
		P *fakeCtx
	}
	w1 := wrapper{P: shared}
	w2 := wrapper{P: shared}
	k1, ok1 := dedupKey(w1)
	k2, ok2 := dedupKey(w2)
	if !ok1 || !ok2 {
		t.Fatalf("dedupKey should be ok for struct wrappers: %v %v", ok1, ok2)
	}
	if k1 != k2 {
		t.Fatalf("dedupKey mismatch for same pointer wrappers: %v vs %v", k1, k2)
	}
	kp1, _ := dedupKey(shared)
	kp2, _ := dedupKey(shared)
	if kp1 != kp2 {
		t.Fatal("pointer dedup mismatch")
	}
}

func TestContainer_Close_DedupInterfaceAndNested(t *testing.T) {
	shared := &fakeCtx{name: "shared-iface"}
	type wrapperIface struct {
		I any
	}
	w1 := wrapperIface{I: shared}
	w2 := wrapperIface{I: shared}
	k1, ok1 := dedupKey(w1)
	k2, ok2 := dedupKey(w2)
	if !ok1 || !ok2 {
		t.Fatalf("dedupKey iface wrapper should be ok: %v %v", ok1, ok2)
	}
	if k1 != k2 {
		t.Fatalf("dedupKey mismatch for interface wrappers: %v vs %v", k1, k2)
	}
	type inner struct{ P *fakeCtx }
	type outer struct{ In inner }
	o1 := outer{In: inner{P: shared}}
	o2 := outer{In: inner{P: shared}}
	k3, ok3 := dedupKey(o1)
	k4, ok4 := dedupKey(o2)
	if !ok3 || !ok4 {
		t.Fatalf("dedupKey nested struct should be ok: %v %v", ok3, ok4)
	}
	if k3 != k4 {
		t.Fatalf("dedupKey mismatch for nested struct wrappers: %v vs %v", k3, k4)
	}
}

func TestContainer_Close_DedupPlainStructMiss(t *testing.T) {
	type plain struct {
		N int
	}
	p1 := plain{N: 1}
	p2 := plain{N: 1}
	if _, ok := dedupKey(p1); ok {
		t.Fatal("dedupKey should be false for plain struct with no pointer field")
	}
	if _, ok := dedupKey(p2); ok {
		t.Fatal("dedupKey should be false for plain struct second copy")
	}
	var seen sync.Map
	count := 0
	for _, v := range []any{p1, p2} {
		if key, ok := dedupKey(v); ok {
			if _, loaded := seen.LoadOrStore(key, struct{}{}); loaded {
				continue
			}
		}
		count++
	}
	if count != 2 {
		t.Fatalf("plain structs should not dedup: count=%d want 2", count)
	}
}

type ctxShape struct{ err error }

func (s *ctxShape) Close(context.Context) error { return s.err }

type noCtxShape struct{ err error }

func (s *noCtxShape) Close() error { return s.err }

type stopShape struct{ err error }

func (s *stopShape) Stop() error { return s.err }

type closeAndStop struct{ noCtxErr error }

func (s *closeAndStop) Close() error { return s.noCtxErr }
func (s *closeAndStop) Stop() error  { return errors.New("stop must not win over Close()") }

// shutdownShape implements ONLY Shutdown(context.Context) error, matching the
// shape observability.Provider (and its Tracer/Metrics sub-interfaces) has:
// no Close, no Stop.
type shutdownShape struct{ err error }

func (s *shutdownShape) Shutdown(context.Context) error { return s.err }

// closeAndShutdown has both Close() error and Shutdown(ctx) error to assert
// Close() still wins over the newer Shutdown probe.
type closeAndShutdown struct{ closeErr error }

func (s *closeAndShutdown) Close() error { return s.closeErr }
func (s *closeAndShutdown) Shutdown(context.Context) error {
	return errors.New("shutdown must not win over Close()")
}

func TestContainer_CloseAny_Shapes(t *testing.T) {
	ctx := t.Context()
	sentinel := errors.New("sentinel")
	tests := []struct {
		name    string
		v       any
		wantErr bool
	}{
		{"close ctx nil", &ctxShape{}, false},
		{"close ctx err", &ctxShape{err: sentinel}, true},
		{"close no ctx nil", &noCtxShape{}, false},
		{"close no ctx err", &noCtxShape{err: sentinel}, true},
		{"stop nil", &stopShape{}, false},
		{"stop err", &stopShape{err: sentinel}, true},
		{"shutdown nil", &shutdownShape{}, false},
		{"shutdown err", &shutdownShape{err: sentinel}, true},
		{"none", &noCloser{}, false},
		{"nil", nil, false},
		{"string", "not a closer", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := closeAny(ctx, tt.v)
			if tt.wantErr && err == nil {
				t.Fatal("want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("want nil, got %v", err)
			}
			if tt.wantErr && !errors.Is(err, sentinel) {
				t.Fatalf("want sentinel, got %v", err)
			}
		})
	}
}

func TestContainer_CloseAny_Precedence(t *testing.T) {
	ctx := t.Context()
	sentinel := errors.New("close wins")
	cs := &closeAndStop{noCtxErr: sentinel}
	if err := closeAny(ctx, cs); !errors.Is(err, sentinel) {
		t.Fatalf("Close() should win over Stop(), got %v", err)
	}
}

func TestContainer_CloseAny_ClosePrecedenceOverShutdown(t *testing.T) {
	ctx := t.Context()
	sentinel := errors.New("close wins over shutdown")
	cs := &closeAndShutdown{closeErr: sentinel}
	if err := closeAny(ctx, cs); !errors.Is(err, sentinel) {
		t.Fatalf("Close() should win over Shutdown(ctx), got %v", err)
	}
}

// TestContainer_Close_ShutdownOnlyServiceInvoked is the regression test for
// the bug where closeAny silently no-oped for any service whose only
// shutdown shape is Shutdown(context.Context) error (observability.Provider
// and its Tracer/Metrics sub-interfaces never implement Close or Stop).
// It seeds a synthetic fake implementing ONLY Shutdown into the
// observability lazy slot (whose field type is observability.Provider, so it
// still must satisfy Tracer/Meter, but deliberately has no Close/Stop) and
// asserts Container.Close actually invokes it.
func TestContainer_Close_ShutdownOnlyServiceInvoked(t *testing.T) {
	c := New(config.Default())
	fake := &shutdownOnlyProvider{}
	c.observability.val = fake
	c.observability.done = true
	c.observability.ready.Store(true)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := fake.shutdownCalls.Load(); got != 1 {
		t.Fatalf("Shutdown-only service closed %d times via Container.Close, want 1", got)
	}
}

// shutdownOnlyProvider implements observability.Provider (Tracer/Meter are
// required by the field's static type) but exposes no Close or Stop method
// anywhere in its method set, matching the real bug shape.
type shutdownOnlyProvider struct {
	shutdownCalls atomic.Int32
}

func (p *shutdownOnlyProvider) Tracer(string) observability.Tracer { return nil }
func (p *shutdownOnlyProvider) Meter(string) observability.Metrics { return nil }
func (p *shutdownOnlyProvider) Shutdown(context.Context) error {
	p.shutdownCalls.Add(1)
	return nil
}

// trackedNoopProvider wraps the real observability/noop.Provider, delegating
// Tracer/Meter and Shutdown to it while counting Shutdown invocations, so
// the test can observe whether Container.Close actually reached the
// underlying real service's Shutdown method (the noop provider itself has no
// externally observable side effect to assert against).
type trackedNoopProvider struct {
	observability.Provider
	shutdownCalls atomic.Int32
}

func (p *trackedNoopProvider) Shutdown(ctx context.Context) error {
	p.shutdownCalls.Add(1)
	return p.Provider.Shutdown(ctx)
}

// TestContainer_Close_ObservabilityProviderShutdownFlushed exercises the
// actual affected service (observability.Provider, backed by the real
// observability/noop adapter) through the real Container.Close path, not
// just a synthetic fake, confirming the fix closes the real bug end-to-end.
func TestContainer_Close_ObservabilityProviderShutdownFlushed(t *testing.T) {
	c := New(config.Default())
	tracked := &trackedNoopProvider{Provider: observabilitynoop.New()}
	c.observability.val = tracked
	c.observability.done = true
	c.observability.ready.Store(true)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := tracked.shutdownCalls.Load(); got != 1 {
		t.Fatalf("observability provider Shutdown called %d times via Container.Close, want 1", got)
	}
}

func TestContainer_Close_TimeoutError(t *testing.T) {
	c := New(config.Default())
	slow := &fakeCtx{name: "slow", block: 500 * time.Millisecond, ignoreCtx: true}
	c.db.val = slow
	c.db.done = true
	c.db.ready.Store(true)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err := c.Close(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	var timeoutErr CloseTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("want CloseTimeoutError, got %T: %v", err, err)
	}
	if timeoutErr.Service != "db" {
		t.Fatalf("Service = %q, want db", timeoutErr.Service)
	}
	// Wait for the ignored-ctx goroutine to finish its block so no
	// goroutine leaks past the test (goleak): poll the completion event
	// instead of sleeping past the 500ms block.
	waitForCloseCalls(t, slow, 2*time.Second)
}

func TestContainer_Close_DerivedTimeoutCancel(t *testing.T) {
	c := New(config.Default())
	// Shrunk from 2s to 500ms: the block only needs to outlive the
	// wait-for-start + cancel below so Close is still in flight when the
	// parent is canceled; the timeout branch under test is unchanged.
	slow := &fakeCtx{name: "slow-derived", block: 500 * time.Millisecond, ignoreCtx: true}
	c.db.val = slow
	c.db.done = true
	c.db.ready.Store(true)

	// No parent deadline, so Close derives a 5s per-service timeout
	// (cancel != nil). Canceling the parent aborts the derived context
	// through the timeout branch with a non-nil cancel.
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- c.Close(ctx) }()
	// Event wait: cancel only once Close is provably in flight.
	waitForCloseStarted(t, slow, 2*time.Second)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		var timeoutErr CloseTimeoutError
		if !errors.As(err, &timeoutErr) {
			t.Fatalf("want CloseTimeoutError, got %T: %v", err, err)
		}
		if timeoutErr.Service != "db" {
			t.Fatalf("Service = %q, want db", timeoutErr.Service)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close hung after parent cancel")
	}
	// Wait for the ignored-ctx goroutine to finish its block so no
	// goroutine leaks past the test (goleak): poll the completion event
	// instead of sleeping past the 500ms block.
	waitForCloseCalls(t, slow, 2*time.Second)
}

func TestContainer_Close_PanicError(t *testing.T) {
	c := New(config.Default())
	p := &fakeCtx{name: "panicking", panicOnClose: true}
	c.db.val = p
	c.db.done = true
	c.db.ready.Store(true)

	err := c.Close(t.Context())
	if err == nil {
		t.Fatal("expected panic error, got nil")
	}
	var panicErr ClosePanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("want ClosePanicError, got %T: %v", err, err)
	}
	if panicErr.Service != "db" {
		t.Fatalf("Service = %q, want db", panicErr.Service)
	}
	if !strings.Contains(err.Error(), "db") {
		t.Fatalf("error should name service, got: %v", err)
	}
}

func TestContainer_Close_ErrorsJoinServices(t *testing.T) {
	c := New(config.Default())
	e1 := errors.New("boom-a")
	e2 := errors.New("boom-b")
	a := &fakeCtx{name: "a", closeErr: e1}
	b := &fakeQ{name: "b"}
	c.db.val = a
	c.db.done = true
	c.db.ready.Store(true)
	c.queue.val = b
	c.queue.done = true
	c.queue.ready.Store(true)

	// Force the queue path to fail by replacing with a failing ctx closer
	// on another snapshot field instead: use auth field via reflection-free
	// seeding through db/cache only would single-fail, so seed a second
	// failing ctx service on cache.
	c.cache.val = &fakeCtx{name: "cache-fail", closeErr: e2}
	c.cache.done = true
	c.cache.ready.Store(true)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := c.Close(ctx)
	if err == nil {
		t.Fatal("expected joined errors, got nil")
	}
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Fatalf("joined error must wrap both causes, got: %v", err)
	}
	if !strings.Contains(err.Error(), "db") || !strings.Contains(err.Error(), "cache") {
		t.Fatalf("joined error should name services, got: %v", err)
	}
}

func TestContainer_Close_GRPCGracefulStop(t *testing.T) {
	c := New(config.Default())
	if _, err := c.GRPC(); err != nil {
		t.Fatalf("GRPC: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close with fresh gRPC server: %v", err)
	}
}

func TestContainer_Close_GRPCCanceledCtx(t *testing.T) {
	c := New(config.Default())
	if _, err := c.GRPC(); err != nil {
		t.Fatalf("GRPC: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan error, 1)
	go func() { done <- c.Close(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close with canceled ctx: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close with canceled ctx hung")
	}
}
