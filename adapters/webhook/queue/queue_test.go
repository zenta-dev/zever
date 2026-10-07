package queue

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/queue/memory"
	corequeue "github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/webhook"
)

var errTestTransport = errors.New("test transport boom")

func TestMain(m *testing.M) {
	_ = corequeue.Register(corequeue.Memory, func(_ corequeue.Options) (corequeue.Queue, error) {
		return newStubQueue(), nil
	})
	_ = corequeue.Register(corequeue.Redis, func(_ corequeue.Options) (corequeue.Queue, error) {
		return nil, errTestTransport
	})
	os.Exit(m.Run())
}

type stubPush struct {
	topic   string
	payload corequeue.Payload
	headers corequeue.Headers
}

type stubDelayed struct {
	push  stubPush
	delay time.Duration
}

type stubNack struct {
	msg     corequeue.Message
	requeue bool
}

type popOut struct {
	msg corequeue.Message
	err error
}

// stubQueue is a scripted in-memory queue.Queue for tests. No network.
type stubQueue struct {
	mu                sync.Mutex
	pushErr           error
	pushScript        []error
	pushDelayedScript []error
	pushes            []stubPush
	delayed           []stubDelayed
	popScript         []popOut
	popErr            error
	pops              int
	acks              []corequeue.Message
	nacks             []stubNack
	ackPanics         int
	ackPanic          any
	closeErr          error
	closes            int
}

func newStubQueue() *stubQueue {
	return &stubQueue{}
}

func (s *stubQueue) Push(
	_ context.Context,
	topic string,
	payload corequeue.Payload,
	headers corequeue.Headers,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pushes = append(s.pushes, stubPush{topic: topic, payload: payload, headers: headers})

	if len(s.pushScript) > 0 {
		err := s.pushScript[0]
		s.pushScript = s.pushScript[1:]

		return err
	}

	return s.pushErr
}

func (s *stubQueue) PushDelayed(
	_ context.Context,
	topic string,
	payload corequeue.Payload,
	headers corequeue.Headers,
	delay time.Duration,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.delayed = append(s.delayed, stubDelayed{
		push:  stubPush{topic: topic, payload: payload, headers: headers},
		delay: delay,
	})

	if len(s.pushDelayedScript) > 0 {
		err := s.pushDelayedScript[0]
		s.pushDelayedScript = s.pushDelayedScript[1:]

		return err
	}

	return nil
}

func (s *stubQueue) Pop(_ context.Context, _ string) (corequeue.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pops++

	if len(s.popScript) > 0 {
		out := s.popScript[0]
		s.popScript = s.popScript[1:]

		return out.msg, out.err
	}

	if s.popErr != nil {
		return corequeue.Message{}, s.popErr
	}

	return corequeue.Message{}, corequeue.ErrEmpty
}

func (s *stubQueue) Ack(_ context.Context, msg corequeue.Message) error {
	s.mu.Lock()

	if s.ackPanics > 0 {
		s.ackPanics--
		v := s.ackPanic
		s.mu.Unlock()

		panic(v)
	}

	s.acks = append(s.acks, msg)
	s.mu.Unlock()

	return nil
}

func (s *stubQueue) Nack(_ context.Context, msg corequeue.Message, requeue bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nacks = append(s.nacks, stubNack{msg: msg, requeue: requeue})

	return nil
}

func (s *stubQueue) Length(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (s *stubQueue) IsEmpty(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (s *stubQueue) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closes++

	return s.closeErr
}

func (s *stubQueue) Name() string { return "stub" }

func (s *stubQueue) pushCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.pushes)
}

func (s *stubQueue) delayedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.delayed)
}

func (s *stubQueue) ackCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.acks)
}

func (s *stubQueue) nackCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.nacks)
}

func newTestAdapter(q corequeue.Queue) *adapter {
	a := buildTestAdapter(q)
	a.startWorkers()

	return a
}

// newIdleTestAdapter is newTestAdapter with a worker pool whose goroutines
// never run. Register still assigns events (so serving/assignmentCount
// work), but no background consumer pops the queue or touches a.consecutive
// concurrently with a test that drives consumeOne/handleConsumeError
// directly -- a running worker would reset or bump the counter mid-assertion.
// Each worker's done is pre-closed so Close returns without waiting.
func newIdleTestAdapter(q corequeue.Queue) *adapter {
	a := buildTestAdapter(q)
	a.workers = make([]*worker, DefaultConsumerWorkers)

	for i := range a.workers {
		w := newWorker()
		close(w.done)
		a.workers[i] = w
	}

	return a
}

func buildTestAdapter(q corequeue.Queue) *adapter {
	timeout := 5 * time.Second

	return &adapter{
		regs:            make(map[string]map[string]registration),
		queue:           q,
		timeout:         timeout,
		maxRetries:      3,
		dlqTopic:        "webhook:dead-letter",
		consecutive:     make(map[string]int),
		client:          newSafeClient(timeout, true),
		allowPrivate:    true,
		replayTolerance: defaultReplayTolerance,
	}
}

// serving reports whether event is currently assigned to a pool worker.
func (a *adapter) serving(event string) bool {
	a.mu.RLock()
	workers := a.workers
	a.mu.RUnlock()

	for _, w := range workers {
		if w.hasEvent(event) {
			return true
		}
	}

	return false
}

// assignmentCount returns how many workers currently serve event; an event
// must be owned by exactly one worker.
func (a *adapter) assignmentCount(event string) int {
	a.mu.RLock()
	workers := a.workers
	a.mu.RUnlock()

	n := 0
	for _, w := range workers {
		if w.hasEvent(event) {
			n++
		}
	}

	return n
}

// signPayload builds the "t=<ts>,v1=<hex>" envelope tests use to sign
// requests, mirroring buildSignatureHeader with the current time.
func signPayload(secret string, payload []byte) string {
	return signPayloadAt(secret, payload, time.Now().Unix())
}

