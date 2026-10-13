package container

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/cdn"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

// errFakeUnimplemented marks fakeCtx methods that tests never invoke.
var errFakeUnimplemented = errors.New("fakeCtx: unimplemented")

// registerTestAdapters wires the shared zero-value fakes (defined in
// services_test.go) under the default adapter names. Adapter registries are
// global; duplicates from parallel helpers are ignored. Nothing here may
// import nested adapter modules: the container resolves only caller-side
// registrations, and tests stand in for the caller with fakes.
var registerAdaptersOnce sync.Once

func registerTestAdapters() {
	registerAdaptersOnce.Do(func() {
		_ = cache.Register(cache.Memory, newFakeCache)
		_ = db.Register(db.SQLite, newFakeDB)
		_ = queue.Register(queue.Memory, newFakeQueue)
		_ = scheduler.Register(scheduler.Embedded, newFakeScheduler)
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
		name := typ.Field(i).Name
		if name == "cfg" || name == "pluginsMu" || name == "plugins" || name == "pools" || name == "rootCtx" || name == "cancelRoot" {
			continue
		}
		totalLazy++
	}
	const orderedExclusions = 9 // cache, queue, db, scheduler, job, agent, rag, grpcServer, grpcClients
	snaps := c.snapshots()
	if got := len(snaps) + orderedExclusions; got != totalLazy {
		t.Fatalf("snapshots drift: len(snapshots)=%d + ordered %d = %d, want %d lazy fields", len(snaps), orderedExclusions, got, totalLazy)
	}
	names := snapshotServiceNames()
	if len(names) != len(snaps) {
		t.Fatalf("service names drift: names=%d snapshots=%d", len(names), len(snaps))
	}
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		if n == "" {
			t.Fatal("empty service name in snapshotServiceNames")
		}
		if _, dup := seen[n]; dup {
			t.Fatalf("duplicate service name %q in snapshotServiceNames", n)
		}
		seen[n] = struct{}{}
	}
	for i, s := range snaps {
		if s.ok {
			t.Fatalf("fresh container: snapshot %d (%s) must be unresolved", i, names[i])
		}
	}
}

