package cachetest

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

// stubCache is a scriptable cache.Cache double with a small in-memory
// record map and TTL expiry. Per-method error fields drive each
// conformance failure branch.
type stubCache struct {
	mu sync.Mutex

	records   map[string]stubEntry
	getErr    map[string]error
	setErr    error
	setErrKey map[string]error
	siaErr    error
	delErr    error
	incErr    error
	incErrKey map[string]error
	decErr    error
	decErrKey map[string]error
	existsErr error
	closeErr  error
	closeAg   error
	closes    int
	closed    bool
	// neverExpire disables TTL expiry so polls time out.
	neverExpire bool
	// siaDeny forces SetIfAbsent to report present.
	siaDeny bool
}

type stubEntry struct {
	val []byte
	exp time.Time
}

func healthyStubCache() *stubCache {
	return &stubCache{records: map[string]stubEntry{}, getErr: map[string]error{}}
}

func (s *stubCache) live(key string) ([]byte, bool) {
	e, ok := s.records[key]
	if !ok {
		return nil, false
	}
	if !e.exp.IsZero() && !time.Now().Before(e.exp) && !s.neverExpire {
		delete(s.records, key)
		return nil, false
	}
	return e.val, true
}

func (s *stubCache) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, cache.ErrClosed
	}
	if err, ok := s.getErr[key]; ok && err != nil {
		return nil, err
	}
	if v, ok := s.live(key); ok {
		return append([]byte(nil), v...), nil
	}
	return nil, cache.NotFoundError{Key: key}
}

func (s *stubCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return cache.ErrClosed
	}
	if s.setErr != nil {
		return s.setErr
	}
	if err, ok := s.setErrKey[key]; ok && err != nil {
		return err
	}
	e := stubEntry{val: append([]byte(nil), value...)}
	if ttl > 0 {
		e.exp = time.Now().Add(ttl)
	}
	s.records[key] = e
	return nil
}

func (s *stubCache) SetIfAbsent(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, cache.ErrClosed
	}
	if s.siaErr != nil {
		return false, s.siaErr
	}
	if s.siaDeny {
		return false, nil
	}
	if _, ok := s.live(key); ok {
		return false, nil
	}
	e := stubEntry{val: append([]byte(nil), value...)}
	if ttl > 0 {
		e.exp = time.Now().Add(ttl)
	}
	s.records[key] = e
	return true, nil
}

func (s *stubCache) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return cache.ErrClosed
	}
	if s.delErr != nil {
		return s.delErr
	}
	delete(s.records, key)
	return nil
}

func counterOp(val []byte) (int64, error) {
	n, err := strconv.ParseInt(string(val), 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *stubCache) Increment(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return cache.ErrClosed
	}
	if s.incErr != nil {
		return s.incErr
	}
	if err, ok := s.incErrKey[key]; ok && err != nil {
		return err
	}
	v, ok := s.live(key)
	if !ok {
		s.records[key] = stubEntry{val: []byte("1")}
		return nil
	}
	n, err := counterOp(v)
	if err != nil {
		return cache.InvalidValueError{Key: key, Err: err}
	}
	s.records[key] = stubEntry{val: []byte(strconv.FormatInt(n+1, 10))}
	return nil
}

func (s *stubCache) Decrement(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return cache.ErrClosed
	}
	if s.decErr != nil {
		return s.decErr
	}
	if err, ok := s.decErrKey[key]; ok && err != nil {
		return err
	}
	v, ok := s.live(key)
	if !ok {
		s.records[key] = stubEntry{val: []byte("-1")}
		return nil
	}
	n, err := counterOp(v)
	if err != nil {
		return cache.InvalidValueError{Key: key, Err: err}
	}
	s.records[key] = stubEntry{val: []byte(strconv.FormatInt(n-1, 10))}
	return nil
}

func (s *stubCache) Exists(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, cache.ErrClosed
	}
	if s.existsErr != nil {
		return false, s.existsErr
	}
	_, ok := s.live(key)
	return ok, nil
}

func (s *stubCache) Close(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closes++
	if s.closes > 1 {
		return s.closeAg
	}
	if s.closeErr != nil {
		return s.closeErr
	}
	s.closed = true
	return nil
}

func mustPass(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("check err = %v, want nil", err)
	}
}

func TestCheckGetSetSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkGetSet(t.Context(), healthyStubCache()))
}

func TestCheckGetSetFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("cachetest: boom")

	t.Run("missing found", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.records["missing"] = stubEntry{val: []byte("x")}

		err := checkGetSet(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Get(missing)") {
			t.Fatalf("checkGetSet() = %v, want Get(missing) error", err)
		}
	})

	t.Run("not NotFoundError", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.getErr["missing"] = boom

		err := checkGetSet(t.Context(), stub)
		if err == nil {
			t.Fatal("checkGetSet() = nil, want error")
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		t.Parallel()

		stub := &wrongKeyCache{stub: healthyStubCache()}

		err := checkGetSet(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "NotFoundError.Key") {
			t.Fatalf("checkGetSet() = %v, want Key error", err)
		}
	})

	t.Run("set error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.setErr = boom

		if err := checkGetSet(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkGetSet() = %v, want wrap of boom", err)
		}
	})

	t.Run("stored alias", func(t *testing.T) {
		t.Parallel()

		stub := &aliasStoreCache{stub: healthyStubCache()}

		err := checkGetSet(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "stored copy") {
			t.Fatalf("checkGetSet() = %v, want stored-copy error", err)
		}
	})

	t.Run("returned alias", func(t *testing.T) {
		t.Parallel()

		stub := &aliasReturnCache{stub: healthyStubCache()}

		err := checkGetSet(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "returned copy") {
			t.Fatalf("checkGetSet() = %v, want returned-copy error", err)
		}
	})

	t.Run("overwrite mismatch", func(t *testing.T) {
		t.Parallel()

		stub := &frozenCache{stub: healthyStubCache()}

		err := checkGetSet(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "want v2") {
			t.Fatalf("checkGetSet() = %v, want v2 error", err)
		}
	})
}

// wrongKeyCache reports the wrong key in NotFoundError.
type wrongKeyCache struct {
	stub *stubCache
}

func (w *wrongKeyCache) Get(ctx context.Context, key string) ([]byte, error) {
	if key == "missing" {
		return nil, cache.NotFoundError{Key: "other"}
	}
	return w.stub.Get(ctx, key)
}

func (w *wrongKeyCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return w.stub.Set(ctx, k, v, ttl)
}