// signPayloadAt is signPayload with an explicit timestamp, for replay-window tests.
func signPayloadAt(secret string, payload []byte, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte{'.'})
	mac.Write(payload)

	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// verifyHMAC is a test convenience wrapping verifySignatureHeader with the
// package default tolerance, for callers that only care about pass/fail.
func verifyHMAC(secret string, payload []byte, signature string) bool {
	return verifySignatureHeader(secret, payload, signature, defaultReplayTolerance, time.Now()) == nil
}

func testMessage(topic string, payload []byte, headers map[string]string, attempt int) corequeue.Message {
	return corequeue.Message{
		Topic:   topic,
		Payload: corequeue.Payload(payload),
		Headers: corequeue.Headers(headers),
		Attempt: attempt,
	}
}

func okServer(t *testing.T, secret string, got *sync.Mutex, count *int) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		got.Lock()
		*count++
		got.Unlock()

		if !verifyHMAC(secret, body, r.Header.Get("X-Webhook-Signature")) {
			w.WriteHeader(http.StatusForbidden)

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !cond() {
		t.Fatalf("timed out waiting: %s", msg)
	}
}

func mustOpen(t *testing.T, w webhook.Webhook) *adapter {
	t.Helper()

	a, ok := w.(*adapter)
	if !ok {
		t.Fatal("Open did not return *adapter")
	}

	return a
}

func TestSentinels(t *testing.T) {
	t.Parallel()

	if ErrMissingQueueAdapter.Error() != "queue: queue adapter is required" {
		t.Fatalf("ErrMissingQueueAdapter = %q", ErrMissingQueueAdapter)
	}

	if ErrVisibilityTimeout.Error() != "queue: visibility timeout must be greater than timeout" {
		t.Fatalf("ErrVisibilityTimeout = %q", ErrVisibilityTimeout)
	}
}

func TestOpen_MissingAdapter(t *testing.T) {
	t.Parallel()

	_, err := New(webhook.Options{})
	if !errors.Is(err, ErrMissingQueueAdapter) {
		t.Fatalf("New() err = %v, want ErrMissingQueueAdapter", err)
	}
}

func TestOpen_WhitespaceAdapter(t *testing.T) {
	t.Parallel()

	_, err := New(webhook.Options{QueueAdapter: "   "})
	if !errors.Is(err, ErrMissingQueueAdapter) {
		t.Fatalf("New() err = %v, want ErrMissingQueueAdapter", err)
	}
}

func TestOpen_InvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(webhook.Options{Timeout: -time.Second, QueueAdapter: "memory"})
	if !errors.Is(err, webhook.ErrInvalidOptions) {
		t.Fatalf("New() err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpen_BadVisibility(t *testing.T) {
	t.Parallel()

	o := webhook.Options{
		QueueAdapter: "memory",
		QueueOpts:    corequeue.Options{VisibilityTimeout: -time.Second},
	}

	_, err := New(o)
	if !errors.Is(err, ErrVisibilityTimeout) {
		t.Fatalf("New() err = %v, want ErrVisibilityTimeout", err)
	}
}

func TestOpen_TimeoutGTEVisibility(t *testing.T) {
	t.Parallel()

	o := webhook.Options{
		Timeout:      30 * time.Second,
		QueueAdapter: "memory",
		QueueOpts:    corequeue.Options{VisibilityTimeout: 30 * time.Second},
	}

	_, err := New(o)
	if err == nil || !strings.Contains(err.Error(), "must be less than queue visibility timeout") {
		t.Fatalf("New() err = %v, want visibility complaint", err)
	}
}

func TestOpen_UnknownQueueAdapter(t *testing.T) {
	t.Parallel()

	_, err := New(webhook.Options{QueueAdapter: "bogus"})
	if !errors.Is(err, corequeue.ErrUnknownAdapter) {
		t.Fatalf("New() err = %v, want ErrUnknownAdapter", err)
	}

	if !strings.Contains(err.Error(), "queue: ") {
		t.Fatalf("New() err = %v, want queue prefix", err)
	}
}

func TestOpen_QueueOpenError(t *testing.T) {
	t.Parallel()

	_, err := New(webhook.Options{QueueAdapter: "redis"})
	if !errors.Is(err, errTestTransport) {
		t.Fatalf("New() err = %v, want test transport error", err)
	}

	if strings.Contains(err.Error(), "webhook: open queue") {
		t.Fatalf("New() err = %v, must not contain open queue prefix (returns directly)", err)
	}
}

func TestOpen_Defaults(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{QueueAdapter: "memory"})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	defer func() { _ = w.Close() }()

	a := mustOpen(t, w)
	if a.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", a.timeout)
	}

	if a.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", a.maxRetries)
	}

	if a.dlqTopic != "webhook:dead-letter" {
		t.Errorf("dlqTopic = %q", a.dlqTopic)
	}

	if a.client == nil {
		t.Error("client must be set")
	}

	if a.replayTolerance != defaultReplayTolerance {
		t.Errorf("replayTolerance = %v, want default %v", a.replayTolerance, defaultReplayTolerance)
	}
}

func TestOpen_CustomOptions(t *testing.T) {
	t.Parallel()

	o := webhook.Options{
		Timeout:             5 * time.Second,
		MaxRetries:          1,
		AllowPrivateTargets: true,
		QueueAdapter:        " memory ",
		QueueOpts:           corequeue.Options{VisibilityTimeout: 30 * time.Second},
		DeadLetterTopic:     " custom-dlq ",
		ReplayTolerance:     30 * time.Second,
	}

	w, err := New(o)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	defer func() { _ = w.Close() }()

	a := mustOpen(t, w)
	if a.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", a.timeout)
	}

	if a.maxRetries != 1 {
		t.Errorf("maxRetries = %d, want 1", a.maxRetries)
	}

	if a.dlqTopic != "custom-dlq" {
		t.Errorf("dlqTopic = %q, want trimmed custom", a.dlqTopic)
	}

	if !a.allowPrivate {
		t.Error("allowPrivate = false, want true")
	}

	if a.replayTolerance != 30*time.Second {
		t.Errorf("replayTolerance = %v, want 30s", a.replayTolerance)
	}
}

