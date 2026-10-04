package paddle

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/payment"
)

func TestBeginRefundGuard_nilStoreAndEmptyKey(t *testing.T) {
	t.Parallel()

	if ok, err := beginRefundGuard(t.Context(), nil, "k", nil); err != nil || !ok {
		t.Errorf("beginRefundGuard(nil store) = (%v, %v), want (true, nil)", ok, err)
	}

	store := newFakeStore()
	if ok, err := beginRefundGuard(t.Context(), store, "", nil); err != nil || !ok {
		t.Errorf("beginRefundGuard(empty key) = (%v, %v), want (true, nil)", ok, err)
	}

	store.mu.Lock()
	begin := store.begin
	store.mu.Unlock()
	if begin != 0 {
		t.Errorf("Begin calls = %d, want 0 when key empty", begin)
	}
}

func TestBeginRefundGuard_replay(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newFakeStore()
	fp := []byte("fingerprint")

	if _, err := store.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("seed Begin() = %v", err)
	}
	if err := store.Complete(ctx, "k", fp, []byte{1}); err != nil {
		t.Fatalf("seed Complete() = %v", err)
	}

	ok, err := beginRefundGuard(ctx, store, "k", fp)
	if err != nil {
		t.Fatalf("beginRefundGuard() = %v", err)
	}
	if ok {
		t.Error("beginRefundGuard(replay) ok = true, want false")
	}
}

func TestBeginRefundGuard_error(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.failNext = true

	if _, err := beginRefundGuard(t.Context(), store, "k", []byte("fp")); err == nil {
		t.Error("beginRefundGuard(store error) = nil, want error")
	}
}

func TestRefundFingerprint_deterministic(t *testing.T) {
	t.Parallel()

	a := refundFingerprint("txn_1", 100)
	b := refundFingerprint("txn_1", 100)
	if string(a) != string(b) {
		t.Error("refundFingerprint() not deterministic for identical input")
	}
	if string(refundFingerprint("txn_1", 101)) == string(a) {
		t.Error("refundFingerprint() equal across different amounts")
	}
	if string(refundFingerprint("txn_2", 100)) == string(a) {
		t.Error("refundFingerprint() equal across different ids")
	}
}

func TestWebhookEvent_exactLimit_allowed(t *testing.T) {
	t.Parallel()

	const secret = "whsec_test"
	body := []byte(`{"event_type":"transaction.paid","data":{"id":"txn_wh1","status":"paid","currency_code":"USD"}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	p := openTest(t, srv, func(o *payment.Options) { o.MaxWebhookBytes = len(body) })
	if _, err := p.WebhookEvent(t.Context(), body, signWebhook(t, secret, body, time.Now())); err != nil {
		t.Errorf("WebhookEvent(exact limit) = %v, want nil", err)
	}
}

func TestParsePaddleAmount_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"  100.00  ", 10000, false},
		{"0.00", 0, false},
		{"1.", 100, false},
		{"0.5", 50, false},
		{"-0.50", -50, false},
		{"1000000000.99", 100000000099, false},
	}
	for _, tc := range cases {
		got, err := parsePaddleAmount(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parsePaddleAmount(%q) err = nil, want error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("parsePaddleAmount(%q) = (%d, %v), want %d", tc.in, got, err, tc.want)
		}
	}
}

func TestClose_idempotent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	p := openTest(t, srv, nil)
	if err := p.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}
