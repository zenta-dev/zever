package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/cache"
	cachememory "github.com/zenta-dev/zever/cache/memory"
	"github.com/zenta-dev/zever/log/noop"
	"github.com/zenta-dev/zever/queue"
	queuememory "github.com/zenta-dev/zever/queue/memory"
)

// helpers

type fakeCache struct {
	getFn         func(context.Context, string) ([]byte, error)
	setFn         func(context.Context, string, []byte, time.Duration) error
	setIfAbsentFn func(context.Context, string, []byte, time.Duration) (bool, error)
	deleteFn      func(context.Context, string) error
	incrementFn   func(context.Context, string) error
	decrementFn   func(context.Context, string) error
	existsFn      func(context.Context, string) (bool, error)
	closeFn       func(context.Context) error
}

func (f *fakeCache) Get(ctx context.Context, key string) ([]byte, error) {
	if f.getFn != nil {
		return f.getFn(ctx, key)
	}
	return nil, &cache.NotFoundError{Key: key}
}

func (f *fakeCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if f.setFn != nil {
		return f.setFn(ctx, key, value, ttl)
	}
	return nil
}

func (f *fakeCache) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if f.setIfAbsentFn != nil {
		return f.setIfAbsentFn(ctx, key, value, ttl)
	}
	return false, nil
}

func (f *fakeCache) Delete(ctx context.Context, key string) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, key)
	}
	return nil
}

func (f *fakeCache) Increment(ctx context.Context, key string) error {
	if f.incrementFn != nil {
		return f.incrementFn(ctx, key)
	}
	return nil
}

func (f *fakeCache) Decrement(ctx context.Context, key string) error {
	if f.decrementFn != nil {
		return f.decrementFn(ctx, key)
	}
	return nil
}

func (f *fakeCache) Exists(ctx context.Context, key string) (bool, error) {
	if f.existsFn != nil {
		return f.existsFn(ctx, key)
	}
	return false, nil
}

func (f *fakeCache) Close(ctx context.Context) error {
	if f.closeFn != nil {
		return f.closeFn(ctx)
	}
	return nil
}

type stubQueue struct {
	pushFn        func(context.Context, string, queue.Payload, queue.Headers) error
	pushDelayedFn func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error
	popFn         func(context.Context, string) (queue.Message, error)
	ackFn         func(context.Context, queue.Message) error
	nackFn        func(context.Context, queue.Message, bool) error
	lengthFn      func(context.Context, string) (int64, error)
	isEmptyFn     func(context.Context, string) (bool, error)
	closeFn       func() error
	nameFn        func() string
	pushes        int
	delayedPushes int
	lastTopic     string
	lastHeaders   queue.Headers
	lastPayload   queue.Payload
	lastDelay     time.Duration
}

func (s *stubQueue) Push(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	s.pushes++
	s.lastTopic = topic
	s.lastPayload = payload
	s.lastHeaders = headers
	if s.pushFn != nil {
		return s.pushFn(ctx, topic, payload, headers)
	}
	return nil
}

func (s *stubQueue) PushDelayed(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers, delay time.Duration) error {
	s.delayedPushes++
	s.lastTopic = topic
	s.lastPayload = payload
	s.lastHeaders = headers
	s.lastDelay = delay
	if s.pushDelayedFn != nil {
		return s.pushDelayedFn(ctx, topic, payload, headers, delay)
	}
	return nil
}

func (s *stubQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	if s.popFn != nil {
		return s.popFn(ctx, topic)
	}
	return queue.Message{}, queue.ErrEmpty
}

func (s *stubQueue) Ack(ctx context.Context, msg queue.Message) error {
	if s.ackFn != nil {
		return s.ackFn(ctx, msg)
	}
	return nil
}

func (s *stubQueue) Nack(ctx context.Context, msg queue.Message, requeue bool) error {
	if s.nackFn != nil {
		return s.nackFn(ctx, msg, requeue)
	}
	return nil
}

