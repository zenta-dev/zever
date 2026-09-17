package queue

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/log/noop"
	"github.com/zenta-dev/zever/queue"
	"github.com/zenta-dev/zever/webhook"
)

const (
	dlqRetryMinDuration = time.Minute
	dlqRetryJitter      = 30 * time.Second
	maxDLQFailures      = 3
	maxTransportErrors  = 5
)

type registration struct {
	target string
	secret string
}

type consumer struct {
	stop    chan struct{}
	done    chan struct{}
	stopped bool
}

type adapter struct {
	mu           sync.RWMutex
	regs         map[string]map[string]registration
	queue        queue.Queue
	timeout      time.Duration
	maxRetries   int
	dlqTopic     string
	consumers    map[string]*consumer
	closed       bool
	client       *http.Client
	jitter       *rand.Rand
	jitterMu     sync.Mutex
	allowPrivate bool
	logger       log.Logger
}

func (a *adapter) log() log.Logger {
	if a != nil && a.logger != nil {
		return a.logger
	}

	return noop.New()
}

func (a *adapter) Register(_ context.Context, event, target, secret string) error {
	if event == "" {
		return errors.New("queue: event is empty")
	}

	if target == "" {
		return errors.New("queue: target is empty")
	}

	var err error
	if a.allowPrivate {
		err = webhook.ValidateTargetSyntax(target)
	} else {
		err = webhook.ValidateTarget(target)
	}

	if err != nil {
		return fmt.Errorf("queue: reject target %q: %w", target, err)
	}

	a.mu.Lock()

	newEvent := false

	if _, ok := a.regs[event]; !ok {
		a.regs[event] = make(map[string]registration)
		newEvent = true
	}

	a.regs[event][target] = registration{target: target, secret: secret}
	a.mu.Unlock()

	if newEvent {
		a.startConsumer(event)
	}

	return nil
}

func (a *adapter) startConsumer(event string) {
	a.mu.Lock()

	if a.closed {
		a.mu.Unlock()
		return
	}

	c, ok := a.consumers[event]
	if ok && !c.stopped {
		select {
		case <-c.done:
		default:
			a.mu.Unlock()
			return
		}
	}

	a.mu.Unlock()

	if ok {
		a.waitForConsumerDone(c, event)
	}

	a.mu.Lock()

	if a.closed {
		a.mu.Unlock()
		return
	}

	if _, stillReg := a.regs[event]; !stillReg {
		a.mu.Unlock()
		return
	}

	if c2, ok2 := a.consumers[event]; ok2 {
		if !c2.stopped {
			select {
			case <-c2.done:
			default:
				a.mu.Unlock()
				return
			}
		}

		delete(a.consumers, event)
	}

	c = &consumer{stop: make(chan struct{}), done: make(chan struct{})}
	a.consumers[event] = c
	a.mu.Unlock()

	go a.consume(event, c)
}

func (a *adapter) waitForConsumerDone(c *consumer, event string) {
	select {
	case <-c.done:
	case <-time.After(a.timeout + 6*time.Second):
		a.log().Warn().Str("event", event).Msg("webhook: consumer did not stop in time")
	}
}

func (a *adapter) stopConsumerLocked(event string) {
	c, ok := a.consumers[event]
	if !ok || c.stopped {
		return
	}

	c.stopped = true
	close(c.stop)
}

