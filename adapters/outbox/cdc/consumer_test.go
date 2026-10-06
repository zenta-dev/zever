package cdc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/shared/retry"
)

// stubPublisher records delivered messages and can fail the next n calls.
type stubPublisher struct {
	mu       sync.Mutex
	msgs     []outbox.Message
	failures int
}

func (p *stubPublisher) Publish(_ context.Context, msg outbox.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failures > 0 {
		p.failures--

		return errors.New("stub: publish failed")
	}

	p.msgs = append(p.msgs, msg.Clone())

	return nil
}

func (p *stubPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.msgs)
}

// recordingSleep returns a fake sleep that records delays and never waits.
func recordingSleep(delays *[]time.Duration) sleepFunc {
	return func(_ context.Context, d time.Duration) error {
		*delays = append(*delays, d)

		return nil
	}
}

func mustPayload(t *testing.T, msg outbox.Message) []byte {
	t.Helper()

	payload, err := encodeMessage(msg)
	if err != nil {
		t.Fatalf("encodeMessage() error = %v", err)
	}

	return payload
}

func TestHandleMessagePublishSuccess(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, maxAttempts: 3, sleep: recordingSleep(new([]time.Duration))}

	pub := &stubPublisher{}
	msg := outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("p")}

	advanced, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), pub.Publish)
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}

	if !advanced {
		t.Error("handleMessage() advanced = false, want true")
	}

	if pub.count() != 1 {
		t.Errorf("published = %d, want 1", pub.count())
	}

	if st := s.Status(); st.Processed != 1 {
		t.Errorf("Status().Processed = %d, want 1", st.Processed)
	}
}

func TestHandleMessagePrefixMismatch(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, maxAttempts: 3, sleep: recordingSleep(new([]time.Duration))}

	pub := &stubPublisher{}
	msg := outbox.Message{ID: "evt-1", Topic: "orders"}

	advanced, err := s.handleMessage(t.Context(), "other_prefix", mustPayload(t, msg), pub.Publish)
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}

	if !advanced {
		t.Error("handleMessage() advanced = false, want true for foreign prefix")
	}

	if pub.count() != 0 {
		t.Errorf("published = %d, want 0 for foreign prefix", pub.count())
	}

	if st := s.Status(); st.Processed != 0 {
		t.Errorf("Status().Processed = %d, want 0", st.Processed)
	}
}

func TestHandleMessagePublishFailureRetriesAndDoesNotAdvance(t *testing.T) {
	t.Parallel()

	delays := []time.Duration{}
	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 3,
		retry:       retry.Policy{BaseDelay: time.Millisecond, Multiplier: 2},
		sleep:       recordingSleep(&delays),
	}

	msg := outbox.Message{ID: "evt-1", Topic: "orders"}

	alwaysFail := func(context.Context, outbox.Message) error {
		return errors.New("always fail")
	}

	attempts := 0
	countingFail := func(ctx context.Context, m outbox.Message) error {
		attempts++

		return alwaysFail(ctx, m)
	}

	advanced, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), countingFail)
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}

	if advanced {
		t.Error("handleMessage() advanced = true, want false after exhausted retries")
	}

	if attempts != 3 {
		t.Errorf("publish attempts = %d, want 3", attempts)
	}

	if st := s.Status(); st.Failed != 1 {
		t.Errorf("Status().Failed = %d, want 1", st.Failed)
	}

	if len(delays) != 2 {
		t.Fatalf("retry delays = %d, want 2 (between 3 attempts)", len(delays))
	}

	if delays[0] != time.Millisecond || delays[1] != 2*time.Millisecond {
		t.Errorf("retry delays = %v, want [1ms 2ms]", delays)
	}
}

func TestHandleMessageRecoversAfterTransientFailure(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, maxAttempts: 3, sleep: recordingSleep(new([]time.Duration))}

	pub := &stubPublisher{}
	msg := outbox.Message{ID: "evt-1", Topic: "orders"}

	fail := func(context.Context, outbox.Message) error {
		return errors.New("transient")
	}

	advanced, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), fail)
	if err != nil {
		t.Fatalf("handleMessage(fail) error = %v", err)
	}

	if advanced {
		t.Error("handleMessage(fail) advanced = true, want false")
	}

	advanced, err = s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), pub.Publish)
	if err != nil {
		t.Fatalf("handleMessage(recover) error = %v", err)
	}

	if !advanced {
		t.Error("handleMessage(recover) advanced = false, want true")
	}

	if pub.count() != 1 {
		t.Errorf("published = %d, want 1", pub.count())
	}
}

func TestHandleMessageInvalidPayload(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, maxAttempts: 3, sleep: recordingSleep(new([]time.Duration))}

	pub := &stubPublisher{}

	advanced, err := s.handleMessage(t.Context(), DefaultPrefix, []byte("not json"), pub.Publish)
	if err == nil {
		t.Fatal("handleMessage(invalid) = nil error, want error")
	}

	if advanced {
		t.Error("handleMessage(invalid) advanced = true, want false")
	}
}