func TestRegister_Empty(t *testing.T) {
	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	ctx := t.Context()

	if err := a.Register(ctx, "", "http://127.0.0.1:1/x", "s"); err == nil ||
		err.Error() != "queue: event is empty" {
		t.Fatalf("Register empty event err = %v", err)
	}

	if err := a.Register(ctx, "e", "", "s"); err == nil ||
		err.Error() != "queue: target is empty" {
		t.Fatalf("Register empty target err = %v", err)
	}
}

func TestRegister_RejectSyntax(t *testing.T) {
	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	err := a.Register(t.Context(), "e", "ftp://example.com/hook", "s")
	if err == nil || !strings.Contains(err.Error(), "queue: reject target") {
		t.Fatalf("Register ftp err = %v", err)
	}
}

func TestRegister_RejectFullValidation(t *testing.T) {
	sq := newStubQueue()
	a := newTestAdapter(sq)
	a.allowPrivate = false
	defer func() { _ = a.Close() }()

	err := a.Register(t.Context(), "e", "http://example.com/hook", "s")
	if err == nil || !strings.Contains(err.Error(), "queue: reject target") {
		t.Fatalf("Register http without private err = %v", err)
	}
}

func TestRegister_AssignsEventToWorkerPool(t *testing.T) {
	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	ctx := t.Context()
	target := "http://127.0.0.1:1/a"

	if err := a.Register(ctx, "e", target, "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if !a.serving("e") {
		t.Fatal("Register did not assign event to a worker")
	}

	// Idempotent re-register must not duplicate the assignment.
	if err := a.Register(ctx, "e", target, "s2"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	// Second target on the same event reuses the same worker assignment.
	if err := a.Register(ctx, "e", "http://127.0.0.1:1/b", "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if n := a.assignmentCount("e"); n != 1 {
		t.Fatalf("assignments = %d, want 1", n)
	}
}

func TestRegister_ClosedAdapter(t *testing.T) {
	a := newTestAdapter(newStubQueue())

	if err := a.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	if err := a.Register(t.Context(), "e", "http://127.0.0.1:1/a", "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if a.serving("e") {
		t.Fatal("closed adapter assigned event to a worker")
	}
}

func TestUnregister(t *testing.T) {
	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	ctx := t.Context()

	if err := a.Unregister(ctx, "missing", "http://127.0.0.1:1/a"); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Unregister missing event err = %v", err)
	}

	targetA := "http://127.0.0.1:1/a"
	targetB := "http://127.0.0.1:1/b"

	if err := a.Register(ctx, "e", targetA, "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if err := a.Register(ctx, "e", targetB, "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if err := a.Unregister(ctx, "e", "http://127.0.0.1:1/nope"); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Unregister missing target err = %v", err)
	}

	// Partial removal keeps the event assigned.
	if err := a.Unregister(ctx, "e", targetA); err != nil {
		t.Fatalf("Unregister() err = %v", err)
	}

	if !a.serving("e") {
		t.Fatal("event unassigned while targets remain")
	}

	// Last removal prunes the event and unassigns it.
	if err := a.Unregister(ctx, "e", targetB); err != nil {
		t.Fatalf("Unregister() err = %v", err)
	}

	a.mu.RLock()
	_, stillReg := a.regs["e"]
	a.mu.RUnlock()

	if stillReg {
		t.Error("event not pruned")
	}

	if a.serving("e") {
		t.Error("event not unassigned after last unregister")
	}
}

func TestWorker_AddRemoveEvent(t *testing.T) {
	t.Parallel()

	w := newWorker()

	w.addEvent("e")

	if !w.hasEvent("e") {
		t.Fatal("addEvent did not assign")
	}

	w.removeEvent("e")

	if w.hasEvent("e") {
		t.Fatal("removeEvent did not unassign")
	}

	// Signal on an idle worker must not block.
	w.signal()
	w.addEvent("e2")
	w.removeEvent("e2")
}

func TestWorker_StopClosesDone(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	w := newWorker()
	done := make(chan struct{})

	go func() {
		defer close(done)
		a.runWorker(w)
	}()

	close(w.stop)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runWorker did not exit after stop")
	}
}

func TestDeliver_UnknownEvent(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	err := a.Deliver(t.Context(), "nope", []byte(`{}`))
	if !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Deliver() err = %v, want ErrNotFound", err)
	}
}

func TestDeliver_Fanout(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	ctx := t.Context()

	targets := map[string]string{
		"http://127.0.0.1:1/a": "secret-a",
		"http://127.0.0.1:1/b": "secret-b",
	}
	for target, secret := range targets {
		a.regs["e"] = regsWith(a.regs["e"], target, secret)
	}

	payload := []byte(`{"n":1}`)

	if err := a.Deliver(ctx, "e", payload); err != nil {
		t.Fatalf("Deliver() err = %v", err)
	}

	if sq.pushCount() != 2 {
		t.Fatalf("pushes = %d, want 2", sq.pushCount())
	}

	sq.mu.Lock()
	defer sq.mu.Unlock()

	for _, p := range sq.pushes {
		if p.topic != "webhook:e" {
			t.Errorf("topic = %q, want webhook:e", p.topic)
		}

		if p.headers["X-Webhook-Event"] != "e" {
			t.Errorf("event header = %q", p.headers["X-Webhook-Event"])
		}

		secret, ok := targets[p.headers["X-Webhook-Target"]]
		if !ok {
			t.Errorf("unknown target header %q", p.headers["X-Webhook-Target"])

			continue
		}

		if !verifyHMAC(secret, payload, p.headers["X-Webhook-Signature"]) {
			t.Errorf("bad signature for %q", p.headers["X-Webhook-Target"])
		}
	}
}