func (s *stubQueue) Length(ctx context.Context, topic string) (int64, error) {
	if s.lengthFn != nil {
		return s.lengthFn(ctx, topic)
	}
	return 0, nil
}

func (s *stubQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	if s.isEmptyFn != nil {
		return s.isEmptyFn(ctx, topic)
	}
	return true, nil
}

func (s *stubQueue) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

func (s *stubQueue) Name() string {
	if s.nameFn != nil {
		return s.nameFn()
	}
	return "stub"
}

// lock.go

func TestLockNewUniqueLocker(t *testing.T) {
	Reset()
	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("cachememory.New: %v", err)
	}
	defer c.Close(t.Context())
	l := NewUniqueLocker(c)
	if l == nil {
		t.Fatal("NewUniqueLocker returned nil")
	}
	if l.cache == nil {
		t.Fatal("UniqueLocker.cache is nil")
	}
}

func TestLockUniqueKey(t *testing.T) {
	Reset()
	l := NewUniqueLocker(nil)
	if got := l.uniqueKey("k"); got != "job:unique:k" {
		t.Fatalf("uniqueKey(k)=%q want %q", got, "job:unique:k")
	}
	if got := l.uniqueKey(""); got != "job:unique:" {
		t.Fatalf("uniqueKey(empty)=%q want %q", got, "job:unique:")
	}
	if got := l.uniqueKey("a:b"); got != "job:unique:a:b" {
		t.Fatalf("uniqueKey a:b=%q want %q", got, "job:unique:a:b")
	}
}

func TestLockAcquireInvalidTTL(t *testing.T) {
	Reset()
	l := NewUniqueLocker(nil)
	for _, ttl := range []time.Duration{0, -time.Second, -time.Nanosecond} {
		ok, err := l.Acquire(t.Context(), "k", ttl)
		if ok {
			t.Errorf("Acquire ttl=%v ok=true want false", ttl)
		}
		if !errors.Is(err, ErrInvalidLockTTL) {
			t.Errorf("Acquire ttl=%v err=%v want ErrInvalidLockTTL", ttl, err)
		}
	}
}

func TestLockAcquireDedupAndReleaseRealCache(t *testing.T) {
	Reset()
	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("cachememory.New: %v", err)
	}
	defer c.Close(t.Context())
	l := NewUniqueLocker(c)
	ctx := t.Context()
	ok, err := l.Acquire(ctx, "dedup", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first Acquire ok=%v err=%v want ok=true", ok, err)
	}
	ok2, err := l.Acquire(ctx, "dedup", time.Minute)
	if err != nil {
		t.Fatalf("second Acquire err=%v", err)
	}
	if ok2 {
		t.Fatalf("second Acquire ok=true want false")
	}
	if rerr := l.Release(ctx, "dedup"); rerr != nil {
		t.Fatalf("Release: %v", rerr)
	}
	ok3, err := l.Acquire(ctx, "dedup", time.Minute)
	if err != nil || !ok3 {
		t.Fatalf("after Release Acquire ok=%v err=%v want ok=true", ok3, err)
	}
}

func TestLockAcquireSetIfAbsentError(t *testing.T) {
	Reset()
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return false, errors.New("boom")
		},
	}
	l := NewUniqueLocker(fc)
	ok, err := l.Acquire(t.Context(), "k", time.Minute)
	if ok {
		t.Fatalf("Acquire ok=true want false on error")
	}
	if err == nil || err.Error() != "boom" {
		t.Fatalf("Acquire err=%v want boom", err)
	}
}

