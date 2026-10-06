package redis

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/queue"
)

// TestRegister_opensViaCoreOptions proves Register wires the redis adapter
// into the core registry so queue.Open resolves it against a live client.
func TestRegister_opensViaCoreOptions(t *testing.T) {
	s := miniredis.RunT(t)

	Register()

	q, err := queue.Open(queue.Redis, queue.Options{Addr: s.Addr(), PollTimeout: time.Second})
	if err != nil {
		t.Fatalf("queue.Open(redis) = %v", err)
	}

	t.Cleanup(func() { _ = q.Close() })

	if q.Name() != "redis" {
		t.Errorf("Name() = %q, want redis", q.Name())
	}
}

func TestBuildKey_emptyTopicAndPrefix(t *testing.T) {
	t.Parallel()

	a := &redisAdapter{prefix: "queue"}
	if got := a.readyKey(""); got != "queue::ready" {
		t.Errorf("readyKey(empty) = %q, want queue::ready", got)
	}
	if got := a.processingKey("t"); got != "queue:t:processing" {
		t.Errorf("processingKey(t) = %q, want queue:t:processing", got)
	}
	if got := a.deadlineKey("t"); got != "queue:t:processing_deadlines" {
		t.Errorf("deadlineKey(t) = %q, want queue:t:processing_deadlines", got)
	}
	if got := a.delayedKey("t"); got != "queue:t:delayed" {
		t.Errorf("delayedKey(t) = %q, want queue:t:delayed", got)
	}
}

func TestNextBackoff_withinBounds(t *testing.T) {
	t.Parallel()

	for attempt := 1; attempt <= 20; attempt++ {
		got := nextBackoff(attempt)
		if got < DefaultBufferBaseDelay || got > DefaultBufferMaxDelay {
			t.Errorf("nextBackoff(%d) = %v, want within [%v, %v]", attempt, got, DefaultBufferBaseDelay, DefaultBufferMaxDelay)
		}
	}
}

func TestConnOptions_urlTakesPrecedence(t *testing.T) {
	t.Parallel()

	got := connOptions(queue.Options{URL: "redis://url-host:6379", Addr: "addr-host:6380"})
	if got.Addr != "redis://url-host:6379" {
		t.Errorf("connOptions URL precedence: Addr = %q, want URL", got.Addr)
	}

	got = connOptions(queue.Options{Addr: "addr-host:6380"})
	if got.Addr != "addr-host:6380" {
		t.Errorf("connOptions Addr fallback: Addr = %q, want addr-host:6380", got.Addr)
	}
}

func TestConnOptions_trimsWhitespaceURL(t *testing.T) {
	t.Parallel()

	got := connOptions(queue.Options{URL: "  redis://h:1  ", Addr: "addr:2"})
	if got.Addr != "redis://h:1" {
		t.Errorf("connOptions trimmed URL = %q, want redis://h:1", got.Addr)
	}
}

func TestIsEmpty_zeroAfterClose_returnsErrClosed(t *testing.T) {
	t.Parallel()

	a := &redisAdapter{}
	a.closed.Store(true)
	if _, err := a.IsEmpty(t.Context(), "t"); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("IsEmpty(after close) = %v, want ErrClosed", err)
	}
}

func TestClose_releaseOnce(t *testing.T) {
	t.Parallel()

	var calls int
	a := &redisAdapter{release: func() error { calls++; return nil }}
	if err := a.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if calls != 1 {
		t.Errorf("release calls = %d, want 1", calls)
	}
}

func TestSweepInterval_positive(t *testing.T) {
	t.Parallel()

	if sweepInterval <= 0 {
		t.Errorf("sweepInterval = %v, want > 0", sweepInterval)
	}
	if minBlockingTimeout < time.Second {
		t.Errorf("minBlockingTimeout = %v, want >= 1s (go-redis truncates sub-second BLPop)", minBlockingTimeout)
	}
}
