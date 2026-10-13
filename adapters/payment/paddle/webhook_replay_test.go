package paddle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/payment"
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

	delete(s.pending, key)
	s.records[key] = webhookRecord{fingerprint: fingerprint, expires: s.now().Add(payment.DefaultWebhookIdempotencyTTL)}

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

const replayBody = `{"event_id":"evt_1","event_type":"transaction.paid","occurred_at":"2026-08-30T12:00:00Z","data":{"id":"txn_wh1","status":"paid","currency_code":"USD","details":{"totals":{"total":"100.00"}}}}`

// mustReplayDriver opens a driver with store wired as the webhook dedupe store.
func mustReplayDriver(t *testing.T, store idempotency.Store) payment.Payment {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)

	return openTest(t, srv, func(o *payment.Options) { o.Idempotency = store })
}

func TestWebhookEvent_replayReturnsDuplicate(t *testing.T) {
	t.Parallel()

	p := mustReplayDriver(t, newWebhookStore(nil))
	ctx := t.Context()
	body := []byte(replayBody)

	first, err := p.WebhookEvent(ctx, body, signWebhook(t, "whsec_test", body, time.Now()))
	if err != nil {
		t.Fatalf("first WebhookEvent() err = %v", err)
	}

	if first.ID != "evt_1" {
		t.Fatalf("first WebhookEvent() ID = %q, want evt_1", first.ID)
	}

	_, err = p.WebhookEvent(ctx, body, signWebhook(t, "whsec_test", body, time.Now()))
	if !errors.Is(err, payment.ErrDuplicateEvent) {
		t.Fatalf("second WebhookEvent() err = %v, want ErrDuplicateEvent", err)
	}

	var dup payment.DuplicateEventError
	if !errors.As(err, &dup) {
		t.Fatalf("second WebhookEvent() err = %T, want DuplicateEventError", err)
	}

	if dup.EventID != "evt_1" {
		t.Fatalf("DuplicateEventError.EventID = %q, want evt_1", dup.EventID)
	}
}

func TestWebhookEvent_noStoreAllowsRepeat(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)

	p := openTest(t, srv, nil)
	ctx := t.Context()
	body := []byte(replayBody)

	for i := range 2 {
		if _, err := p.WebhookEvent(ctx, body, signWebhook(t, "whsec_test", body, time.Now())); err != nil {
			t.Fatalf("WebhookEvent() [%d] err = %v, want nil without a store", i, err)
		}
	}
}

func TestWebhookEvent_sameIDDifferentPayloadMismatch(t *testing.T) {
	t.Parallel()

	p := mustReplayDriver(t, newWebhookStore(nil))
	ctx := t.Context()
	first := []byte(replayBody)
	second := []byte(`{"event_id":"evt_1","event_type":"transaction.paid","occurred_at":"2026-08-30T12:00:00Z","data":{"id":"txn_wh1","status":"paid","currency_code":"USD","details":{"totals":{"total":"200.00"}}}}`)

	if _, err := p.WebhookEvent(ctx, first, signWebhook(t, "whsec_test", first, time.Now())); err != nil {
		t.Fatalf("first WebhookEvent() err = %v", err)
	}

	// Same event_id, different payload: the fingerprint guard must fail
	// closed rather than report a replay.
	_, err := p.WebhookEvent(ctx, second, signWebhook(t, "whsec_test", second, time.Now()))
	if !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("second WebhookEvent() err = %v, want ErrKeyMismatch", err)
	}
}

func TestWebhookEvent_expiredRecordAllowsRedelivery(t *testing.T) {
	t.Parallel()

	now := time.Now()
	current := now
	p := mustReplayDriver(t, newWebhookStore(func() time.Time { return current }))
	ctx := t.Context()
	body := []byte(replayBody)

	if _, err := p.WebhookEvent(ctx, body, signWebhook(t, "whsec_test", body, time.Now())); err != nil {
		t.Fatalf("first WebhookEvent() err = %v", err)
	}

	// Advance past the webhook TTL: the record has expired, so redelivery
	// processes again instead of reporting a duplicate.
	current = now.Add(payment.DefaultWebhookIdempotencyTTL + time.Second)

	if _, err := p.WebhookEvent(ctx, body, signWebhook(t, "whsec_test", body, time.Now())); err != nil {
		t.Fatalf("redelivery after TTL expiry err = %v, want nil", err)
	}
}

func TestWebhookEvent_missingIDSkipsDedupe(t *testing.T) {
	t.Parallel()

	store := newWebhookStore(nil)
	p := mustReplayDriver(t, store)
	ctx := t.Context()
	body := []byte(`{"event_type":"transaction.paid","data":{"id":"txn_wh1","status":"paid","currency_code":"USD","details":{"totals":{"total":"100.00"}}}}`)

	for i := range 2 {
		got, err := p.WebhookEvent(ctx, body, signWebhook(t, "whsec_test", body, time.Now()))
		if err != nil {
			t.Fatalf("WebhookEvent() [%d] err = %v, want nil when the event has no id", i, err)
		}

		if got.ID != "" {
			t.Fatalf("WebhookEvent() [%d] ID = %q, want empty", i, got.ID)
		}
	}
}