func TestLockReleaseDeletes(t *testing.T) {
	Reset()
	// fake tracking
	var deleted string
	fc := &fakeCache{
		deleteFn: func(_ context.Context, key string) error {
			deleted = key
			return nil
		},
	}
	l := NewUniqueLocker(fc)
	if err := l.Release(t.Context(), "mykey"); err != nil {
		t.Fatalf("Release err=%v", err)
	}
	if deleted != "job:unique:mykey" {
		t.Fatalf("Delete key=%q want %q", deleted, "job:unique:mykey")
	}
	// real cache path: after Release Acquire true again already tested, but also verify Delete idempotent
	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("cachememory.New: %v", err)
	}
	defer c.Close(t.Context())
	l2 := NewUniqueLocker(c)
	ctx := t.Context()
	if ok, _ := l2.Acquire(ctx, "rel", time.Minute); !ok {
		t.Fatal("Acquire before Release failed")
	}
	if err := l2.Release(ctx, "rel"); err != nil {
		t.Fatalf("Release real: %v", err)
	}
	// second Release should not error
	if err := l2.Release(ctx, "rel"); err != nil {
		t.Fatalf("second Release real: %v", err)
	}
	if ok, _ := l2.Acquire(ctx, "rel", time.Minute); !ok {
		t.Fatal("Acquire after Release want true")
	}
}

func TestLockAcquirePrefixCheck(t *testing.T) {
	Reset()
	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("cachememory.New: %v", err)
	}
	defer c.Close(t.Context())
	l := NewUniqueLocker(c)
	ctx := t.Context()
	key := "prefixed"
	ok, err := l.Acquire(ctx, key, time.Minute)
	if err != nil || !ok {
		t.Fatalf("Acquire ok=%v err=%v", ok, err)
	}
	// verify cache has entry under prefix
	val, err := c.Get(ctx, "job:unique:"+key)
	if err != nil {
		t.Fatalf("Get prefix key err=%v", err)
	}
	if string(val) != "1" {
		t.Fatalf("Get value=%q want %q", string(val), "1")
	}
	// non-prefixed should be missing
	if _, err := c.Get(ctx, key); err == nil {
		t.Fatal("Get without prefix should fail")
	}
}

// dispatch.go

func TestDispatchLog(t *testing.T) {
	Reset()
	d := &Dispatcher{}
	if got := d.log(); got == nil {
		t.Fatal("log() nil with nil Logger")
	} else if got.Name() != "noop" {
		t.Fatalf("log() Name=%q want noop", got.Name())
	}
	n := noop.New()
	d2 := &Dispatcher{Logger: n}
	if got := d2.log(); got != n {
		t.Fatal("log() with Logger should return it")
	} else if got.Name() != "noop" {
		t.Fatalf("log() with noop Name=%q want noop", got.Name())
	}
}

func TestDispatchOptionsInAtUniqueBy(t *testing.T) {
	Reset()
	// In
	var o dispatchOptions
	In(time.Second)(&o)
	if o.delay != time.Second {
		t.Fatalf("In delay=%v want %v", o.delay, time.Second)
	}
	In(0)(&o)
	if o.delay != 0 {
		t.Fatalf("In 0 delay=%v want 0", o.delay)
	}
	// At
	ts := time.Now().Add(time.Hour)
	At(ts)(&o)
	if o.at == nil || !o.at.Equal(ts) {
		t.Fatalf("At mismatch got %v want %v", o.at, ts)
	}
	// UniqueBy
	UniqueBy("mykey")(&o)
	if o.uniqueBy != "mykey" {
		t.Fatalf("UniqueBy=%q want %q", o.uniqueBy, "mykey")
	}
	UniqueBy("")(&o)
	if o.uniqueBy != "" {
		t.Fatalf("UniqueBy empty=%q", o.uniqueBy)
	}
}

func TestDispatchUnknownJob(t *testing.T) {
	Reset()
	d := &Dispatcher{Q: &stubQueue{}}
	err := d.Dispatch(t.Context(), "unknown", "arg")
	if err == nil {
		t.Fatal("Dispatch unknown want error")
	}
	if !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("errors.Is ErrUnknownJob false err=%v", err)
	}
	var ue *UnknownJobError
	if !errors.As(err, &ue) {
		t.Fatalf("errors.As UnknownJobError false err=%v", err)
	}
	if ue.Name != "unknown" {
		t.Fatalf("UnknownJobError.Name=%q want unknown", ue.Name)
	}
}