func regsWith(m map[string]registration, target, secret string) map[string]registration {
	if m == nil {
		m = make(map[string]registration)
	}

	m[target] = registration{target: target, secret: secret}

	return m
}

func TestDeliver_PushErrorJoin(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	sq.pushErr = errTestTransport

	a := newTestAdapter(sq)
	a.regs["e"] = regsWith(nil, "http://127.0.0.1:1/a", "s")
	a.regs["e"] = regsWith(a.regs["e"], "http://127.0.0.1:1/b", "s")

	err := a.Deliver(t.Context(), "e", []byte(`{}`))
	if err == nil {
		t.Fatal("Deliver() = nil, want joined push errors")
	}

	if got := strings.Count(err.Error(), "queue: push to queue"); got != 2 {
		t.Fatalf("Deliver() err = %v, want 2 joined push errors", err)
	}
}

func TestProcessMessage_SuccessAck(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	count := 0
	srv := okServer(t, "s3cret", &mu, &count)

	defer srv.Close()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	target := srv.URL + "/hook"
	a.regs["e"] = regsWith(nil, target, "s3cret")

	payload := []byte(`{"ok":true}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s3cret", payload),
	}, 1)

	a.processMessage("e", msg)

	if sq.ackCount() != 1 {
		t.Fatalf("acks = %d, want 1", sq.ackCount())
	}

	mu.Lock()
	defer mu.Unlock()

	if count != 1 {
		t.Fatalf("server hits = %d, want 1", count)
	}
}

func TestProcessMessage_SignatureMismatchDLQ(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	target := "http://127.0.0.1:1/hook"
	a.regs["e"] = regsWith(nil, target, "s3cret")

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": "bogus",
	}, 1)

	a.processMessage("e", msg)

	assertDeadLetter(t, sq, ErrSignatureMismatch.Error())
}

func assertDeadLetter(t *testing.T, sq *stubQueue, reason string) {
	t.Helper()

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.pushes) != 1 {
		t.Fatalf("dlq pushes = %d, want 1", len(sq.pushes))
	}

	if sq.pushes[0].topic != "webhook:dead-letter" {
		t.Fatalf("dlq topic = %q", sq.pushes[0].topic)
	}

	if sq.pushes[0].headers["X-Webhook-Error"] != reason {
		t.Fatalf("dlq reason = %q, want %q", sq.pushes[0].headers["X-Webhook-Error"], reason)
	}

	if len(sq.acks) != 1 {
		t.Fatalf("acks = %d, want 1", len(sq.acks))
	}
}

func TestProcessMessage_PrivateBlockedDLQ(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	a.allowPrivate = false
	target := "http://127.0.0.1:1/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", payload),
	}, 1)

	a.processMessage("e", msg)

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.pushes) != 1 {
		t.Fatalf("dlq pushes = %d, want 1", len(sq.pushes))
	}

	if !strings.Contains(sq.pushes[0].headers["X-Webhook-Error"], "blocked private target") {
		t.Fatalf("dlq reason = %q, want private block", sq.pushes[0].headers["X-Webhook-Error"])
	}
}

func TestProcessMessage_Status500Requeue(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer srv.Close()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	target := srv.URL + "/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", payload),
	}, 1)

	a.processMessage("e", msg)

	if sq.delayedCount() != 1 {
		t.Fatalf("delayed = %d, want 1", sq.delayedCount())
	}

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if sq.delayed[0].push.headers["X-Webhook-Attempt"] != "2" {
		t.Fatalf("attempt = %q, want 2", sq.delayed[0].push.headers["X-Webhook-Attempt"])
	}

	if len(sq.nacks) != 1 || sq.nacks[0].requeue {
		t.Fatalf("nacks = %+v, want one non-requeue nack", sq.nacks)
	}
}

func TestProcessMessage_ExhaustedDLQ(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer srv.Close()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	target := srv.URL + "/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", payload),
		"X-Webhook-Attempt":   "3",
	}, 1)

	a.processMessage("e", msg)

	assertDeadLetter(t, sq, "target returned status 500")
}

func TestProcessMessage_DoErrorRequeue(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	target := "http://127.0.0.1:1/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", payload),
	}, 1)

	a.processMessage("e", msg)

	if sq.delayedCount() != 1 {
		t.Fatalf("delayed = %d, want 1", sq.delayedCount())
	}

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.nacks) != 1 {
		t.Fatalf("nacks = %d, want 1", len(sq.nacks))
	}
}

func TestProcessMessage_DoErrorExhaustedDLQ(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	target := "http://127.0.0.1:1/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", payload),
		"X-Webhook-Attempt":   "3",
	}, 3)

	a.processMessage("e", msg)

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.pushes) != 1 {
		t.Fatalf("dlq pushes = %d, want 1", len(sq.pushes))
	}

	if !strings.HasPrefix(sq.pushes[0].headers["X-Webhook-Error"], "deliver: ") {
		t.Fatalf("dlq reason = %q, want deliver prefix", sq.pushes[0].headers["X-Webhook-Error"])
	}
}

func TestProcessMessage_BadRequestTarget(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	target := "://bad-url"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", payload),
	}, 1)

	a.processMessage("e", msg)

	if sq.delayedCount() != 1 {
		t.Fatalf("delayed = %d, want 1 requeue on request build error", sq.delayedCount())
	}
}

func TestProcessMessage_DLQRetryShortCircuit(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{
		"X-Webhook-DLQ-Failures": "2",
		"X-Webhook-Error":        "boom",
	}, 1)

	a.processMessage("e", msg)

	assertDeadLetter(t, sq, "boom")
}

func TestProcessMessage_DLQRetryDefaultReason(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{
		"X-Webhook-DLQ-Failures": "1",
	}, 1)

	a.processMessage("e", msg)

	assertDeadLetter(t, sq, "dead-letter push retry")
}

func TestProcessMessage_Unregistered(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{
		"X-Webhook-Target": "http://127.0.0.1:1/x",
	}, 1)

	a.processMessage("missing", msg)
	assertDeadLetter(t, sq, "event not registered")

	sq2 := newStubQueue()
	a.queue = sq2
	a.regs["e"] = regsWith(nil, "http://127.0.0.1:1/other", "s")

	a.processMessage("e", msg)
	assertDeadLetter(t, sq2, "target not registered")
}

func TestSafeProcess_PanicRequeue(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	count := 0
	srv := okServer(t, "s", &mu, &count)

	defer srv.Close()

	sq := newStubQueue()
	sq.ackPanic = "test panic"
	sq.ackPanics = 1
	a := newTestAdapter(sq)
	target := srv.URL + "/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", payload),
	}, 1)

	a.safeProcess("e", msg)

	if sq.delayedCount() != 1 {
		t.Fatalf("delayed = %d, want panic requeue", sq.delayedCount())
	}
}

func TestSafeProcess_PanicAtMaxDLQ(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	count := 0
	srv := okServer(t, "s", &mu, &count)

	defer srv.Close()

	sq := newStubQueue()
	sq.ackPanic = "test panic"
	sq.ackPanics = 1
	a := newTestAdapter(sq)
	target := srv.URL + "/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	msg := testMessage("webhook:e", payload, map[string]string{
		"X-Webhook-Target":    target,
		"X-Webhook-Signature": signPayload("s", payload),
		"X-Webhook-Attempt":   "3",
	}, 3)

	a.safeProcess("e", msg)

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.pushes) != 1 || sq.pushes[0].topic != "webhook:dead-letter" {
		t.Fatalf("pushes = %+v, want one DLQ push", sq.pushes)
	}

	if !strings.Contains(sq.pushes[0].headers["X-Webhook-Error"], "panic: test panic") {
		t.Fatalf("reason = %q, want panic reason", sq.pushes[0].headers["X-Webhook-Error"])
	}
}

func TestHandleConsumeError(t *testing.T) {
	t.Parallel()

	// Idle pool: a live worker serving "e" would pop ErrEmpty from the stub
	// and clear a.consecutive between the assertions below.
	a := newIdleTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	if err := a.Register(t.Context(), "e", "http://127.0.0.1:1/a", "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if !a.serving("e") {
		t.Fatal("event not assigned")
	}

	if a.handleConsumeError("e", corequeue.ErrEmpty) {
		t.Fatal("empty must not stop")
	}

	// Typed empties must behave identically: memory/redis Pop returns
	// *EmptyError on some paths, and bare == would miscount them as
	// transport errors, killing idle consumers after 5 polls.
	if a.handleConsumeError("e", corequeue.EmptyError{Topic: "t"}) {
		t.Fatal("typed empty must not stop")
	}

	if a.handleConsumeError("e", errTestTransport) {
		t.Fatal("first transport error must not stop")
	}

	a.mu.Lock()
	n := a.consecutive["e"]
	a.mu.Unlock()

	if n != 1 {
		t.Fatalf("consecutive = %d, want 1", n)
	}

	for range maxTransportErrors - 2 {
		a.handleConsumeError("e", errTestTransport)
	}

	if !a.handleConsumeError("e", errTestTransport) {
		t.Fatal("cap transport errors must stop")
	}

	if a.serving("e") {
		t.Error("capped event must be unassigned from its worker")
	}
}

func TestSleepWithStop(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	closed := make(chan struct{})
	close(closed)
	a.sleepWithStop(closed, time.Hour)

	a.sleepWithStop(make(chan struct{}), 20*time.Millisecond)
}

func TestRetryDelay_Table(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	for _, attempt := range []int{-5, 0, 1, 2, 10, 100} {
		d := a.retryDelay(attempt)
		if d < 500*time.Millisecond || d > 72*time.Hour {
			t.Fatalf("retryDelay(%d) = %v, out of [500ms, 72h]", attempt, d)
		}
	}

	if got := a.retryDelay(100); got < 54*time.Hour || got > 72*time.Hour {
		t.Fatalf("retryDelay(100) = %v, want capped near 72h", got)
	}

	// Many draws hit both the min clamp and the plain path.
	for range 200 {
		if got := a.retryDelay(1); got < 500*time.Millisecond || got > 750*time.Millisecond {
			t.Fatalf("retryDelay(1) = %v, out of [500ms, 750ms]", got)
		}
	}

	// Many draws hit both the max-delay clamp and the plain path.
	for range 200 {
		if got := a.retryDelay(100); got < 54*time.Hour || got > 72*time.Hour {
			t.Fatalf("retryDelay(100) = %v, want capped near 72h", got)
		}
	}
}

func TestDLQRetryDelay_Range(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	for range 20 {
		d := a.dlqRetryDelay()
		if d < time.Minute || d > time.Minute+30*time.Second {
			t.Fatalf("dlqRetryDelay() = %v, out of [1m, 1m30s]", d)
		}
	}
}

func TestCloneHeaders(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	if got := a.cloneHeaders(nil); got == nil || len(got) != 0 {
		t.Fatalf("cloneHeaders(nil) = %v, want empty map", got)
	}

	src := map[string]string{"a": "1"}
	cp := a.cloneHeaders(src)
	cp["a"] = "2"
	cp["b"] = "3"

	if src["a"] != "1" || len(src) != 1 {
		t.Fatalf("cloneHeaders aliased src: %v", src)
	}
}

func TestAttemptPrecedence(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	cases := []struct {
		headers map[string]string
		attempt int
		want    int
	}{
		{nil, 2, 2},
		{map[string]string{"X-Webhook-Attempt": "9"}, 2, 9},
		{map[string]string{"X-Webhook-Attempt": "1"}, 5, 5},
		{map[string]string{"X-Webhook-Attempt": "abc"}, 4, 4},
	}

	for _, c := range cases {
		msg := testMessage("t", []byte(`{}`), c.headers, c.attempt)

		if got := a.attemptFromHeader(msg); got != c.want {
			t.Errorf("attemptFromHeader(%v, %d) = %d, want %d", c.headers, c.attempt, got, c.want)
		}

		if got := a.effectiveAttempt(msg); got != c.want {
			t.Errorf("effectiveAttempt(%v, %d) = %d, want %d", c.headers, c.attempt, got, c.want)
		}
	}
}

func TestLookupRegistration(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	if _, _, ok := a.lookupRegistration("missing", "x"); ok {
		t.Fatal("lookup missing event = true")
	}

	a.regs["e"] = regsWith(nil, "http://127.0.0.1:1/a", "s")

	if _, _, ok := a.lookupRegistration("e", "http://127.0.0.1:1/b"); ok {
		t.Fatal("lookup missing target = true")
	}

	reg, reason, ok := a.lookupRegistration("e", "http://127.0.0.1:1/a")
	if !ok || reason != "" || reg.secret != "s" {
		t.Fatalf("lookup = %+v %q %v", reg, reason, ok)
	}
}

func TestCheckSignatureUnit(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())
	reg := registration{target: "http://127.0.0.1:1/a", secret: "s"}
	payload := []byte(`{}`)

	if got := a.checkSignature(reg, payload, "nope"); got == "" {
		t.Fatal("checkSignature bad = empty")
	}

	if got := a.checkSignature(reg, payload, signPayload("s", payload)); got != "" {
		t.Fatalf("checkSignature good = %q", got)
	}

	if got := a.checkSignature(reg, payload, "nope"); got != ErrSignatureMismatch.Error() {
		t.Fatalf("checkSignature malformed = %q, want %q", got, ErrSignatureMismatch.Error())
	}

	stale := signPayloadAt("s", payload, time.Now().Add(-defaultReplayTolerance-time.Minute).Unix())
	if got := a.checkSignature(reg, payload, stale); got != ErrSignatureExpired.Error() {
		t.Fatalf("checkSignature stale = %q, want %q", got, ErrSignatureExpired.Error())
	}
}

func TestCheckPrivateTargetUnit(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	if got := a.checkPrivateTarget(t.Context(), registration{target: "x"}); got != "" {
		t.Fatalf("allowPrivate check = %q, want empty", got)
	}

	a.allowPrivate = false

	if got := a.checkPrivateTarget(t.Context(), registration{target: "https://8.8.8.8/hook"}); got != "" {
		t.Fatalf("public IP check = %q, want empty", got)
	}

	if got := a.checkPrivateTarget(t.Context(), registration{target: "http://127.0.0.1:1/x"}); got == "" {
		t.Fatal("private check = empty, want reason")
	}
}

func TestVerifyHMAC_Table(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"a":1}`)
	good := signPayload("s", payload)

	cases := []struct {
		name   string
		secret string
		sig    string
		want   bool
	}{
		{"empty secret", "", good, false},
		{"valid envelope", "s", good, true},
		{"wrong value", "s", "t=1,v1=deadbeef", false},
		{"bad hex", "s", "t=1,v1=!!!", false},
		{"missing v1", "s", "t=1", false},
		{"missing t", "s", "v1=deadbeef", false},
		{"legacy bare hex unsupported", "s", "sha256=deadbeef", false},
		{"empty sig", "s", "", false},
		{"wrong secret", "other", good, false},
	}

	for _, c := range cases {
		if got := verifyHMAC(c.secret, payload, c.sig); got != c.want {
			t.Errorf("%s: verifyHMAC = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestVerifySignatureHeader_ReplayWindow(t *testing.T) {
	t.Parallel()

	secret := "s"
	payload := []byte(`{"a":1}`)
	now := time.Unix(1_700_000_000, 0)
	tolerance := 5 * time.Minute

	// Valid signature within tolerance passes.
	within := signPayloadAt(secret, payload, now.Add(-2*time.Minute).Unix())
	if err := verifySignatureHeader(secret, payload, within, tolerance, now); err != nil {
		t.Errorf("within tolerance: err = %v, want nil", err)
	}

	// Valid signature outside the tolerance window fails with the
	// timestamp-specific error, not the generic mismatch error.
	stale := signPayloadAt(secret, payload, now.Add(-10*time.Minute).Unix())
	if err := verifySignatureHeader(secret, payload, stale, tolerance, now); !errors.Is(err, ErrSignatureExpired) {
		t.Errorf("outside tolerance: err = %v, want ErrSignatureExpired", err)
	}

	future := signPayloadAt(secret, payload, now.Add(10*time.Minute).Unix())
	if err := verifySignatureHeader(secret, payload, future, tolerance, now); !errors.Is(err, ErrSignatureExpired) {
		t.Errorf("future outside tolerance: err = %v, want ErrSignatureExpired", err)
	}

	// A tampered payload fails with the signature-mismatch error even when
	// the timestamp is fresh.
	tamperedSig := signPayloadAt(secret, []byte(`{"a":2}`), now.Unix())
	if err := verifySignatureHeader(secret, payload, tamperedSig, tolerance, now); !errors.Is(err, ErrSignatureMismatch) {
		t.Errorf("tampered payload: err = %v, want ErrSignatureMismatch", err)
	}

	// Round trip: sign then verify end to end.
	roundTrip := buildSignatureHeader(secret, payload, now.Unix())
	if err := verifySignatureHeader(secret, payload, roundTrip, tolerance, now); err != nil {
		t.Errorf("round trip: err = %v, want nil", err)
	}
}

func TestIsDLQRetry(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())

	if a.isDLQRetry(testMessage("t", nil, nil, 1)) {
		t.Fatal("missing header = true")
	}

	if a.isDLQRetry(testMessage("t", nil, map[string]string{"X-Webhook-DLQ-Failures": "0"}, 1)) {
		t.Fatal("zero failures = true")
	}

	if a.isDLQRetry(testMessage("t", nil, map[string]string{"X-Webhook-DLQ-Failures": "abc"}, 1)) {
		t.Fatal("bad header = true")
	}

	if !a.isDLQRetry(testMessage("t", nil, map[string]string{"X-Webhook-DLQ-Failures": "2"}, 1)) {
		t.Fatal("failures = false")
	}
}

func TestRequeue_Success(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{"k": "v"}, 1)
	a.requeue("e", msg, 1)

	if sq.delayedCount() != 1 {
		t.Fatalf("delayed = %d, want 1", sq.delayedCount())
	}

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if sq.delayed[0].push.headers["X-Webhook-Attempt"] != "2" {
		t.Fatalf("attempt = %q, want 2", sq.delayed[0].push.headers["X-Webhook-Attempt"])
	}

	if sq.delayed[0].push.headers["k"] != "v" {
		t.Fatal("headers not cloned")
	}

	if sq.delayed[0].delay <= 0 {
		t.Fatal("delay must be positive")
	}

	if len(sq.nacks) != 1 || sq.nacks[0].requeue {
		t.Fatalf("nacks = %+v", sq.nacks)
	}
}

func TestRequeue_PushFailFallsBack(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	sq.pushDelayedScript = []error{errTestTransport, nil}

	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{"X-Webhook-Attempt": "1"}, 1)
	a.requeue("e", msg, 1)

	if sq.delayedCount() != 2 {
		t.Fatalf("delayed = %d, want fallback push", sq.delayedCount())
	}

	if sq.nackCount() != 1 {
		t.Fatalf("nacks = %d, want 1", sq.nackCount())
	}
}