func TestContainer_Close_OnlyResolvedClosed(t *testing.T) {
	registerTestAdapters()
	cfg := config.Default()
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
	sPos, dPos, cPos, qPos := pos("scheduler"), pos("other"), pos("cache"), pos("queue")
	if sPos == -1 || dPos == -1 || cPos == -1 || qPos == -1 {
		t.Fatalf("missing closes in order %v", order)
	}
	if sPos >= cPos || sPos >= qPos || sPos >= dPos {
		t.Fatalf("scheduler should close before cache/queue/db: order %v", order)
	}
	if cPos >= dPos || qPos >= dPos {
		t.Fatalf("db should close after cache/queue (registry band): order %v", order)
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

// sharedEmbedResource stands in for a shared logger: one pointer embedded by
// two distinct service structs.
type sharedEmbedResource struct{}

// embedCDN is a struct-value service embedding a shared pointer. It implements
// cdn.CDN with value receivers so the container stores the struct value
// itself, not a pointer to it.
type embedCDN struct {
	shared *sharedEmbedResource
	calls  *atomic.Int32
}

func (s embedCDN) Purge(context.Context, cdn.PurgeRequest) error { return nil }
func (s embedCDN) Name() string                                  { return "embed" }
func (s embedCDN) Close(context.Context) error {
	s.calls.Add(1)
	return nil
}

// embedI18n is a second struct-value service type embedding the same shared
// pointer, implementing i18n.I18n.
type embedI18n struct {
	shared *sharedEmbedResource
	calls  *atomic.Int32
}

func (s embedI18n) Translate(context.Context, string, string, map[string]string) (string, error) {
	return "", nil
}
func (s embedI18n) Locales(context.Context) ([]string, error) { return nil, nil }
func (s embedI18n) Close() error {
	s.calls.Add(1)
	return nil
}

// TestContainer_Close_DedupSharedEmbeddedPointer is the P1-3 regression test.
// Two distinct services whose struct values embed the same pointer must both
// close: deduping on the embedded pointer would skip the second service.
func TestContainer_Close_DedupSharedEmbeddedPointer(t *testing.T) {
	c := New(config.Default())
	shared := &sharedEmbedResource{}
	var callsA, callsB atomic.Int32
	c.cdn.val = embedCDN{shared: shared, calls: &callsA}
	c.cdn.done = true
	c.cdn.ready.Store(true)
	c.i18n.val = embedI18n{shared: shared, calls: &callsB}
	c.i18n.done = true
	c.i18n.ready.Store(true)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := callsA.Load(); got != 1 {
		t.Fatalf("service A closed %d times, want 1", got)
	}
	if got := callsB.Load(); got != 1 {
		t.Fatalf("service B closed %d times, want 1 (shared embedded pointer must not dedup distinct services)", got)
	}
}

// TestContainer_Close_DedupStructValuesNeverDedup pins the P1-3 fix: struct
// values never dedupe, even when they embed the same pointer. Only the
// service value's own reference identity dedupes.
func TestContainer_Close_DedupStructValuesNeverDedup(t *testing.T) {
	shared := &sharedEmbedResource{}
	if _, ok := dedupKey(embedCDN{shared: shared}); ok {
		t.Fatal("struct value service must not dedupe, even embedding a pointer")
	}
	if _, ok := dedupKey(embedI18n{shared: shared}); ok {
		t.Fatal("second struct value type must not dedupe either")
	}
	type plain struct {
		N int
	}
	if _, ok := dedupKey(plain{N: 1}); ok {
		t.Fatal("plain struct must not dedupe")
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

// trackedNoopProvider wraps a fake observability.Provider, delegating
// Tracer/Meter and Shutdown to it while counting Shutdown invocations, so
// the test can observe whether Container.Close actually reached the
// underlying service's Shutdown method (the fake itself has no
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
// actual affected service (observability.Provider, backed by a fake provider
// implementing the full Provider shape) through the real Container.Close
// path, confirming the fix closes the real bug end-to-end.
func TestContainer_Close_ObservabilityProviderShutdownFlushed(t *testing.T) {
	c := New(config.Default())
	tracked := &trackedNoopProvider{Provider: &fakeObservability{}}
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

// recordingLogger captures emitted messages so tests can assert the
// container reported an abandoned close goroutine through its logger.
type recordingLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (r *recordingLogger) warn(msg string) {
	r.mu.Lock()
	r.msgs = append(r.msgs, msg)
	r.mu.Unlock()
}

func (r *recordingLogger) warnings() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.msgs...)
}

func (r *recordingLogger) Debug() log.Event                         { return &recordingEvent{r} }
func (r *recordingLogger) Info() log.Event                          { return &recordingEvent{r} }
func (r *recordingLogger) Warn() log.Event                          { return &recordingEvent{r} }
func (r *recordingLogger) Error() log.Event                         { return &recordingEvent{r} }
func (r *recordingLogger) Fatal() log.Event                         { return &recordingEvent{r} }
func (r *recordingLogger) With() log.Context                        { return &recordingContext{r} }
func (r *recordingLogger) WithContext(_ context.Context) log.Logger { return r }
func (r *recordingLogger) Enabled(_ log.Level) bool                 { return true }
func (r *recordingLogger) Sync() error                              { return nil }
func (r *recordingLogger) Name() string                             { return "recording" }

type recordingEvent struct{ r *recordingLogger }

func (e *recordingEvent) Str(_, _ string) log.Event               { return e }
func (e *recordingEvent) Int(_ string, _ int) log.Event           { return e }
func (e *recordingEvent) Int64(_ string, _ int64) log.Event       { return e }
func (e *recordingEvent) Float64(_ string, _ float64) log.Event   { return e }
func (e *recordingEvent) Bool(_ string, _ bool) log.Event         { return e }
func (e *recordingEvent) Dur(_ string, _ time.Duration) log.Event { return e }
func (e *recordingEvent) Time(_ string, _ time.Time) log.Event    { return e }
func (e *recordingEvent) Err(_ error) log.Event                   { return e }
func (e *recordingEvent) AnErr(_ string, _ error) log.Event       { return e }
func (e *recordingEvent) Any(_ string, _ any) log.Event           { return e }
func (e *recordingEvent) Msg(msg string)                          { e.r.warn(msg) }
func (e *recordingEvent) Msgf(format string, args ...any)         { e.r.warn(fmt.Sprintf(format, args...)) }
func (e *recordingEvent) Send()                                   {}

type recordingContext struct{ r *recordingLogger }

func (c *recordingContext) Str(_, _ string) log.Context               { return c }
func (c *recordingContext) Int(_ string, _ int) log.Context           { return c }
func (c *recordingContext) Int64(_ string, _ int64) log.Context       { return c }
func (c *recordingContext) Float64(_ string, _ float64) log.Context   { return c }
func (c *recordingContext) Bool(_ string, _ bool) log.Context         { return c }
func (c *recordingContext) Dur(_ string, _ time.Duration) log.Context { return c }
func (c *recordingContext) Time(_ string, _ time.Time) log.Context    { return c }
func (c *recordingContext) Err(_ error) log.Context                   { return c }
func (c *recordingContext) AnErr(_ string, _ error) log.Context       { return c }
func (c *recordingContext) Any(_ string, _ any) log.Context           { return c }
func (c *recordingContext) Logger() log.Logger                        { return c.r }

// TestContainer_Close_TimeoutError_Abandoned is the P1-5 test: a service
// whose Close ignores ctx times out, the closeAsync goroutine is abandoned,
// and the resulting CloseTimeoutError reports Abandoned=true with the
// service name; the container logger is warned about the abandoned goroutine.
func TestContainer_Close_TimeoutError_Abandoned(t *testing.T) {
	c := New(config.Default())
	slow := &fakeCtx{name: "slow", block: 500 * time.Millisecond, ignoreCtx: true}
	c.db.val = slow
	c.db.done = true
	c.db.ready.Store(true)

	rec := &recordingLogger{}
	c.log.val = rec
	c.log.done = true
	c.log.ready.Store(true)

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
	if !timeoutErr.Abandoned {
		t.Fatal("Abandoned = false, want true when the closeAsync goroutine is abandoned on timeout")
	}
	if timeoutErr.Service != "db" {
		t.Fatalf("Service = %q, want db", timeoutErr.Service)
	}

	// The warn is emitted synchronously on Close's goroutine before it
	// returns, so by now the recording logger must hold exactly one warning
	// naming the abandoned service.
	msgs := rec.warnings()
	if len(msgs) != 1 {
		t.Fatalf("warnings = %v, want exactly one", msgs)
	}
	if !strings.Contains(msgs[0], "db") {
		t.Fatalf("warning %q should name the abandoned service", msgs[0])
	}

	// Wait for the ignored-ctx goroutine to finish its block so no
	// goroutine leaks past the test (goleak): poll the completion event
	// instead of sleeping past the 500ms block.
	waitForCloseCalls(t, slow, 3*time.Second)
}

// TestContainer_Close_TimeoutError_AbandonedNoLogger pins that a timed-out
// close still reports Abandoned=true when no logger is resolved: the warn
// is best-effort and must never block or panic Close.
func TestContainer_Close_TimeoutError_AbandonedNoLogger(t *testing.T) {
	c := New(config.Default())
	slow := &fakeCtx{name: "slow-nolog", block: 500 * time.Millisecond, ignoreCtx: true}
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
	if !timeoutErr.Abandoned {
		t.Fatal("Abandoned = false, want true even without a resolved logger")
	}
	if timeoutErr.Service != "db" {
		t.Fatalf("Service = %q, want db", timeoutErr.Service)
	}

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