// consume runs the per-event delivery loop.
// It uses context.Background by design: the loop outlives any single caller
// request, and each queue/process operation derives its own bounded timeout.
func (a *adapter) consume(event string, c *consumer) {
	defer close(c.done)

	topic := "webhook:" + event
	consecutiveErrors := 0

	for {
		select {
		case <-c.stop:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		msg, err := a.queue.Pop(ctx, topic)

		cancel()

		if err != nil {
			if a.handleConsumeError(event, err, &consecutiveErrors, c) {
				return
			}

			continue
		}

		consecutiveErrors = 0

		a.safeProcess(event, msg)
	}
}

func (a *adapter) handleConsumeError(event string, err error, consecutive *int, c *consumer) bool {
	// errors.Is (not ==): memory/redis Pop may return *EmptyError, which only
	// matches via Unwrap. Bare == miscounted idle polls as transport errors
	// and killed consumers after 5 empties.
	if errors.Is(err, queue.ErrEmpty) {
		*consecutive = 0

		a.sleepWithStop(c.stop, 100*time.Millisecond)

		return false
	}

	*consecutive++

	a.log().Warn().Str("event", event).Err(err).Msg("webhook: consumer pop failed")

	if *consecutive >= maxTransportErrors {
		a.log().Warn().Str("event", event).Int("consecutive", *consecutive).Msg("webhook: consumer stopped after consecutive queue transport errors; close and re-open the webhook to resume")

		return true
	}

	a.sleepWithStop(c.stop, 200*time.Millisecond)

	return false
}

func (a *adapter) sleepWithStop(stop <-chan struct{}, d time.Duration) {
	select {
	case <-stop:
	case <-time.After(d):
	}
}

func (a *adapter) safeProcess(event string, msg queue.Message) {
	defer func() {
		if r := recover(); r != nil {
			a.log().Warn().Str("event", event).Any("panic", r).Msg("webhook: consumer panic")

			attempt := a.attemptFromHeader(msg)
			reason := fmt.Sprintf("panic: %v", r)

			if attempt >= a.maxRetries {
				a.deadLetter(event, msg, reason)

				return
			}

			a.requeue(event, msg, attempt)
		}
	}()

	a.processMessage(event, msg)
}

func (a *adapter) processMessage(event string, msg queue.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	if a.isDLQRetry(msg) {
		a.deadLetter(event, msg, a.dlqErrorReason(msg))
		return
	}

	reg, reason, ok := a.lookupRegistration(event, msg.Headers["X-Webhook-Target"])
	if !ok {
		a.deadLetter(event, msg, reason)
		return
	}

	sig := msg.Headers["X-Webhook-Signature"]
	if msgReason := a.checkSignature(reg, msg.Payload, sig); msgReason != "" {
		a.deadLetter(event, msg, msgReason)
		return
	}

	if msgReason := a.checkPrivateTarget(ctx, reg); msgReason != "" {
		a.deadLetter(event, msg, msgReason)
		return
	}

	attempt := a.effectiveAttempt(msg)

	status, reqErr := a.doHTTPRequest(ctx, reg, event, msg.Payload, sig)
	if reqErr != nil {
		a.handleDeliveryError(event, msg, attempt, "deliver: "+reqErr.Error())
		return
	}

	if status >= 200 && status < 300 {
		_ = a.queue.Ack(ctx, msg)
		return
	}

	a.handleDeliveryError(event, msg, attempt, fmt.Sprintf("target returned status %d", status))
}

func (a *adapter) isDLQRetry(msg queue.Message) bool {
	v, err := strconv.Atoi(msg.Headers["X-Webhook-DLQ-Failures"])
	return err == nil && v > 0
}

func (a *adapter) dlqErrorReason(msg queue.Message) string {
	reason := msg.Headers["X-Webhook-Error"]
	if reason == "" {
		reason = "dead-letter push retry"
	}

	return reason
}

func (a *adapter) lookupRegistration(event, targetURL string) (registration, string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	targets, ok := a.regs[event]
	if !ok {
		return registration{}, "event not registered", false
	}

	reg, found := targets[targetURL]
	if !found {
		return registration{}, "target not registered", false
	}

	return reg, "", true
}

func (a *adapter) checkSignature(reg registration, payload []byte, sig string) string {
	if !verifyHMAC(reg.secret, payload, sig) {
		return "signature mismatch (tampered or replayed)"
	}

	return ""
}

func (a *adapter) checkPrivateTarget(ctx context.Context, reg registration) string {
	if a.allowPrivate {
		return ""
	}

	if err := webhook.ValidateTargetContext(ctx, reg.target); err != nil {
		return fmt.Sprintf("blocked private target: %v", err)
	}

	return ""
}

func (a *adapter) effectiveAttempt(msg queue.Message) int {
	attempt := msg.Attempt
	if v, err := strconv.Atoi(msg.Headers["X-Webhook-Attempt"]); err == nil && v > attempt {
		attempt = v
	}

	return attempt
}

func (a *adapter) doHTTPRequest(
	ctx context.Context,
	reg registration,
	event string,
	payload []byte,
	sig string,
) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reg.target, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", event)
	req.Header.Set("X-Webhook-Signature", sig)

	resp, err := a.client.Do(req)
	if err != nil {
		return 0, err
	}

	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	return resp.StatusCode, nil
}

func (a *adapter) handleDeliveryError(event string, msg queue.Message, attempt int, reason string) {
	if attempt >= a.maxRetries {
		a.deadLetter(event, msg, reason)
		return
	}

	a.requeue(event, msg, attempt)
}

