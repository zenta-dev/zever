package redis

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/lock"
)

// benchAdapter wires an adapter to a fake client that always grants the
// claim and reports a successful script result, exercising the adapter's
// non-network code path.
func benchAdapter(b *testing.B) *adapter {
	b.Helper()

	return &adapter{
		client: &fakeClient{
			setNXFn: func(context.Context, string, any, time.Duration) (bool, error) { return true, nil },
			evalFn:  func(context.Context, string, []string, ...any) (int64, error) { return 1, nil },
		},
		prefix:        "lock:",
		ttl:           time.Hour,
		retryInterval: time.Millisecond,
	}
}

// BenchmarkTryAcquire measures the winning SET NX claim path.
func BenchmarkTryAcquire(b *testing.B) {
	a := benchAdapter(b)

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if _, ok, err := a.TryAcquire(ctx, "bench-key", time.Minute); err != nil || !ok {
			b.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
		}
	}
}

// BenchmarkTryAcquireContended measures the rejected claim on a held key.
func BenchmarkTryAcquireContended(b *testing.B) {
	a := benchAdapter(b)
	a.client = &fakeClient{
		setNXFn: func(context.Context, string, any, time.Duration) (bool, error) { return false, nil },
		evalFn:  func(context.Context, string, []string, ...any) (int64, error) { return 1, nil },
	}

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		_, ok, err := a.TryAcquire(ctx, "hot", time.Minute)
		if err != nil {
			b.Fatalf("TryAcquire failed: %v", err)
		}

		if ok {
			b.Fatal("TryAcquire acquired a held key")
		}
	}
}

// BenchmarkUnlock measures the compare-and-delete release script dispatch.
func BenchmarkUnlock(b *testing.B) {
	a := benchAdapter(b)

	h := &handle{a: a, client: a.client, key: "bench-key", holder: "holder"}

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if err := h.Unlock(ctx); err != nil {
			b.Fatalf("Unlock failed: %v", err)
		}
	}
}

// BenchmarkExtend measures the compare-and-expire renewal script dispatch.
func BenchmarkExtend(b *testing.B) {
	a := benchAdapter(b)

	h := &handle{a: a, client: a.client, key: "bench-key", holder: "holder"}

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if err := h.Extend(ctx, time.Minute); err != nil {
			b.Fatalf("Extend failed: %v", err)
		}
	}
}

// BenchmarkAcquireImmediate measures the uncontended Acquire path, which
// succeeds on the first attempt.
func BenchmarkAcquireImmediate(b *testing.B) {
	a := benchAdapter(b)

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		held, err := a.Acquire(ctx, "bench-key", time.Minute)
		if err != nil {
			b.Fatalf("Acquire failed: %v", err)
		}

		if held == nil {
			b.Fatal("Acquire returned nil lock")
		}
	}
}

// BenchmarkNewHolderID measures random holder-id generation.
func BenchmarkNewHolderID(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := newHolderID(); err != nil {
			b.Fatalf("newHolderID failed: %v", err)
		}
	}
}

// BenchmarkConnOptions measures mapping lock options onto shared client
// options.
func BenchmarkConnOptions(b *testing.B) {
	opts := lock.Options{URL: "redis://h:6379", Addr: "h2:6379", Password: "pw", DB: 4, TLS: true}

	b.ReportAllocs()

	for b.Loop() {
		_ = connOptions(opts)
	}
}

// BenchmarkRedactURL measures credential masking for error messages.
func BenchmarkRedactURL(b *testing.B) {
	opts := lock.Options{URL: "redis://user:secret@localhost:6379"} //nolint:gosec // test credential

	b.ReportAllocs()

	for b.Loop() {
		_ = redactURL(opts)
	}
}