func (w *wrongKeyCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return w.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (w *wrongKeyCache) Delete(ctx context.Context, k string) error {
	return w.stub.Delete(ctx, k)
}

func (w *wrongKeyCache) Increment(ctx context.Context, k string) error {
	return w.stub.Increment(ctx, k)
}

func (w *wrongKeyCache) Decrement(ctx context.Context, k string) error {
	return w.stub.Decrement(ctx, k)
}

func (w *wrongKeyCache) Exists(ctx context.Context, k string) (bool, error) {
	return w.stub.Exists(ctx, k)
}

func (w *wrongKeyCache) Close(ctx context.Context) error { return w.stub.Close(ctx) }

// aliasStoreCache keeps the caller slice so later mutation is visible.
type aliasStoreCache struct {
	stub *stubCache
}

func (a *aliasStoreCache) Get(ctx context.Context, k string) ([]byte, error) {
	return a.stub.Get(ctx, k)
}

func (a *aliasStoreCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	a.stub.mu.Lock()
	defer a.stub.mu.Unlock()

	a.stub.records[k] = stubEntry{val: v}
	return nil
}

func (a *aliasStoreCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return a.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (a *aliasStoreCache) Delete(ctx context.Context, k string) error {
	return a.stub.Delete(ctx, k)
}

func (a *aliasStoreCache) Increment(ctx context.Context, k string) error {
	return a.stub.Increment(ctx, k)
}

func (a *aliasStoreCache) Decrement(ctx context.Context, k string) error {
	return a.stub.Decrement(ctx, k)
}

func (a *aliasStoreCache) Exists(ctx context.Context, k string) (bool, error) {
	return a.stub.Exists(ctx, k)
}

func (a *aliasStoreCache) Close(ctx context.Context) error { return a.stub.Close(ctx) }

// aliasReturnCache returns the live slice so caller mutation persists.
type aliasReturnCache struct {
	stub *stubCache
}

func (a *aliasReturnCache) Get(_ context.Context, k string) ([]byte, error) {
	a.stub.mu.Lock()
	defer a.stub.mu.Unlock()

	e, ok := a.stub.records[k]
	if !ok {
		return nil, cache.NotFoundError{Key: k}
	}
	return e.val, nil
}

func (a *aliasReturnCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return a.stub.Set(ctx, k, v, ttl)
}

func (a *aliasReturnCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return a.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (a *aliasReturnCache) Delete(ctx context.Context, k string) error {
	return a.stub.Delete(ctx, k)
}

func (a *aliasReturnCache) Increment(ctx context.Context, k string) error {
	return a.stub.Increment(ctx, k)
}

func (a *aliasReturnCache) Decrement(ctx context.Context, k string) error {
	return a.stub.Decrement(ctx, k)
}

func (a *aliasReturnCache) Exists(ctx context.Context, k string) (bool, error) {
	return a.stub.Exists(ctx, k)
}

func (a *aliasReturnCache) Close(ctx context.Context) error { return a.stub.Close(ctx) }

// frozenCache ignores overwrites so the v2 assertion fails.
type frozenCache struct {
	stub *stubCache
	set  bool
}

func (f *frozenCache) Get(ctx context.Context, k string) ([]byte, error) {
	return f.stub.Get(ctx, k)
}

func (f *frozenCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	if f.set {
		return nil
	}
	f.set = true
	return f.stub.Set(ctx, k, v, ttl)
}

func (f *frozenCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return f.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (f *frozenCache) Delete(ctx context.Context, k string) error {
	return f.stub.Delete(ctx, k)
}

func (f *frozenCache) Increment(ctx context.Context, k string) error {
	return f.stub.Increment(ctx, k)
}

func (f *frozenCache) Decrement(ctx context.Context, k string) error {
	return f.stub.Decrement(ctx, k)
}

func (f *frozenCache) Exists(ctx context.Context, k string) (bool, error) {
	return f.stub.Exists(ctx, k)
}

func (f *frozenCache) Close(ctx context.Context) error { return f.stub.Close(ctx) }

func TestCheckTTLExpirySuccess(t *testing.T) {
	t.Parallel()

	stub := healthyStubCache()
	mustPass(t, checkTTLExpiry(t.Context(), stub, 2*time.Second))
}

func TestCheckTTLExpiryFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("cachetest: boom")

	t.Run("set error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.setErr = boom

		if err := checkTTLExpiry(t.Context(), stub, time.Second); !errors.Is(err, boom) {
			t.Fatalf("checkTTLExpiry() = %v, want wrap of boom", err)
		}
	})

	t.Run("never expires", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.neverExpire = true

		err := checkTTLExpiry(t.Context(), stub, 60*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "condition not met") {
			t.Fatalf("checkTTLExpiry() = %v, want poll-timeout error", err)
		}
	})

	t.Run("keep lost", func(t *testing.T) {
		t.Parallel()

		stub := &dropKeepCache{stub: healthyStubCache()}

		err := checkTTLExpiry(t.Context(), stub, 2*time.Second)
		if err == nil || !strings.Contains(err.Error(), "retained") {
			t.Fatalf("checkTTLExpiry() = %v, want retained error", err)
		}
	})
}

// dropKeepCache drops the "keep" key so the no-ttl assertion fails.
type dropKeepCache struct {
	stub *stubCache
}

func (d *dropKeepCache) Get(ctx context.Context, k string) ([]byte, error) {
	return d.stub.Get(ctx, k)
}

func (d *dropKeepCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	if k == "keep" {
		return nil
	}
	return d.stub.Set(ctx, k, v, ttl)
}

func (d *dropKeepCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return d.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (d *dropKeepCache) Delete(ctx context.Context, k string) error {
	return d.stub.Delete(ctx, k)
}

func (d *dropKeepCache) Increment(ctx context.Context, k string) error {
	return d.stub.Increment(ctx, k)
}

func (d *dropKeepCache) Decrement(ctx context.Context, k string) error {
	return d.stub.Decrement(ctx, k)
}

func (d *dropKeepCache) Exists(ctx context.Context, k string) (bool, error) {
	return d.stub.Exists(ctx, k)
}

func (d *dropKeepCache) Close(ctx context.Context) error { return d.stub.Close(ctx) }

func TestCheckSetIfAbsentSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkSetIfAbsent(t.Context(), healthyStubCache(), 2*time.Second))
}

func TestCheckSetIfAbsentFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("cachetest: boom")

	t.Run("first error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.siaErr = boom

		err := checkSetIfAbsent(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "SetIfAbsent() =") {
			t.Fatalf("checkSetIfAbsent() = %v, want first-write error", err)
		}
	})

	t.Run("always absent", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.siaDeny = true

		err := checkSetIfAbsent(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "SetIfAbsent() =") {
			t.Fatalf("checkSetIfAbsent() = %v, want first-write error", err)
		}
	})

	t.Run("never expires for sia", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.neverExpire = true

		err := checkSetIfAbsent(t.Context(), stub, 60*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "condition not met") {
			t.Fatalf("checkSetIfAbsent() = %v, want poll-timeout error", err)
		}
	})

	t.Run("second wins", func(t *testing.T) {
		t.Parallel()

		stub := &overwriteSiaCache{stub: healthyStubCache()}

		err := checkSetIfAbsent(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "second") {
			t.Fatalf("checkSetIfAbsent() = %v, want second-write error", err)
		}
	})

	t.Run("wrong value", func(t *testing.T) {
		t.Parallel()

		stub := &wrongSiaValueCache{stub: healthyStubCache()}

		err := checkSetIfAbsent(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "want v1") {
			t.Fatalf("checkSetIfAbsent() = %v, want v1 error", err)
		}
	})

	t.Run("get new error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.getErr["e"] = errors.New("cachetest: e boom")

		err := checkSetIfAbsent(t.Context(), stub, 3*time.Second)
		if err == nil || !strings.Contains(err.Error(), "Get() error") {
			t.Fatalf("checkSetIfAbsent() = %v, want Get error", err)
		}
	})
}