func TestDelayedRequeue_DropAtCap(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{"X-Webhook-Attempt": "99"}, 99)
	a.delayedRequeue("e", msg, nil)

	if sq.delayedCount() != 0 {
		t.Fatalf("delayed = %d, want drop", sq.delayedCount())
	}

	if sq.nackCount() != 1 {
		t.Fatalf("nacks = %d, want 1", sq.nackCount())
	}
}

func TestDelayedRequeue_PushFailLeavesInflight(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	sq.pushDelayedScript = []error{errTestTransport}

	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{"X-Webhook-Attempt": "1"}, 1)
	a.delayedRequeue("e", msg, map[string]string{"X-Webhook-Attempt": "2"})

	if sq.nackCount() != 0 {
		t.Fatalf("nacks = %d, want 0 (left in-flight)", sq.nackCount())
	}
}

func TestDeadLetter_Success(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), map[string]string{"k": "v"}, 3)
	a.deadLetter("e", msg, "boom")

	assertDeadLetter(t, sq, "boom")
}

func TestDeadLetter_PushFailRetries(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	sq.pushErr = errTestTransport

	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), nil, 3)
	a.deadLetter("e", msg, "boom")

	if sq.delayedCount() != 1 {
		t.Fatalf("delayed = %d, want retry", sq.delayedCount())
	}

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if sq.delayed[0].push.headers["X-Webhook-DLQ-Failures"] != "1" {
		t.Fatalf("failures = %q, want 1", sq.delayed[0].push.headers["X-Webhook-DLQ-Failures"])
	}

	if len(sq.nacks) != 1 {
		t.Fatalf("nacks = %d, want 1", len(sq.nacks))
	}
}