func (a *adapter) retryDelay(attempt int) time.Duration {
	const (
		base     = 500 * time.Millisecond
		maxDelay = 72 * time.Hour
	)

	if attempt <= 0 {
		attempt = 1
	}

	shift := attempt - 1
	if shift > 20 {
		shift = 20
	}

	d := base * time.Duration(1<<uint(shift))
	if d > maxDelay || d <= 0 {
		d = maxDelay
	}

	a.jitterMu.Lock()
	n := a.jitter.Int63n(int64(d/2) + 1)
	a.jitterMu.Unlock()

	jittered := d - d/4 + time.Duration(n)
	if jittered > maxDelay {
		jittered = maxDelay
	}

	if jittered < base {
		jittered = base
	}

	return jittered
}

func (a *adapter) dlqRetryDelay() time.Duration {
	a.jitterMu.Lock()
	defer a.jitterMu.Unlock()

	return dlqRetryMinDuration + time.Duration(a.jitter.Int63n(int64(dlqRetryJitter)+1))
}

func (a *adapter) cloneHeaders(src map[string]string) map[string]string {
	if src == nil {
		return make(map[string]string)
	}

	cp := make(map[string]string, len(src)+1)
	for k, v := range src {
		cp[k] = v
	}

	return cp
}

func (a *adapter) attemptFromHeader(msg queue.Message) int {
	attempt := msg.Attempt
	if v, err := strconv.Atoi(msg.Headers["X-Webhook-Attempt"]); err == nil && v > attempt {
		attempt = v
	}

	return attempt
}

func (a *adapter) requeue(event string, msg queue.Message, attempt int) {
	attempt++

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	headers := a.cloneHeaders(msg.Headers)
	headers["X-Webhook-Attempt"] = strconv.Itoa(attempt)

	if err := a.queue.PushDelayed(ctx, msg.Topic, msg.Payload, headers, a.retryDelay(attempt)); err != nil {
		a.log().Warn().Str("event", event).Err(err).Msg("webhook: requeue push failed")
		a.delayedRequeue(event, msg, headers)

		return
	}

	_ = a.queue.Nack(ctx, msg, false)
}

func (a *adapter) delayedRequeue(event string, msg queue.Message, headers map[string]string) {
	if headers == nil {
		headers = make(map[string]string)
	}

	attempt := a.attemptFromHeader(msg)
	if attempt > a.maxRetries {
		ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
		defer cancel()

		_ = a.queue.Nack(ctx, msg, false)
		a.log().Warn().Str("event", event).Int("attempt", attempt).Int("max_retries", a.maxRetries).Msg("webhook: delayed requeue dropped: DLQ unreachable")

		return
	}

	attempt = min(attempt+1, a.maxRetries)
	headers["X-Webhook-Attempt"] = strconv.Itoa(attempt)

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	if err := a.queue.PushDelayed(ctx, msg.Topic, msg.Payload, headers, a.dlqRetryDelay()); err != nil {
		a.log().Warn().Str("event", event).Err(err).Msg("webhook: delayed requeue push failed; leaving message in-flight for visibility redelivery")

		return
	}

	_ = a.queue.Nack(ctx, msg, false)
}

func (a *adapter) deadLetter(event string, msg queue.Message, reason string) {
	headers := a.cloneHeaders(msg.Headers)
	headers["X-Webhook-Error"] = reason

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	if err := a.queue.Push(ctx, a.dlqTopic, msg.Payload, headers); err != nil {
		a.log().Warn().Str("event", event).Err(err).Msg("webhook: dead-letter push failed")
		a.retryDeadLetter(event, msg, headers)

		return
	}

	_ = a.queue.Ack(ctx, msg)
}

