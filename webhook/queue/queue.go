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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenta-dev/zever/internal/retry"
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

	// defaultReplayTolerance is the window either side of "now" that a
	// signature's embedded timestamp may fall in before verification treats
	// it as expired or replayed. Options.ReplayTolerance overrides it; zero
	// or negative there falls back to this default.
	defaultReplayTolerance = 5 * time.Minute
)

// DefaultConsumerStopSlack is the grace period beyond the operation timeout for consumer shutdown.
// DefaultPopTimeout bounds each queue pop operation.
// DefaultIdlePollInterval is the pause between empty queue polls.
// DefaultTransportBackoff is the pause between queue transport failures.
// DefaultRetryBaseDelay is the initial redelivery backoff delay.
// DefaultRetryMaxDelay caps the redelivery backoff delay.
// DefaultTimeout is the per-operation timeout used when Options.Timeout is zero.
// DefaultVisibilityTimeout is the queue visibility timeout used when QueueOpts leaves it zero.
const (
	DefaultConsumerStopSlack = 6 * time.Second
	DefaultPopTimeout        = 5 * time.Second
	DefaultIdlePollInterval  = 100 * time.Millisecond
	DefaultTransportBackoff  = 200 * time.Millisecond
	DefaultRetryBaseDelay    = 500 * time.Millisecond
	DefaultRetryMaxDelay     = 72 * time.Hour
	DefaultTimeout           = 10 * time.Second
	DefaultVisibilityTimeout = 30 * time.Second
)

// retryPolicy computes the delay before redelivering a failed webhook:
// exponential backoff from 500ms, capped at 72h, jittered +/-25%.
var retryPolicy = retry.Policy{
	BaseDelay:  DefaultRetryBaseDelay,
	Multiplier: 2,
	MaxDelay:   DefaultRetryMaxDelay,
	Jitter:     0.25,
	JitterMode: retry.JitterSymmetric,
}

// dlqRetryPolicy computes the delay before retrying a dead-lettered
// delivery: a flat minimum plus up to dlqRetryJitter of additive jitter.
var dlqRetryPolicy = retry.Policy{
	BaseDelay:  dlqRetryMinDuration,
	JitterMode: retry.JitterFlat,
	JitterMax:  dlqRetryJitter,
}

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
	mu              sync.RWMutex
	regs            map[string]map[string]registration
	queue           queue.Queue
	timeout         time.Duration
	maxRetries      int
	dlqTopic        string
	consumers       map[string]*consumer
	closed          bool
	client          *http.Client
	allowPrivate    bool
	logger          log.Logger
	replayTolerance time.Duration
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
		err = webhook.ValidateTarget(target) //nolint:contextcheck // validation has no context dependency, matching webhook/http's Register
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
		a.startConsumer(event) //nolint:contextcheck // spawns a long-running background consumer that must outlive this Register call, so it deliberately takes no context
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
	case <-time.After(a.timeout + DefaultConsumerStopSlack):
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

		ctx, cancel := context.WithTimeout(context.Background(), DefaultPopTimeout)
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

		a.sleepWithStop(c.stop, DefaultIdlePollInterval)

		return false
	}

	*consecutive++

	a.log().Warn().Str("event", event).Err(err).Msg("webhook: consumer pop failed")

	if *consecutive >= maxTransportErrors {
		a.log().Warn().Str("event", event).Int("consecutive", *consecutive).Msg("webhook: consumer stopped after consecutive queue transport errors; close and re-open the webhook to resume")

		return true
	}

	a.sleepWithStop(c.stop, DefaultTransportBackoff)

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
	tolerance := a.replayTolerance
	if tolerance <= 0 {
		tolerance = defaultReplayTolerance
	}

	if err := verifySignatureHeader(reg.secret, payload, sig, tolerance, time.Now()); err != nil {
		return err.Error()
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
	// NextDelay's symmetric jitter can dip below BaseDelay (e.g. attempt 1
	// jittered down by up to 25%); re-floor so the delay never drops below
	// the configured base, matching the original unjittered floor.
	d := retryPolicy.NextDelay(attempt)
	if d < retryPolicy.BaseDelay {
		d = retryPolicy.BaseDelay
	}

	if d > retryPolicy.MaxDelay {
		d = retryPolicy.MaxDelay
	}

	return d
}