// overwriteSiaCache lets the second SetIfAbsent win.
type overwriteSiaCache struct {
	stub *stubCache
}

func (o *overwriteSiaCache) Get(ctx context.Context, k string) ([]byte, error) {
	return o.stub.Get(ctx, k)
}

func (o *overwriteSiaCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return o.stub.Set(ctx, k, v, ttl)
}

func (o *overwriteSiaCache) SetIfAbsent(_ context.Context, k string, v []byte, _ time.Duration) (bool, error) {
	o.stub.mu.Lock()
	defer o.stub.mu.Unlock()

	o.stub.records[k] = stubEntry{val: append([]byte(nil), v...)}
	return true, nil
}

func (o *overwriteSiaCache) Delete(ctx context.Context, k string) error {
	return o.stub.Delete(ctx, k)
}

func (o *overwriteSiaCache) Increment(ctx context.Context, k string) error {
	return o.stub.Increment(ctx, k)
}

func (o *overwriteSiaCache) Decrement(ctx context.Context, k string) error {
	return o.stub.Decrement(ctx, k)
}

func (o *overwriteSiaCache) Exists(ctx context.Context, k string) (bool, error) {
	return o.stub.Exists(ctx, k)
}

func (o *overwriteSiaCache) Close(ctx context.Context) error { return o.stub.Close(ctx) }

// wrongSiaValueCache serves "v2" for "k".
type wrongSiaValueCache struct {
	stub *stubCache
}

func (w *wrongSiaValueCache) Get(ctx context.Context, k string) ([]byte, error) {
	if k == "k" {
		return []byte("v2"), nil
	}
	return w.stub.Get(ctx, k)
}

func (w *wrongSiaValueCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return w.stub.Set(ctx, k, v, ttl)
}

func (w *wrongSiaValueCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return w.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (w *wrongSiaValueCache) Delete(ctx context.Context, k string) error {
	return w.stub.Delete(ctx, k)
}

func (w *wrongSiaValueCache) Increment(ctx context.Context, k string) error {
	return w.stub.Increment(ctx, k)
}

func (w *wrongSiaValueCache) Decrement(ctx context.Context, k string) error {
	return w.stub.Decrement(ctx, k)
}

func (w *wrongSiaValueCache) Exists(ctx context.Context, k string) (bool, error) {
	return w.stub.Exists(ctx, k)
}

func (w *wrongSiaValueCache) Close(ctx context.Context) error { return w.stub.Close(ctx) }

func TestCheckDeleteSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkDelete(t.Context(), healthyStubCache()))
}

func TestCheckDeleteFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("cachetest: boom")

	t.Run("delete missing error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.delErr = boom

		if err := checkDelete(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkDelete() = %v, want wrap of boom", err)
		}
	})

	t.Run("set error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.setErr = boom

		if err := checkDelete(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkDelete() = %v, want wrap of boom", err)
		}
	})

	t.Run("kept after delete", func(t *testing.T) {
		t.Parallel()

		stub := &noDeleteCache{stub: healthyStubCache()}

		err := checkDelete(t.Context(), stub)
		if err == nil {
			t.Fatal("checkDelete(no-delete) = nil, want joined errors")
		}
		for _, want := range []string{"ErrNotFound", "Exists()", "Delete(again)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkDelete() = %v, want containing %q", err, want)
			}
		}
	})
}

// noDeleteCache keeps records on Delete and fails the last Delete.
type noDeleteCache struct {
	stub  *stubCache
	calls int
}

func (n *noDeleteCache) Get(ctx context.Context, k string) ([]byte, error) {
	return n.stub.Get(ctx, k)
}

func (n *noDeleteCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return n.stub.Set(ctx, k, v, ttl)
}

func (n *noDeleteCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return n.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (n *noDeleteCache) Delete(_ context.Context, _ string) error {
	n.calls++
	if n.calls == 3 {
		return errors.New("cachetest: delete-again boom")
	}
	return nil
}

func (n *noDeleteCache) Increment(ctx context.Context, k string) error {
	return n.stub.Increment(ctx, k)
}

func (n *noDeleteCache) Decrement(ctx context.Context, k string) error {
	return n.stub.Decrement(ctx, k)
}

func (n *noDeleteCache) Exists(ctx context.Context, k string) (bool, error) {
	return n.stub.Exists(ctx, k)
}

func (n *noDeleteCache) Close(ctx context.Context) error { return n.stub.Close(ctx) }

func TestCheckCountersSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkCounters(t.Context(), healthyStubCache()))
}

func TestCheckCountersFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("cachetest: boom")

	t.Run("increment error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.incErr = boom

		if err := checkCounters(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkCounters() = %v, want wrap of boom", err)
		}
	})

	t.Run("wrong start", func(t *testing.T) {
		t.Parallel()

		stub := &wrongStartCache{stub: healthyStubCache()}

		err := checkCounters(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), `want 1`) {
			t.Fatalf("checkCounters() = %v, want start-1 error", err)
		}
	})

	t.Run("bad value ok", func(t *testing.T) {
		t.Parallel()

		stub := &tolerantIncrementCache{stub: healthyStubCache()}

		err := checkCounters(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "ErrInvalidValue") {
			t.Fatalf("checkCounters() = %v, want invalid-value error", err)
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		t.Parallel()

		stub := &wrongCounterKeyCache{stub: healthyStubCache()}

		err := checkCounters(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "InvalidValueError.Key") {
			t.Fatalf("checkCounters() = %v, want key error", err)
		}
	})
}

// wrongStartCache starts counters at 2.
type wrongStartCache struct {
	stub *stubCache
}

func (w *wrongStartCache) Get(ctx context.Context, k string) ([]byte, error) {
	return w.stub.Get(ctx, k)
}

func (w *wrongStartCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return w.stub.Set(ctx, k, v, ttl)
}