func TestRetryDeadLetter_CapDrops(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	headers := map[string]string{
		"X-Webhook-Error":        "boom",
		"X-Webhook-DLQ-Failures": "2",
	}
	msg := testMessage("webhook:e", []byte(`{}`), nil, 1)
	a.retryDeadLetter("e", msg, headers)

	if headers["X-Webhook-DLQ-Failures"] != "3" {
		t.Fatalf("failures = %q, want 3", headers["X-Webhook-DLQ-Failures"])
	}

	if sq.delayedCount() != 0 {
		t.Fatalf("delayed = %d, want drop", sq.delayedCount())
	}

	if sq.nackCount() != 1 {
		t.Fatalf("nacks = %d, want 1", sq.nackCount())
	}
}

func TestRetryDeadLetter_RetryPushFail(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	sq.pushDelayedScript = []error{errTestTransport}

	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), nil, 1)
	a.retryDeadLetter("e", msg, map[string]string{"X-Webhook-DLQ-Failures": "1"})

	if sq.delayedCount() != 1 {
		t.Fatalf("delayed = %d, want attempted push", sq.delayedCount())
	}

	if sq.nackCount() != 0 {
		t.Fatalf("nacks = %d, want 0 (left in-flight)", sq.nackCount())
	}
}

func TestStartWorkers_BoundedPool(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	if len(a.workers) != DefaultConsumerWorkers {
		t.Fatalf("workers = %d, want %d", len(a.workers), DefaultConsumerWorkers)
	}
}

