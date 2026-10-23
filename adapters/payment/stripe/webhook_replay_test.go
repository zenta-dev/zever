package stripe

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v82"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/shared/codec"
)

// webhookStore is a clock-injectable idempotency.Store fake for webhook
// replay tests: completed records expire after their TTL against now.
type webhookStore struct {
	mu      sync.Mutex
	now     func() time.Time
	records map[string]webhookRecord
	pending map[string][]byte
}

type webhookRecord struct {
	fingerprint []byte
	expires     time.Time
}

func newWebhookStore(now func() time.Time) *webhookStore {
	if now == nil {
		now = time.Now
	}

	return &webhookStore{now: now, records: make(map[string]webhookRecord), pending: make(map[string][]byte)}
}

func (s *webhookStore) Begin(_ context.Context, key string, opts idempotency.BeginOptions) (idempotency.Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec, ok := s.records[key]; ok {
		if !s.now().Before(rec.expires) {
			delete(s.records, key)
		} else {
			if string(rec.fingerprint) != string(opts.Fingerprint) {
				return idempotency.Outcome{}, idempotency.ErrKeyMismatch
			}

			return idempotency.Outcome{Replay: true, Result: []byte{1}}, nil
		}
	}

	if _, ok := s.pending[key]; ok {
		return idempotency.Outcome{}, idempotency.ErrInProgress
	}

	s.pending[key] = opts.Fingerprint

	return idempotency.Outcome{}, nil
}

func (s *webhookStore) Complete(_ context.Context, key string, fingerprint, _ []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ttl := payment.DefaultWebhookIdempotencyTTL

	delete(s.pending, key)
	s.records[key] = webhookRecord{fingerprint: fingerprint, expires: s.now().Add(ttl)}

	return nil
}

func (s *webhookStore) Forget(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.pending, key)
	delete(s.records, key)

	return nil
}

func (s *webhookStore) Close() error { return nil }

// eventPayloadNoID builds a Stripe event envelope without an id, for the
// missing-id path that must skip dedupe.
func eventPayloadNoID(typ string, obj any) []byte {
	raw, _ := codec.Marshal(obj)

	env := map[string]any{
		"object":      "event",
		"api_version": stripe.APIVersion,
		"type":        typ,
		"data": map[string]any{
			"object": jsontext.Value(raw),
		},
	}

	out, _ := codec.Marshal(env)

	return out
}

// mustReplayDriver opens a driver with store wired as the webhook dedupe store.
func mustReplayDriver(t *testing.T, srvURL string, store idempotency.Store) payment.Payment {
	t.Helper()

	p, err := New(payment.Options{
		SecretKey:     "sk_test_x",
		WebhookSecret: "whsec_test",
		Endpoint:      srvURL,
		Idempotency:   store,
	})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	return p
}

func TestWebhookEvent_replayReturnsDuplicate(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustReplayDriver(t, srv.URL, newWebhookStore(nil))

	ctx := t.Context()
	raw := eventPayload("payment_intent.succeeded", piObject("pi_123", "succeeded"))

	first, err := p.WebhookEvent(ctx, raw, sign(raw, "whsec_test"))
	if err != nil {
		t.Fatalf("first WebhookEvent() err = %v", err)
	}

	if first.ID != "evt_test_1" {
		t.Fatalf("first WebhookEvent() ID = %q, want evt_test_1", first.ID)
	}

	_, err = p.WebhookEvent(ctx, raw, sign(raw, "whsec_test"))
	if !errors.Is(err, payment.ErrDuplicateEvent) {
		t.Fatalf("second WebhookEvent() err = %v, want ErrDuplicateEvent", err)
	}

	var dup payment.DuplicateEventError
	if !errors.As(err, &dup) {
		t.Fatalf("second WebhookEvent() err = %T, want DuplicateEventError", err)
	}

	if dup.EventID != "evt_test_1" {
		t.Fatalf("DuplicateEventError.EventID = %q, want evt_test_1", dup.EventID)
	}
}

func TestWebhookEvent_noStoreAllowsRepeat(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	ctx := t.Context()
	raw := eventPayload("payment_intent.succeeded", piObject("pi_123", "succeeded"))

	for i := range 2 {
		if _, err := p.WebhookEvent(ctx, raw, sign(raw, "whsec_test")); err != nil {
			t.Fatalf("WebhookEvent() [%d] err = %v, want nil without a store", i, err)
		}
	}
}

func TestWebhookEvent_sameIDDifferentPayloadMismatch(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustReplayDriver(t, srv.URL, newWebhookStore(nil))

	ctx := t.Context()
	first := eventPayload("payment_intent.succeeded", piObject("pi_123", "succeeded"))
	second := eventPayload("payment_intent.succeeded", piObject("pi_999", "succeeded"))

	if _, err := p.WebhookEvent(ctx, first, sign(first, "whsec_test")); err != nil {
		t.Fatalf("first WebhookEvent() err = %v", err)
	}

	// Same envelope id, different payload: the fingerprint guard must fail
	// closed rather than report a replay.
	_, err := p.WebhookEvent(ctx, second, sign(second, "whsec_test"))
	if !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("second WebhookEvent() err = %v, want ErrKeyMismatch", err)
	}
}

func TestWebhookEvent_expiredRecordAllowsRedelivery(t *testing.T) {
	t.Parallel()

	now := time.Now()
	current := now
	store := newWebhookStore(func() time.Time { return current })

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustReplayDriver(t, srv.URL, store)

	ctx := t.Context()
	raw := eventPayload("payment_intent.succeeded", piObject("pi_123", "succeeded"))

	if _, err := p.WebhookEvent(ctx, raw, sign(raw, "whsec_test")); err != nil {
		t.Fatalf("first WebhookEvent() err = %v", err)
	}

	// Advance past the webhook TTL: the record has expired, so redelivery
	// processes again instead of reporting a duplicate.
	current = now.Add(payment.DefaultWebhookIdempotencyTTL + time.Second)

	if _, err := p.WebhookEvent(ctx, raw, sign(raw, "whsec_test")); err != nil {
		t.Fatalf("redelivery after TTL expiry err = %v, want nil", err)
	}
}

func TestWebhookEvent_missingIDSkipsDedupe(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	store := newWebhookStore(nil)
	p := mustReplayDriver(t, srv.URL, store)

	ctx := t.Context()
	raw := eventPayloadNoID("payment_intent.succeeded", piObject("pi_123", "succeeded"))

	for i := range 2 {
		got, err := p.WebhookEvent(ctx, raw, sign(raw, "whsec_test"))
		if err != nil {
			t.Fatalf("WebhookEvent() [%d] err = %v, want nil when the event has no id", i, err)
		}

		if got.ID != "" {
			t.Fatalf("WebhookEvent() [%d] ID = %q, want empty", i, got.ID)
		}
	}
}
