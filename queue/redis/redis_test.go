package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/queue"
)

func TestRedactURL_masks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts queue.Options
		want string
	}{
		{"empty", queue.Options{}, ""},
		{"plain addr passthrough", queue.Options{Addr: "localhost:6379"}, "localhost:6379"},
		{"url password masked", queue.Options{URL: "redis://:s3cret@h:6379"}, "redis://:xxxxx@h:6379"},
		{"url user+password masked", queue.Options{URL: "redis://user:s3cret@h:6379"}, "redis://user:xxxxx@h:6379"}, //nolint:gosec
		{"addr fallback masked", queue.Options{Addr: "redis://:s3cret@h:6379"}, "redis://:xxxxx@h:6379"},
		{"url preferred over addr", queue.Options{URL: "redis://:s3cret@h:6379", Addr: "redis://:other@h:6379"}, "redis://:xxxxx@h:6379"},
		{"unparsable raw", queue.Options{URL: "redis://[::1"}, "redis://[::1"},
		{"whitespace trimmed", queue.Options{URL: "  redis://:s3cret@h:6379  "}, "redis://:xxxxx@h:6379"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redactURL(tc.opts)
			if got != tc.want {
				t.Errorf("redactURL(%+v) = %q, want %q", tc.opts, got, tc.want)
			}
			if strings.Contains(got, "s3cret") {
				t.Errorf("redactURL leaks password: %q", got)
			}
			if tc.want != "" && strings.Contains(tc.want, "xxxxx") && !strings.Contains(got, "xxxxx") {
				t.Errorf("redactURL missing xxxxx: %q", got)
			}
		})
	}
}

func TestBuildKey(t *testing.T) {
	t.Parallel()
	a := &redisAdapter{prefix: "queue"}
	if got := a.buildKey("jobs", ":ready"); got != "queue:jobs:ready" {
		t.Errorf("buildKey = %q, want %q", got, "queue:jobs:ready")
	}
	cases := []struct {
		fn   func(string) string
		want string
	}{
		{a.readyKey, "queue:jobs:ready"},
		{a.processingKey, "queue:jobs:processing"},
		{a.delayedKey, "queue:jobs:delayed"},
		{a.deadlineKey, "queue:jobs:processing_deadlines"},
	}
	for _, tc := range cases {
		if got := tc.fn("jobs"); got != tc.want {
			t.Errorf("key = %q, want %q", got, tc.want)
		}
	}
}

func TestToWireMessage_clone(t *testing.T) {
	t.Parallel()
	payload := queue.Payload([]byte("p"))
	headers := queue.Headers{"h": "v"}
	msg := queue.NewMessage("t", payload, headers)
	wire := toWireMessage(msg)
	// mutate original
	payload[0] = 'x'
	headers["h"] = "changed"
	headers["new"] = "oops"
	msg.Payload[0] = 'y'
	if string(wire.Payload) != "p" {
		t.Errorf("wire Payload = %q, want %q", string(wire.Payload), "p")
	}
	if got := wire.Headers["h"]; got != "v" {
		t.Errorf("wire Headers[h] = %q, want %q", got, "v")
	}
	if _, ok := wire.Headers["new"]; ok {
		t.Errorf("wire Headers unexpectedly contains new key")
	}
	if wire.ID != msg.ID.String() {
		t.Errorf("wire ID = %q, want %q", wire.ID, msg.ID.String())
	}
	if wire.Attempt != msg.Attempt {
		t.Errorf("wire Attempt = %d, want %d", wire.Attempt, msg.Attempt)
	}
}

func TestRedisClosed_fastPaths(t *testing.T) {
	t.Parallel()
	a := &redisAdapter{}
	a.closed.Store(true)
	ctx := context.Background()
	topic := "jobs"
	payload := queue.Payload([]byte("p"))
	headers := queue.Headers{"h": "v"}
	msg := queue.NewMessage(topic, payload, headers)

	if err := a.Push(ctx, topic, payload, headers); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Push err = %v, want ErrClosed", err)
	}
	if err := a.PushDelayed(ctx, topic, payload, headers, 0); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("PushDelayed err = %v, want ErrClosed", err)
	}
	if _, err := a.Pop(ctx, topic); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Pop err = %v, want ErrClosed", err)
	}
	if err := a.Ack(ctx, msg); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Ack err = %v, want ErrClosed", err)
	}
	if err := a.Nack(ctx, msg, true); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Nack err = %v, want ErrClosed", err)
	}
	if _, err := a.Length(ctx, topic); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Length err = %v, want ErrClosed", err)
	}
	if _, err := a.IsEmpty(ctx, topic); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("IsEmpty err = %v, want ErrClosed", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("Close idempotent err = %v, want nil", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("Close second idempotent err = %v, want nil", err)
	}
}

func TestRedisNew_invalidAddr_failsBeforeDial(t *testing.T) {
	_, err := New(queue.Options{Addr: "redis://"})
	if err == nil {
		t.Fatal("New() = nil, want error for invalid addr")
	}
}

func TestName(t *testing.T) {
	t.Parallel()
	a := &redisAdapter{}
	if got := a.Name(); got != "redis" {
		t.Errorf("Name() = %q, want redis", got)
	}
}

func TestNextBackoff(t *testing.T) {
	t.Parallel()
	if got := nextBackoff(1); got < 10*time.Millisecond || got > 200*time.Millisecond {
		t.Errorf("nextBackoff(1) = %v out of range", got)
	}
	if got := nextBackoff(20); got != 200*time.Millisecond {
		t.Errorf("nextBackoff clamp failed got %v, want 200ms", got)
	}
}

func TestDecodeMessage_errors(t *testing.T) {
	t.Parallel()
	if _, err := decodeMessage("not-json", "t"); err == nil {
		t.Error("decodeMessage invalid json = nil want error")
	}
	if _, err := decodeMessage(`{"id":"bad-id","payload":"aGVsbG8=","headers":{}}`, "t"); err == nil {
		t.Error("decodeMessage bad id = nil want error")
	}
}
