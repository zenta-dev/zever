package http

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
	"sync"
	"time"

	"github.com/zenta-dev/zever/internal/retry"
	"github.com/zenta-dev/zever/webhook"
)

// DefaultBaseDelay is the initial delivery retry delay.
// DefaultMaxDelay caps the delivery retry delay.
// DefaultJitterMax bounds flat additive retry jitter.
// DefaultTimeout is the per-delivery operation timeout used when Options.Timeout is zero.
const (
	DefaultBaseDelay = 500 * time.Millisecond
	DefaultMaxDelay  = 72 * time.Hour
	DefaultJitterMax = 250 * time.Millisecond
	DefaultTimeout   = 10 * time.Second
)

// deliveryBackoffPolicy computes the delay before retrying a failed
// delivery: linear growth from 500ms, capped at 72h, plus up to 250ms of
// flat additive jitter.
var deliveryBackoffPolicy = retry.Policy{
	BaseDelay:  DefaultBaseDelay,
	Linear:     true,
	MaxDelay:   DefaultMaxDelay,
	JitterMode: retry.JitterFlat,
	JitterMax:  DefaultJitterMax,
}

type registration struct {
	target string
	secret string
}

type adapter struct {
	mu           sync.RWMutex
	regs         map[string]map[string]registration
	client       *http.Client
	timeout      time.Duration
	maxRetries   int
	allowPrivate bool
}

func (a *adapter) Register(_ context.Context, event string, target string, secret string) error {
	if event == "" {
		return ErrMissingEvent
	}

	if target == "" {
		return ErrMissingTarget
	}

	var err error
	if a.allowPrivate {
		err = webhook.ValidateTargetSyntax(target)
	} else {
		err = webhook.ValidateTarget(target) //nolint:contextcheck // validation has no context dependency.
	}

	if err != nil {
		return fmt.Errorf("http: reject target %q: %w", target, err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.regs[event]; !ok {
		a.regs[event] = make(map[string]registration)
	}

	a.regs[event][target] = registration{target: target, secret: secret}

	return nil
}

func (a *adapter) Unregister(_ context.Context, event string, target string) error {
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
	}

	return nil
}

func (a *adapter) Deliver(ctx context.Context, event string, payload []byte) error {
	a.mu.RLock()

	targets, ok := a.regs[event]
	if !ok {
		a.mu.RUnlock()
		return fmt.Errorf("http: %w", webhook.ErrNotFound)
	}

	copyRegs := make([]registration, 0, len(targets))
	for _, r := range targets {
		copyRegs = append(copyRegs, r)
	}

	a.mu.RUnlock()

	var errs []error

	for _, r := range copyRegs {
		if err := a.deliverOne(ctx, r, payload); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (a *adapter) deliverOne(ctx context.Context, r registration, payload []byte) error {
	if err := a.validateIfNeeded(ctx, r.target); err != nil {
		return err
	}

	var lastErr error

	for attempt := 0; attempt < a.maxRetries; attempt++ {
		lastErr = a.singleAttempt(ctx, r, payload)
		if lastErr == nil {
			return nil
		}

		if attempt+1 >= a.maxRetries {
			break
		}

		if err := a.sleepWithContext(ctx, a.backoff(attempt+1)); err != nil {
			return err
		}
	}

	return fmt.Errorf("http: delivery failed after %d attempts: %w", a.maxRetries, lastErr)
}

func (a *adapter) validateIfNeeded(ctx context.Context, target string) error {
	if a.allowPrivate {
		return nil
	}

	if err := webhook.ValidateTargetContext(ctx, target); err != nil {
		return fmt.Errorf("http: blocked private target %q: %w", target, err)
	}

	return nil
}

func (a *adapter) singleAttempt(ctx context.Context, r registration, payload []byte) error {
	req, err := a.buildRequest(ctx, r, payload)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		a.drain(resp)

		return err
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		a.drain(resp)

		return nil
	}

	body := a.drainForError(resp)

	if len(body) == 0 {
		return fmt.Errorf("http: target returned status %d", resp.StatusCode)
	}

	return fmt.Errorf("http: target returned status %d: %s", resp.StatusCode, body)
}

func (a *adapter) buildRequest(
	ctx context.Context,
	r registration,
	payload []byte,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.target, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("http: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if r.secret != "" {
		req.Header.Set("X-Hub-Signature-256", sign(r.secret, payload))
	}

	return req, nil
}

func (a *adapter) drain(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	// Discard at most 1MiB so error paths do not buffer unbounded bodies.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	// Close explicitly; drain is best-effort.
	_ = resp.Body.Close()
}

// drainForError reads and closes resp.Body, returning up to 512 bytes of it
// for inclusion in a delivery-failure error -- matching geo/osm's and
// document/remote's checkStatus pattern, so "why didn't my webhook fire" is
// answerable from the error text alone instead of just a bare status code.
func (a *adapter) drainForError(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	_ = resp.Body.Close()

	return string(body)
}

func (a *adapter) sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func (a *adapter) backoff(attempt int) time.Duration {
	// NextDelay caps the delay at MaxDelay before jitter, so a flat jitter
	// draw on top of an already-capped delay can push the result slightly
	// past MaxDelay; re-clamp so the cap is exact regardless of jitter.
	d := deliveryBackoffPolicy.NextDelay(attempt)
	if d > DefaultMaxDelay {
		d = DefaultMaxDelay
	}

	return d
}

func (a *adapter) Close() error {
	return nil
}

// sign returns the current envelope for payload: "t=<unix-timestamp>,v1=<hex-hmac>"
// where the hex-hmac is HMAC-SHA256 over "<unix-timestamp>.<payload>". The
// timestamp lets a verifier (see webhook/queue's verifySignatureHeader) reject
// stale or replayed deliveries in addition to checking the MAC.
func sign(secret string, payload []byte) string {
	return signAt(secret, payload, time.Now().Unix())
}

// signAt is sign with an explicit timestamp, split out so tests can produce
// deterministic envelopes instead of racing time.Now().
func signAt(secret string, payload []byte, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte{'.'})
	mac.Write(payload)

	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func newSafeClient(timeout time.Duration, allowPrivate bool) *http.Client {
	return webhook.NewSafeClient(timeout, allowPrivate)
}

// New creates an HTTP webhook adapter from o.
// A zero Timeout defaults to 10 seconds; MaxRetries below 1 defaults to 3.
func New(o webhook.Options) (webhook.Webhook, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	maxRetries := o.MaxRetries
	if maxRetries < 1 {
		maxRetries = 3
	}

	return &adapter{
		regs:         make(map[string]map[string]registration),
		timeout:      timeout,
		maxRetries:   maxRetries,
		allowPrivate: o.AllowPrivateTargets,
		client:       newSafeClient(timeout, o.AllowPrivateTargets),
	}, nil
}