func (w *wrongStartCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return w.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (w *wrongStartCache) Delete(ctx context.Context, k string) error {
	return w.stub.Delete(ctx, k)
}

func (w *wrongStartCache) Increment(_ context.Context, k string) error {
	w.stub.mu.Lock()
	defer w.stub.mu.Unlock()

	w.stub.records[k] = stubEntry{val: []byte("2")}
	return nil
}

func (w *wrongStartCache) Decrement(ctx context.Context, k string) error {
	return w.stub.Decrement(ctx, k)
}

func (w *wrongStartCache) Exists(ctx context.Context, k string) (bool, error) {
	return w.stub.Exists(ctx, k)
}

func (w *wrongStartCache) Close(ctx context.Context) error { return w.stub.Close(ctx) }

// tolerantIncrementCache accepts non-numeric increments.
type tolerantIncrementCache struct {
	stub *stubCache
}

func (t2 *tolerantIncrementCache) Get(ctx context.Context, k string) ([]byte, error) {
	return t2.stub.Get(ctx, k)
}

func (t2 *tolerantIncrementCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return t2.stub.Set(ctx, k, v, ttl)
}

func (t2 *tolerantIncrementCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return t2.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (t2 *tolerantIncrementCache) Delete(ctx context.Context, k string) error {
	return t2.stub.Delete(ctx, k)
}

func (t2 *tolerantIncrementCache) Increment(ctx context.Context, k string) error {
	if k == "bad" {
		return nil
	}
	return t2.stub.Increment(ctx, k)
}

func (t2 *tolerantIncrementCache) Decrement(ctx context.Context, k string) error {
	return t2.stub.Decrement(ctx, k)
}

func (t2 *tolerantIncrementCache) Exists(ctx context.Context, k string) (bool, error) {
	return t2.stub.Exists(ctx, k)
}

func (t2 *tolerantIncrementCache) Close(ctx context.Context) error { return t2.stub.Close(ctx) }

// wrongCounterKeyCache reports the wrong key in InvalidValueError.
type wrongCounterKeyCache struct {
	stub *stubCache
}

func (w *wrongCounterKeyCache) Get(ctx context.Context, k string) ([]byte, error) {
	return w.stub.Get(ctx, k)
}

func (w *wrongCounterKeyCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return w.stub.Set(ctx, k, v, ttl)
}

func (w *wrongCounterKeyCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return w.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (w *wrongCounterKeyCache) Delete(ctx context.Context, k string) error {
	return w.stub.Delete(ctx, k)
}

func (w *wrongCounterKeyCache) Increment(ctx context.Context, k string) error {
	if k == "bad" {
		return cache.InvalidValueError{Key: "other", Err: errors.New("cachetest: bad")}
	}
	return w.stub.Increment(ctx, k)
}

func (w *wrongCounterKeyCache) Decrement(ctx context.Context, k string) error {
	return w.stub.Decrement(ctx, k)
}

func (w *wrongCounterKeyCache) Exists(ctx context.Context, k string) (bool, error) {
	return w.stub.Exists(ctx, k)
}

func (w *wrongCounterKeyCache) Close(ctx context.Context) error { return w.stub.Close(ctx) }

func TestCheckCountersLateBranches(t *testing.T) {
	t.Parallel()

	boom := errors.New("cachetest: boom")

	cases := []struct {
		name     string
		setup    func(*stubCache)
		contains string
	}{
		{"second increment error", func(s *stubCache) { s.incErrKey = map[string]error{"n": boom} }, "Increment() error"},
		{"decrement error", func(s *stubCache) { s.decErrKey = map[string]error{"n": boom} }, "Decrement() error"},
		{"get n error", func(s *stubCache) { s.getErr["n"] = boom }, "Get() error"},
		{"fresh decrement error", func(s *stubCache) { s.decErrKey = map[string]error{"fresh": boom} }, "Decrement() error"},
		{"get fresh error", func(s *stubCache) { s.getErr["fresh"] = boom }, "Get() error"},
		{"set base error", func(s *stubCache) { s.setErrKey = map[string]error{"base": boom} }, "Set() error"},
		{"increment base error", func(s *stubCache) { s.incErrKey = map[string]error{"base": boom} }, "Increment() error"},
		{"get base error", func(s *stubCache) { s.getErr["base"] = boom }, "Get() error"},
		{"set bad error", func(s *stubCache) { s.setErrKey = map[string]error{"bad": boom} }, "Set() error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubCache()
			tc.setup(stub)

			err := checkCounters(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkCounters() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkCounters() = %v, want containing %q", err, tc.contains)
			}
		})
	}

	t.Run("soft mismatches join", func(t *testing.T) {
		t.Parallel()

		// Decrement that reports success without changing state leaves
		// n at 2 after the inc/dec pair, tripping the soft want-1 branch.
		stub := &noopDecrementCache{stub: healthyStubCache()}

		err := checkCounters(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "want 1") {
			t.Fatalf("checkCounters() = %v, want soft want-1 error", err)
		}
	})

	t.Run("fresh mismatch", func(t *testing.T) {
		t.Parallel()

		stub := &wrongFreshCache{stub: healthyStubCache()}

		err := checkCounters(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "missing base") {
			t.Fatalf("checkCounters() = %v, want missing-base error", err)
		}
	})

	t.Run("base mismatch", func(t *testing.T) {
		t.Parallel()

		stub := &noopBaseIncrementCache{stub: healthyStubCache()}

		err := checkCounters(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "want 42") {
			t.Fatalf("checkCounters() = %v, want want-42 error", err)
		}
	})

	t.Run("not InvalidValueError", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.incErrKey = map[string]error{"bad": boom}

		err := checkCounters(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "ErrInvalidValue") {
			t.Fatalf("checkCounters() = %v, want ErrInvalidValue error", err)
		}
	})
}

// noopDecrementCache reports Decrement success without changing state.
type noopDecrementCache struct {
	stub *stubCache
}

func (n *noopDecrementCache) Get(ctx context.Context, k string) ([]byte, error) {
	return n.stub.Get(ctx, k)
}

func (n *noopDecrementCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return n.stub.Set(ctx, k, v, ttl)
}

func (n *noopDecrementCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return n.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (n *noopDecrementCache) Delete(ctx context.Context, k string) error {
	return n.stub.Delete(ctx, k)
}

func (n *noopDecrementCache) Increment(ctx context.Context, k string) error {
	return n.stub.Increment(ctx, k)
}

func (n *noopDecrementCache) Decrement(context.Context, string) error { return nil }

func (n *noopDecrementCache) Exists(ctx context.Context, k string) (bool, error) {
	return n.stub.Exists(ctx, k)
}

func (n *noopDecrementCache) Close(ctx context.Context) error { return n.stub.Close(ctx) }

// wrongFreshCache reports "0" for the fresh counter.
type wrongFreshCache struct {
	stub *stubCache
}

func (w *wrongFreshCache) Get(ctx context.Context, k string) ([]byte, error) {
	if k == "fresh" {
		return []byte("0"), nil
	}
	return w.stub.Get(ctx, k)
}

func (w *wrongFreshCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return w.stub.Set(ctx, k, v, ttl)
}

func (w *wrongFreshCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return w.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (w *wrongFreshCache) Delete(ctx context.Context, k string) error {
	return w.stub.Delete(ctx, k)
}

func (w *wrongFreshCache) Increment(ctx context.Context, k string) error {
	return w.stub.Increment(ctx, k)
}

func (w *wrongFreshCache) Decrement(ctx context.Context, k string) error {
	return w.stub.Decrement(ctx, k)
}

func (w *wrongFreshCache) Exists(ctx context.Context, k string) (bool, error) {
	return w.stub.Exists(ctx, k)
}

func (w *wrongFreshCache) Close(ctx context.Context) error { return w.stub.Close(ctx) }

// noopBaseIncrementCache skips Increment for "base" only.
type noopBaseIncrementCache struct {
	stub *stubCache
}

func (n *noopBaseIncrementCache) Get(ctx context.Context, k string) ([]byte, error) {
	return n.stub.Get(ctx, k)
}

func (n *noopBaseIncrementCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return n.stub.Set(ctx, k, v, ttl)
}

func (n *noopBaseIncrementCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return n.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (n *noopBaseIncrementCache) Delete(ctx context.Context, k string) error {
	return n.stub.Delete(ctx, k)
}

func (n *noopBaseIncrementCache) Increment(ctx context.Context, k string) error {
	if k == "base" {
		return nil
	}
	return n.stub.Increment(ctx, k)
}

func (n *noopBaseIncrementCache) Decrement(ctx context.Context, k string) error {
	return n.stub.Decrement(ctx, k)
}

func (n *noopBaseIncrementCache) Exists(ctx context.Context, k string) (bool, error) {
	return n.stub.Exists(ctx, k)
}

func (n *noopBaseIncrementCache) Close(ctx context.Context) error { return n.stub.Close(ctx) }

func TestCheckExistsSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkExists(t.Context(), healthyStubCache(), 2*time.Second))
}

func TestCheckExistsFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("cachetest: boom")

	t.Run("missing present", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.records["missing"] = stubEntry{val: []byte("x")}

		err := checkExists(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "Exists() =") {
			t.Fatalf("checkExists() = %v, want Exists error", err)
		}
	})

	t.Run("exists error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.existsErr = boom

		// Exists(missing) errors, tripping the first branch.
		err := checkExists(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "Exists() =") {
			t.Fatalf("checkExists() = %v, want Exists error", err)
		}
	})

	t.Run("set error", func(t *testing.T) {
		t.Parallel()

		stub := &failNthSetCache{stub: healthyStubCache(), failOn: 1, err: boom}

		err := checkExists(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "Set() error") {
			t.Fatalf("checkExists() = %v, want Set error", err)
		}
	})

	t.Run("delete error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.delErr = boom

		err := checkExists(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "Delete() error") {
			t.Fatalf("checkExists() = %v, want Delete error", err)
		}
	})

	t.Run("still exists after delete", func(t *testing.T) {
		t.Parallel()

		stub := &stickyExistsCache{stub: healthyStubCache()}

		err := checkExists(t.Context(), stub, time.Second)
		if err == nil || !strings.Contains(err.Error(), "after delete") {
			t.Fatalf("checkExists() = %v, want after-delete error", err)
		}
	})

	t.Run("never expires", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.neverExpire = true

		err := checkExists(t.Context(), stub, 60*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "condition not met") {
			t.Fatalf("checkExists() = %v, want poll-timeout error", err)
		}
	})
}