func (a *adapter) retryDeadLetter(event string, msg queue.Message, headers map[string]string) {
	failures := 1
	if v, err := strconv.Atoi(headers["X-Webhook-DLQ-Failures"]); err == nil && v >= 1 {
		failures = v + 1
	}

	headers["X-Webhook-DLQ-Failures"] = strconv.Itoa(failures)

	if failures >= maxDLQFailures {
		ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
		defer cancel()

		_ = a.queue.Nack(ctx, msg, false)

		a.log().Warn().Str("event", event).Int("failures", failures).Str("reason", headers["X-Webhook-Error"]).Msg("webhook: dropped message after consecutive dead-letter push failures (DLQ unreachable)")

		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	if err := a.queue.PushDelayed(ctx, msg.Topic, msg.Payload, headers, a.dlqRetryDelay()); err != nil {
		a.log().Warn().Str("event", event).Err(err).Msg("webhook: dead-letter retry push failed; leaving message in-flight for visibility redelivery")

		return
	}

	_ = a.queue.Nack(ctx, msg, false)
}

func (a *adapter) Unregister(_ context.Context, event, target string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	targets, ok := a.regs[event]
	if !ok {
		return webhook.ErrNotFound
	}

	if _, ok := targets[target]; !ok {
		return webhook.ErrNotFound
	}

	delete(targets, target)

	if len(targets) == 0 {
		delete(a.regs, event)
		a.stopConsumerLocked(event)
	}

	return nil
}

func (a *adapter) Deliver(ctx context.Context, event string, payload []byte) error {
	a.mu.RLock()

	targets, ok := a.regs[event]
	if !ok {
		a.mu.RUnlock()
		return fmt.Errorf("queue: %w", webhook.ErrNotFound)
	}

	regsCopy := make([]registration, 0, len(targets))
	for _, r := range targets {
		regsCopy = append(regsCopy, r)
	}

	a.mu.RUnlock()

	var errs []error

	for _, r := range regsCopy {
		mac := hmac.New(sha256.New, []byte(r.secret))
		mac.Write(payload)
		sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		headers := map[string]string{
			"X-Webhook-Event":     event,
			"X-Webhook-Signature": sig,
			"X-Webhook-Target":    r.target,
		}

		if err := a.queue.Push(ctx, "webhook:"+event, payload, headers); err != nil {
			errs = append(errs, fmt.Errorf("queue: push to queue: %w", err))
		}
	}

	return errors.Join(errs...)
}

func (a *adapter) Close() error {
	a.mu.Lock()

	if a.closed {
		a.mu.Unlock()
		return nil
	}

	a.closed = true

	for event := range a.consumers {
		a.stopConsumerLocked(event)
	}

	consumers := a.consumers
	a.consumers = make(map[string]*consumer)
	a.mu.Unlock()

	deadline := time.After(a.timeout + 6*time.Second)

	for event, c := range consumers {
		select {
		case <-c.done:
		case <-deadline:
			a.log().Warn().Str("event", event).Msg("webhook: consumer did not stop in time; closing queue anyway")
		}
	}

	if err := a.queue.Close(); err != nil {
		return fmt.Errorf("queue: close queue: %w", err)
	}

	return nil
}

func verifyHMAC(secret string, payload []byte, signature string) bool {
	if secret == "" {
		return false
	}

	sigHex := strings.TrimPrefix(signature, "sha256=")

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(expected, sigBytes)
}

// newJitter returns a dedicated random source for retry backoff jitter.
// A dedicated source keeps webhook retries from perturbing the shared generator.
func newJitter() *rand.Rand {
	//nolint:gosec // G404: math/rand suffices for retry jitter, which is not security-sensitive.
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func newSafeClient(timeout time.Duration, allowPrivate bool) *http.Client {
	return webhook.NewSafeClient(timeout, allowPrivate)
}

// Open builds a queue-backed Webhook from o.
// QueueAdapter names a registered queue backend and QueueOpts carries its settings.
func Open(o webhook.Options) (webhook.Webhook, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	queueAdapter := strings.TrimSpace(o.QueueAdapter)
	if queueAdapter == "" {
		return nil, ErrMissingQueueAdapter
	}

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	maxRetries := o.MaxRetries
	if maxRetries < 1 {
		maxRetries = 3
	}

	dlqTopic := "webhook:dead-letter"
	if v := strings.TrimSpace(o.DeadLetterTopic); v != "" {
		dlqTopic = v
	}

	visibility := o.QueueOpts.VisibilityTimeout
	if visibility == 0 {
		visibility = 30 * time.Second
	}

	if visibility <= 0 {
		return nil, ErrVisibilityTimeout
	}

	if timeout >= visibility {
		return nil, fmt.Errorf(
			"queue: option %q (%s) must be less than queue visibility timeout (%s); duplicate delivery otherwise",
			"timeout", timeout, visibility,
		)
	}

	qa, err := queue.ParseAdapter(queueAdapter)
	if err != nil {
		return nil, fmt.Errorf("queue: %w", err)
	}

	q, err := queue.Open(qa, o.QueueOpts)
	if err != nil {
		return nil, fmt.Errorf("queue: open queue: %w", err)
	}

	return &adapter{
		regs:         make(map[string]map[string]registration),
		queue:        q,
		timeout:      timeout,
		maxRetries:   maxRetries,
		dlqTopic:     dlqTopic,
		consumers:    make(map[string]*consumer),
		client:       newSafeClient(timeout, o.AllowPrivateTargets),
		jitter:       newJitter(),
		allowPrivate: o.AllowPrivateTargets,
		logger:       o.Logger,
	}, nil
}