func TestDispatchMarshalError(t *testing.T) {
	Reset()
	if err := Register("marshal-job", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	d := &Dispatcher{Q: &stubQueue{}}
	err := d.Dispatch(t.Context(), "marshal-job", make(chan int))
	if err == nil {
		t.Fatal("Dispatch chan want marshal error")
	}
	if !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("err %q does not contain marshal", err.Error())
	}
}

func TestDispatchUniqueByNilLocker(t *testing.T) {
	Reset()
	if err := Register("uniq-nil", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	d := &Dispatcher{Q: &stubQueue{}, UniqueLocker: nil}
	err := d.Dispatch(t.Context(), "uniq-nil", "arg", UniqueBy("k"))
	if err == nil {
		t.Fatal("Dispatch UniqueBy nil locker want error")
	}
	if !errors.Is(err, ErrUniqueLockerNil) {
		t.Fatalf("errors.Is ErrUniqueLockerNil false err=%v", err)
	}
	if !strings.Contains(err.Error(), "uniq-nil") {
		t.Fatalf("err %q should contain job name", err.Error())
	}
}

func TestDispatchUniqueLockAcquireError(t *testing.T) {
	Reset()
	if err := Register("uniq-err", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return false, errors.New("boom")
		},
	}
	d := &Dispatcher{Q: &stubQueue{}, UniqueLocker: NewUniqueLocker(fc)}
	err := d.Dispatch(t.Context(), "uniq-err", "arg", UniqueBy("k"))
	if err == nil {
		t.Fatal("Dispatch want unique lock error")
	}
	if !strings.Contains(err.Error(), "unique lock") {
		t.Fatalf("err %q missing unique lock", err.Error())
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err %q missing boom", err.Error())
	}
}

func TestDispatchUniqueLockNotAcquired(t *testing.T) {
	Reset()
	if err := Register("uniq-not", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sq := &stubQueue{}
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return false, nil
		},
	}
	d := &Dispatcher{Q: sq, UniqueLocker: NewUniqueLocker(fc)}
	err := d.Dispatch(t.Context(), "uniq-not", "arg", UniqueBy("k"))
	if err != nil {
		t.Fatalf("Dispatch not acquired err=%v want nil", err)
	}
	if sq.pushes != 0 || sq.delayedPushes != 0 {
		t.Fatalf("Push called pushes=%d delayed=%d want 0", sq.pushes, sq.delayedPushes)
	}
}

func TestDispatchUniqueLockAcquiredPush(t *testing.T) {
	Reset()
	if err := Register("uniq-ok", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sq := &stubQueue{}
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return true, nil
		},
	}
	d := &Dispatcher{Q: sq, UniqueLocker: NewUniqueLocker(fc)}
	err := d.Dispatch(t.Context(), "uniq-ok", "arg", UniqueBy("mykey"))
	if err != nil {
		t.Fatalf("Dispatch acquired err=%v", err)
	}
	if sq.pushes != 1 {
		t.Fatalf("Push count=%d want 1", sq.pushes)
	}
	expected := uniqueID("uniq-ok", "mykey")
	if sq.lastHeaders["unique_id"] != expected {
		t.Fatalf("unique_id header=%q want %q", sq.lastHeaders["unique_id"], expected)
	}
	if sq.lastHeaders["job_name"] != "uniq-ok" {
		t.Fatalf("job_name header=%q want uniq-ok", sq.lastHeaders["job_name"])
	}
}

func TestDispatchUniqueLockAcquiredPushDelayed(t *testing.T) {
	Reset()
	if err := Register("uniq-delay", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sq := &stubQueue{}
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return true, nil
		},
	}
	d := &Dispatcher{Q: sq, UniqueLocker: NewUniqueLocker(fc)}
	err := d.Dispatch(t.Context(), "uniq-delay", "arg", UniqueBy("k"), In(time.Second))
	if err != nil {
		t.Fatalf("Dispatch err=%v", err)
	}
	if sq.delayedPushes != 1 {
		t.Fatalf("delayedPushes=%d want 1", sq.delayedPushes)
	}
	if sq.lastHeaders["unique_id"] == "" {
		t.Fatal("unique_id header missing on delayed push")
	}
	expected := uniqueID("uniq-delay", "k")
	if sq.lastHeaders["unique_id"] != expected {
		t.Fatalf("unique_id=%q want %q", sq.lastHeaders["unique_id"], expected)
	}
}