func (a *adapter) dlqRetryDelay() time.Duration {
	return dlqRetryPolicy.NextDelay(1)
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
		sig := buildSignatureHeader(r.secret, payload, time.Now().Unix())

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

	deadline := time.After(a.timeout + DefaultConsumerStopSlack)

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

// buildSignatureHeader returns the envelope for payload signed at ts:
// "t=<unix-timestamp>,v1=<hex-hmac>", where the hex-hmac is HMAC-SHA256 over
// "<unix-timestamp>.<payload>". webhook/http builds the same envelope
// independently (it has no reason to import this package); the two must stay
// byte-for-byte consistent.
func buildSignatureHeader(secret string, payload []byte, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte{'.'})
	mac.Write(payload)

	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// parseSignatureHeader splits a "t=<ts>,v1=<hex>" envelope into its
// timestamp and hex-encoded MAC. ok is false when either field is missing or
// the timestamp does not parse.
func parseSignatureHeader(header string) (ts int64, macHex string, ok bool) {
	for _, part := range strings.Split(header, ",") {
		key, val, found := strings.Cut(part, "=")
		if !found {
			continue
		}

		switch key {
		case "t":
			v, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return 0, "", false
			}

			ts = v
		case "v1":
			macHex = val
		}
	}

	if macHex == "" {
		return 0, "", false
	}

	return ts, macHex, true
}

// verifySignatureHeader verifies header against payload under secret.
// It recomputes the HMAC over "<timestamp-from-header>.<payload>" and
// compares with hmac.Equal, then separately checks that timestamp falls
// within tolerance of now. The two checks are reported through distinct
// sentinel errors so callers can tell a tampered payload (ErrSignatureMismatch)
// from an expired or replayed one (ErrSignatureExpired).
func verifySignatureHeader(secret string, payload []byte, header string, tolerance time.Duration, now time.Time) error {
	if secret == "" {
		return ErrSignatureMismatch
	}

	ts, macHex, ok := parseSignatureHeader(header)
	if !ok {
		return ErrSignatureMismatch
	}

	sigBytes, err := hex.DecodeString(macHex)
	if err != nil {
		return ErrSignatureMismatch
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte{'.'})
	mac.Write(payload)
	expected := mac.Sum(nil)

	if !hmac.Equal(expected, sigBytes) {
		return ErrSignatureMismatch
	}

	age := now.Sub(time.Unix(ts, 0))
	if age < 0 {
		age = -age
	}

	if age > tolerance {
		return ErrSignatureExpired
	}

	return nil
}

func newSafeClient(timeout time.Duration, allowPrivate bool) *http.Client {
	return webhook.NewSafeClient(timeout, allowPrivate)
}

// New builds a queue-backed Webhook from o.
// QueueAdapter names a registered queue backend and QueueOpts carries its settings.
func New(o webhook.Options) (webhook.Webhook, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	queueAdapter := strings.TrimSpace(o.QueueAdapter)
	if queueAdapter == "" {
		return nil, ErrMissingQueueAdapter
	}

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
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
		visibility = DefaultVisibilityTimeout
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
		return nil, err
	}

	replayTolerance := o.ReplayTolerance
	if replayTolerance <= 0 {
		replayTolerance = defaultReplayTolerance
	}

	return &adapter{
		regs:            make(map[string]map[string]registration),
		queue:           q,
		timeout:         timeout,
		maxRetries:      maxRetries,
		dlqTopic:        dlqTopic,
		consumers:       make(map[string]*consumer),
		client:          newSafeClient(timeout, o.AllowPrivateTargets),
		allowPrivate:    o.AllowPrivateTargets,
		logger:          o.Logger,
		replayTolerance: replayTolerance,
	}, nil
}