func TestRegister_DistributesEventsAcrossWorkers(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	const events = 20

	assigned := 0

	for i := range events {
		event := fmt.Sprintf("e%d", i)
		if err := a.Register(t.Context(), event, "http://127.0.0.1:1/a", "s"); err != nil {
			t.Fatalf("Register(%s) err = %v", event, err)
		}
	}

	for _, w := range a.workers {
		assigned += w.eventCount()
	}

	if assigned != events {
		t.Fatalf("assigned = %d, want %d", assigned, events)
	}

	for i := range events {
		if !a.serving(fmt.Sprintf("e%d", i)) {
			t.Fatalf("event e%d not assigned to any worker", i)
		}

		if n := a.assignmentCount(fmt.Sprintf("e%d", i)); n != 1 {
			t.Fatalf("event e%d assignments = %d, want 1", i, n)
		}
	}
}

func TestConsumeOne_Success(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	count := 0
	srv := okServer(t, "s", &mu, &count)

	defer srv.Close()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	defer func() { _ = a.Close() }()

	target := srv.URL + "/hook"
	a.regs["e"] = regsWith(nil, target, "s")

	payload := []byte(`{}`)
	sq.popScript = []popOut{
		{msg: testMessage("webhook:e", payload, map[string]string{
			"X-Webhook-Target":    target,
			"X-Webhook-Signature": signPayload("s", payload),
		}, 1)},
	}

	if !a.consumeOne("e") {
		t.Fatal("consumeOne reported no work for a queued message")
	}

	waitFor(t, 5*time.Second, func() bool { return sq.ackCount() == 1 }, "ack")
}