func TestDispatchPushPaths(t *testing.T) {
	Reset()
	if err := Register("push-path", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// delay>0 → PushDelayed
	sq := &stubQueue{}
	d := &Dispatcher{Q: sq}
	if err := d.Dispatch(t.Context(), "push-path", "arg", In(time.Second)); err != nil {
		t.Fatalf("Dispatch In 1s err=%v", err)
	}
	if sq.delayedPushes != 1 || sq.pushes != 0 {
		t.Fatalf("In 1s pushes=%d delayed=%d want 0/1", sq.pushes, sq.delayedPushes)
	}
	if sq.lastDelay != time.Second {
		t.Fatalf("delay=%v want 1s", sq.lastDelay)
	}
	// delay<=0 → Push
	sq2 := &stubQueue{}
	d2 := &Dispatcher{Q: sq2}
	if err := d2.Dispatch(t.Context(), "push-path", "arg"); err != nil {
		t.Fatalf("Dispatch no delay err=%v", err)
	}
	if sq2.pushes != 1 || sq2.delayedPushes != 0 {
		t.Fatalf("no delay pushes=%d delayed=%d want 1/0", sq2.pushes, sq2.delayedPushes)
	}
	// In(0) → Push
	sq3 := &stubQueue{}
	d3 := &Dispatcher{Q: sq3}
	if err := d3.Dispatch(t.Context(), "push-path", "arg", In(0)); err != nil {
		t.Fatalf("Dispatch In 0 err=%v", err)
	}
	if sq3.pushes != 1 {
		t.Fatalf("In 0 pushes=%d want 1", sq3.pushes)
	}
}

func TestDispatchAtFutureAndPast(t *testing.T) {
	Reset()
	if err := Register("at-job", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// future → PushDelayed
	sq := &stubQueue{}
	d := &Dispatcher{Q: sq}
	future := time.Now().Add(5 * time.Second)
	if err := d.Dispatch(t.Context(), "at-job", "arg", At(future)); err != nil {
		t.Fatalf("Dispatch At future err=%v", err)
	}
	if sq.delayedPushes != 1 {
		t.Fatalf("At future delayed=%d want 1", sq.delayedPushes)
	}
	// past → Push
	sq2 := &stubQueue{}
	d2 := &Dispatcher{Q: sq2}
	past := time.Now().Add(-5 * time.Second)
	if err := d2.Dispatch(t.Context(), "at-job", "arg", At(past)); err != nil {
		t.Fatalf("Dispatch At past err=%v", err)
	}
	if sq2.pushes != 1 || sq2.delayedPushes != 0 {
		t.Fatalf("At past pushes=%d delayed=%d want 1/0", sq2.pushes, sq2.delayedPushes)
	}
}

func TestDispatchAtOverridesIn(t *testing.T) {
	Reset()
	if err := Register("at-over", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// At should override In delay; even if In is large, At past should Push
	sq := &stubQueue{}
	d := &Dispatcher{Q: sq}
	past := time.Now().Add(-time.Second)
	if err := d.Dispatch(t.Context(), "at-over", "arg", In(time.Hour), At(past)); err != nil {
		t.Fatalf("Dispatch At+In err=%v", err)
	}
	if sq.pushes != 1 {
		t.Fatalf("At past overrides In pushes=%d want 1", sq.pushes)
	}
	// At future overrides In(0)
	sq2 := &stubQueue{}
	d2 := &Dispatcher{Q: sq2}
	future := time.Now().Add(time.Hour)
	if err := d2.Dispatch(t.Context(), "at-over", "arg", In(0), At(future)); err != nil {
		t.Fatalf("Dispatch At future+In0 err=%v", err)
	}
	if sq2.delayedPushes != 1 {
		t.Fatalf("At future overrides In0 delayed=%d want 1", sq2.delayedPushes)
	}
}

func TestDispatchPushErrorReleasesLock(t *testing.T) {
	Reset()
	if err := Register("rel-job", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	pushErr := errors.New("push boom")
	sq := &stubQueue{
		pushFn: func(context.Context, string, queue.Payload, queue.Headers) error {
			return pushErr
		},
	}
	released := false
	var releasedKey string
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return true, nil
		},
		deleteFn: func(_ context.Context, key string) error {
			released = true
			releasedKey = key
			return nil
		},
	}
	d := &Dispatcher{Q: sq, UniqueLocker: NewUniqueLocker(fc)}
	err := d.Dispatch(t.Context(), "rel-job", "arg", UniqueBy("k"))
	if !errors.Is(err, pushErr) {
		t.Fatalf("err=%v want pushErr", err)
	}
	if !released {
		t.Fatal("Release not called on push error")
	}
	expected := "job:unique:" + uniqueID("rel-job", "k")
	if releasedKey != expected {
		t.Fatalf("releasedKey=%q want %q", releasedKey, expected)
	}
}

func TestDispatchPushDelayedErrorReleasesLock(t *testing.T) {
	Reset()
	if err := Register("rel-delay", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	pushErr := errors.New("delayed boom")
	sq := &stubQueue{
		pushDelayedFn: func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error {
			return pushErr
		},
	}
	released := false
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return true, nil
		},
		deleteFn: func(context.Context, string) error {
			released = true
			return nil
		},
	}
	d := &Dispatcher{Q: sq, UniqueLocker: NewUniqueLocker(fc)}
	err := d.Dispatch(t.Context(), "rel-delay", "arg", UniqueBy("k"), In(time.Second))
	if !errors.Is(err, pushErr) {
		t.Fatalf("err=%v want pushErr", err)
	}
	if !released {
		t.Fatal("Release not called on PushDelayed error")
	}
}

func TestDispatchPushErrorReleaseWarn(t *testing.T) {
	Reset()
	if err := Register("warn-job", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	pushErr := errors.New("push fail")
	sq := &stubQueue{
		pushFn: func(context.Context, string, queue.Payload, queue.Headers) error {
			return pushErr
		},
	}
	fc := &fakeCache{
		setIfAbsentFn: func(context.Context, string, []byte, time.Duration) (bool, error) {
			return true, nil
		},
		deleteFn: func(context.Context, string) error {
			return errors.New("delete boom")
		},
	}
	// Logger nil → log() returns noop, Warn should not panic
	d := &Dispatcher{Q: sq, UniqueLocker: NewUniqueLocker(fc), Logger: nil}
	err := d.Dispatch(t.Context(), "warn-job", "arg", UniqueBy("k"))
	if !errors.Is(err, pushErr) {
		t.Fatalf("err=%v want pushErr", err)
	}
	// with custom logger also should not panic and return pushErr
	d2 := &Dispatcher{Q: sq, UniqueLocker: NewUniqueLocker(fc), Logger: noop.New()}
	err2 := d2.Dispatch(t.Context(), "warn-job", "arg", UniqueBy("k"))
	if !errors.Is(err2, pushErr) {
		t.Fatalf("err2=%v want pushErr", err2)
	}
}

func TestDispatchPushErrorNoReleaseWhenNotAcquired(t *testing.T) {
	Reset()
	if err := Register("no-rel", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	pushErr := errors.New("push fail")
	sq := &stubQueue{
		pushFn: func(context.Context, string, queue.Payload, queue.Headers) error {
			return pushErr
		},
	}
	// no UniqueBy → acquired false, so Release should not be called even on push error
	deleteCalled := false
	fc := &fakeCache{
		deleteFn: func(context.Context, string) error {
			deleteCalled = true
			return nil
		},
	}
	d := &Dispatcher{Q: sq, UniqueLocker: NewUniqueLocker(fc)}
	err := d.Dispatch(t.Context(), "no-rel", "arg")
	if !errors.Is(err, pushErr) {
		t.Fatalf("err=%v want pushErr", err)
	}
	if deleteCalled {
		t.Fatal("Delete should not be called when not acquired")
	}
}

func TestDispatchSuccessRealQueue(t *testing.T) {
	Reset()
	if err := Register("real-q", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	q, err := queuememory.New(queue.Options{})
	if err != nil {
		t.Fatalf("queuememory.New: %v", err)
	}
	defer q.Close()
	d := &Dispatcher{Q: q}
	if derr := d.Dispatch(t.Context(), "real-q", map[string]string{"x": "1"}); derr != nil {
		t.Fatalf("Dispatch real queue err=%v", derr)
	}
	// verify message enqueued
	n, err := q.Length(t.Context(), "low")
	if err != nil {
		t.Fatalf("Length: %v", err)
	}
	if n != 1 {
		t.Fatalf("Length=%d want 1", n)
	}
}

func TestDispatchUniqueTTL(t *testing.T) {
	Reset()
	d := &Dispatcher{UniqueTTL: 5 * time.Minute}
	if got := d.uniqueTTL(); got != 5*time.Minute {
		t.Fatalf("uniqueTTL=%v want 5m", got)
	}
	d2 := &Dispatcher{UniqueTTL: 0}
	if got := d2.uniqueTTL(); got != 24*time.Hour {
		t.Fatalf("uniqueTTL 0=%v want 24h", got)
	}
	d3 := &Dispatcher{UniqueTTL: -time.Second}
	if got := d3.uniqueTTL(); got != 24*time.Hour {
		t.Fatalf("uniqueTTL negative=%v want 24h", got)
	}
}

func TestDispatchUniqueID(t *testing.T) {
	Reset()
	// determinism
	a := uniqueID("job", "key")
	b := uniqueID("job", "key")
	if a != b {
		t.Fatalf("uniqueID not deterministic %q vs %q", a, b)
	}
	// different inputs → different ids
	c := uniqueID("job", "key2")
	if a == c {
		t.Fatal("uniqueID collision for different key")
	}
	d := uniqueID("job2", "key")
	if a == d {
		t.Fatal("uniqueID collision for different jobName")
	}
	// known sha256 hex
	h := sha256.New()
	_, _ = h.Write([]byte("job"))
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte("key"))
	expected := hex.EncodeToString(h.Sum(nil))
	if a != expected {
		t.Fatalf("uniqueID=%q want %q", a, expected)
	}
	if len(a) != 64 {
		t.Fatalf("uniqueID len=%d want 64", len(a))
	}
	// another known vector
	h2 := sha256.New()
	_, _ = h2.Write([]byte("myjob"))
	_, _ = h2.Write([]byte(":"))
	_, _ = h2.Write([]byte("mykey"))
	exp2 := hex.EncodeToString(h2.Sum(nil))
	if got := uniqueID("myjob", "mykey"); got != exp2 {
		t.Fatalf("uniqueID myjob:mykey=%q want %q", got, exp2)
	}
}

func TestDispatchPriorityAndHeaders(t *testing.T) {
	Reset()
	if err := Register("prio-job", func(context.Context, string) error { return nil }, WithPriority(PriorityHigh)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sq := &stubQueue{}
	d := &Dispatcher{Q: sq}
	if err := d.Dispatch(t.Context(), "prio-job", "arg"); err != nil {
		t.Fatalf("Dispatch err=%v", err)
	}
	if sq.lastTopic != "high" {
		t.Fatalf("topic=%q want high", sq.lastTopic)
	}
	if sq.lastHeaders["job_name"] != "prio-job" {
		t.Fatalf("header job_name=%q want prio-job", sq.lastHeaders["job_name"])
	}
}

func TestLockReleaseFakeError(t *testing.T) {
	Reset()
	fc := &fakeCache{
		deleteFn: func(context.Context, string) error {
			return errors.New("del boom")
		},
	}
	l := NewUniqueLocker(fc)
	err := l.Release(t.Context(), "k")
	if err == nil || err.Error() != "del boom" {
		t.Fatalf("Release err=%v want del boom", err)
	}
}