// failNthSetCache fails the nth Set call.
type failNthSetCache struct {
	stub   *stubCache
	failOn int
	calls  int
	err    error
}

func (f *failNthSetCache) Get(ctx context.Context, k string) ([]byte, error) {
	return f.stub.Get(ctx, k)
}

func (f *failNthSetCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	f.calls++
	if f.calls == f.failOn {
		return f.err
	}
	return f.stub.Set(ctx, k, v, ttl)
}

func (f *failNthSetCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return f.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (f *failNthSetCache) Delete(ctx context.Context, k string) error {
	return f.stub.Delete(ctx, k)
}

func (f *failNthSetCache) Increment(ctx context.Context, k string) error {
	return f.stub.Increment(ctx, k)
}

func (f *failNthSetCache) Decrement(ctx context.Context, k string) error {
	return f.stub.Decrement(ctx, k)
}

func (f *failNthSetCache) Exists(ctx context.Context, k string) (bool, error) {
	return f.stub.Exists(ctx, k)
}

func (f *failNthSetCache) Close(ctx context.Context) error { return f.stub.Close(ctx) }

// stickyExistsCache always reports present for "k".
type stickyExistsCache struct {
	stub *stubCache
}

func (s *stickyExistsCache) Get(ctx context.Context, k string) ([]byte, error) {
	return s.stub.Get(ctx, k)
}

func (s *stickyExistsCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return s.stub.Set(ctx, k, v, ttl)
}

func (s *stickyExistsCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return s.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (s *stickyExistsCache) Delete(ctx context.Context, k string) error {
	return s.stub.Delete(ctx, k)
}

func (s *stickyExistsCache) Increment(ctx context.Context, k string) error {
	return s.stub.Increment(ctx, k)
}

func (s *stickyExistsCache) Decrement(ctx context.Context, k string) error {
	return s.stub.Decrement(ctx, k)
}

func (s *stickyExistsCache) Exists(ctx context.Context, k string) (bool, error) {
	if k == "k" {
		return true, nil
	}
	return s.stub.Exists(ctx, k)
}

func (s *stickyExistsCache) Close(ctx context.Context) error { return s.stub.Close(ctx) }

func TestCheckCloseSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkClose(t.Context(), healthyStubCache()))
}

func TestCheckCloseFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("cachetest: boom")

	t.Run("first close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubCache()
		stub.closeErr = boom

		if err := checkClose(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkClose() = %v, want wrap of boom", err)
		}
	})

	t.Run("post-close ops open", func(t *testing.T) {
		t.Parallel()

		stub := &neverClosesCache{stub: healthyStubCache()}

		err := checkClose(t.Context(), stub)
		if err == nil {
			t.Fatal("checkClose(never-closes) = nil, want ErrClosed errors")
		}
		for _, want := range []string{"Get()", "Set()", "SetIfAbsent()", "Delete()", "Increment()", "Decrement()", "Exists()"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkClose() = %v, want containing %q", err, want)
			}
		}
	})
}

// neverClosesCache reports nil from Close without marking closed.
type neverClosesCache struct {
	stub *stubCache
}

func (n *neverClosesCache) Get(ctx context.Context, k string) ([]byte, error) {
	return n.stub.Get(ctx, k)
}

func (n *neverClosesCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return n.stub.Set(ctx, k, v, ttl)
}

func (n *neverClosesCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return n.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (n *neverClosesCache) Delete(ctx context.Context, k string) error {
	return n.stub.Delete(ctx, k)
}

func (n *neverClosesCache) Increment(ctx context.Context, k string) error {
	return n.stub.Increment(ctx, k)
}

func (n *neverClosesCache) Decrement(ctx context.Context, k string) error {
	return n.stub.Decrement(ctx, k)
}

func (n *neverClosesCache) Exists(ctx context.Context, k string) (bool, error) {
	return n.stub.Exists(ctx, k)
}

func (n *neverClosesCache) Close(context.Context) error { return nil }

func TestPollExpiryTimeout(t *testing.T) {
	t.Parallel()

	err := pollExpiry(t.Context(), 40*time.Millisecond, "never true", func(context.Context) bool {
		return false
	})
	if err == nil || !strings.Contains(err.Error(), "condition not met") {
		t.Fatalf("pollExpiry() = %v, want timeout error", err)
	}

	mustPass(t, pollExpiry(t.Context(), time.Second, "at once", func(context.Context) bool {
		return true
	}))
}

func TestMaybeFastForward(t *testing.T) {
	t.Parallel()

	maybeFastForward(healthyStubCache())

	ff := &ffCache{stub: healthyStubCache()}
	maybeFastForward(ff)

	if ff.calls != 1 {
		t.Fatalf("FastForward calls = %d, want 1", ff.calls)
	}
}

// ffCache implements fastForwarder and counts advances.
type ffCache struct {
	stub  *stubCache
	calls int
}

func (f *ffCache) Get(ctx context.Context, k string) ([]byte, error) {
	return f.stub.Get(ctx, k)
}

func (f *ffCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	return f.stub.Set(ctx, k, v, ttl)
}

func (f *ffCache) SetIfAbsent(ctx context.Context, k string, v []byte, ttl time.Duration) (bool, error) {
	return f.stub.SetIfAbsent(ctx, k, v, ttl)
}

func (f *ffCache) Delete(ctx context.Context, k string) error {
	return f.stub.Delete(ctx, k)
}

func (f *ffCache) Increment(ctx context.Context, k string) error {
	return f.stub.Increment(ctx, k)
}

func (f *ffCache) Decrement(ctx context.Context, k string) error {
	return f.stub.Decrement(ctx, k)
}

func (f *ffCache) Exists(ctx context.Context, k string) (bool, error) {
	return f.stub.Exists(ctx, k)
}

func (f *ffCache) Close(ctx context.Context) error { return f.stub.Close(ctx) }

func (f *ffCache) FastForward(time.Duration) { f.calls++ }

func TestCheckConcurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := context.Background()

			if err := checkGetSet(ctx, healthyStubCache()); err != nil {
				t.Errorf("checkGetSet() = %v, want nil", err)
			}

			if err := checkDelete(ctx, healthyStubCache()); err != nil {
				t.Errorf("checkDelete() = %v, want nil", err)
			}

			if err := checkClose(ctx, healthyStubCache()); err != nil {
				t.Errorf("checkClose() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