func TestConsumeOne_Empty(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(newStubQueue())
	defer func() { _ = a.Close() }()

	if a.consumeOne("e") {
		t.Fatal("consumeOne reported work for an empty queue")
	}
}

func TestConsumeOne_TransportErrorKeepsEventAssigned(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	sq.popErr = errTestTransport

	// Idle pool: a live worker serving "e" would hit the same transport
	// error on every poll and could reach the cap (unassigning "e") before
	// the serving assertion runs.
	a := newIdleTestAdapter(sq)
	defer func() { _ = a.Close() }()

	if err := a.Register(t.Context(), "e", "http://127.0.0.1:1/a", "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if a.consumeOne("e") {
		t.Fatal("consumeOne reported work for a failed pop")
	}

	if !a.serving("e") {
		t.Fatal("event unassigned after a single transport error")
	}
}

func TestWorker_ServesAssignedEvent(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	count := 0
	srv := okServer(t, "s", &mu, &count)

	defer srv.Close()

	sq := newStubQueue()

	target := srv.URL + "/hook"

	payload := []byte(`{}`)
	sq.popScript = []popOut{
		{msg: testMessage("webhook:e", payload, map[string]string{
			"X-Webhook-Target":    target,
			"X-Webhook-Signature": signPayload("s", payload),
		}, 1)},
	}

	// Script the pop before the adapter starts its workers: a worker must
	// never observe the popScript slice mid-write.
	a := newTestAdapter(sq)

	defer func() { _ = a.Close() }()

	if err := a.Register(t.Context(), "e", target, "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	waitFor(t, 5*time.Second, func() bool { return sq.ackCount() == 1 }, "ack")
}

func TestClose_Idempotent(t *testing.T) {
	sq := newStubQueue()
	a := newTestAdapter(sq)

	ctx := t.Context()
	if err := a.Register(ctx, "e", "http://127.0.0.1:1/a", "s"); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("second Close() err = %v", err)
	}

	if sq.closes != 1 {
		t.Fatalf("queue closes = %d, want 1", sq.closes)
	}
}

func TestClose_QueueError(t *testing.T) {
	sq := newStubQueue()
	sq.closeErr = errTestTransport

	a := newTestAdapter(sq)

	err := a.Close()
	if !errors.Is(err, errTestTransport) {
		t.Fatalf("Close() err = %v, want transport error", err)
	}

	if !strings.Contains(err.Error(), "queue: close queue") {
		t.Fatalf("Close() err = %v, want close prefix", err)
	}
}

func TestClose_DeadlineOverrun(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)
	a.timeout = -6 * time.Second

	if err := a.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	if sq.closes != 1 {
		t.Fatalf("queue closes = %d, want 1", sq.closes)
	}
}

func TestEndToEnd_MemoryQueue(t *testing.T) {
	var mu sync.Mutex

	var (
		hits      int
		eventSeen string
	)

	const secret = "e2e-secret"

	target := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		hits++

		eventSeen = r.Header.Get("X-Webhook-Event")
		if !verifyHMAC(secret, body, r.Header.Get("X-Webhook-Signature")) {
			mu.Unlock()
			w.WriteHeader(http.StatusForbidden)

			return
		}

		if r.Header.Get("X-Webhook-Target") != target {
			mu.Unlock()
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))

	defer srv.Close()

	target = srv.URL + "/hook"

	memQ, err := memory.New(corequeue.Options{
		VisibilityTimeout: 30 * time.Second,
		PollTimeout:       20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("memory.New() err = %v", err)
	}

	timeout := 2 * time.Second
	a := &adapter{
		regs:         make(map[string]map[string]registration),
		queue:        memQ,
		timeout:      timeout,
		maxRetries:   3,
		dlqTopic:     "webhook:dead-letter",
		consecutive:  make(map[string]int),
		client:       newSafeClient(timeout, true),
		allowPrivate: true,
	}

	a.startWorkers()

	ctx := t.Context()
	if err := a.Register(ctx, "deploy", target, secret); err != nil {
		t.Fatalf("Register() err = %v", err)
	}

	payload := []byte(`{"ok":true}`)
	if err := a.Deliver(ctx, "deploy", payload); err != nil {
		t.Fatalf("Deliver() err = %v", err)
	}

	waitFor(t, 10*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()

		n, lerr := memQ.Length(ctx, "webhook:deploy")
		if lerr != nil {
			return false
		}

		return hits == 1 && n == 0 && eventSeen == "deploy"
	}, "end-to-end delivery")

	if err := a.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}
}

func TestHandleDeliveryError_Exhausted(t *testing.T) {
	t.Parallel()

	sq := newStubQueue()
	a := newTestAdapter(sq)

	msg := testMessage("webhook:e", []byte(`{}`), nil, 3)
	a.handleDeliveryError("e", msg, 3, "boom")

	assertDeadLetter(t, sq, "boom")
}
