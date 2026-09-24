package paddle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/payment"
)

// fakeStore is an in-memory idempotency.Store fake for refund guard tests.
type fakeStore struct {
	mu       sync.Mutex
	begin    int
	complete int
	forget   int
	records  map[string][]byte
	pending  map[string][]byte
	failNext bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{records: make(map[string][]byte), pending: make(map[string][]byte)}
}

func (f *fakeStore) Begin(_ context.Context, key string, opts idempotency.BeginOptions) (idempotency.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.begin++
	if f.failNext {
		f.failNext = false
		return idempotency.Outcome{}, errors.New("begin boom")
	}
	if fp, ok := f.records[key]; ok {
		if string(fp) != string(opts.Fingerprint) {
			return idempotency.Outcome{}, idempotency.ErrKeyMismatch
		}
		return idempotency.Outcome{Replay: true, Result: []byte{1}}, nil
	}
	if _, ok := f.pending[key]; ok {
		return idempotency.Outcome{}, idempotency.ErrInProgress
	}
	f.pending[key] = opts.Fingerprint
	return idempotency.Outcome{}, nil
}

func (f *fakeStore) Complete(_ context.Context, key string, fingerprint, _ []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.complete++
	delete(f.pending, key)
	f.records[key] = fingerprint
	return nil
}

func (f *fakeStore) Forget(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forget++
	delete(f.pending, key)
	return nil
}

func (f *fakeStore) Close() error { return nil }

func refundTestServer(t *testing.T, adjustments *atomic.Int64, failAdjust *atomic.Bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/transactions/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, txnPayload("txn_full", "paid", "USD", "2500", []map[string]any{{"id": "txnitm_1", "totals": map[string]any{"total": "2500"}}}))
	})
	mux.HandleFunc("/adjustments", func(w http.ResponseWriter, _ *http.Request) {
		if failAdjust.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		adjustments.Add(1)
		writeJSON(t, w, map[string]any{"data": map[string]any{"id": "adj_1"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRefundIdempotency_replayHitsAPIOnce(t *testing.T) {
	t.Parallel()
	var adjustments atomic.Int64
	var failAdjust atomic.Bool
	store := newFakeStore()
	srv := refundTestServer(t, &adjustments, &failAdjust)
	p := openTest(t, srv, func(o *payment.Options) { o.Idempotency = store })
	ctx := t.Context()
	if err := p.Refund(ctx, "txn_full", 2500, "key-abc"); err != nil {
		t.Fatalf("first Refund err = %v", err)
	}
	if err := p.Refund(ctx, "txn_full", 2500, "key-abc"); err != nil {
		t.Fatalf("replay Refund err = %v", err)
	}
	if got := adjustments.Load(); got != 1 {
		t.Fatalf("adjustments = %d, want 1", got)
	}
}

func TestRefundIdempotency_failureForgetsAllowsRetry(t *testing.T) {
	t.Parallel()
	var adjustments atomic.Int64
	var failAdjust atomic.Bool
	failAdjust.Store(true)
	store := newFakeStore()
	srv := refundTestServer(t, &adjustments, &failAdjust)
	p := openTest(t, srv, func(o *payment.Options) { o.Idempotency = store })
	ctx := t.Context()
	if err := p.Refund(ctx, "txn_full", 2500, "key-retry"); err == nil {
		t.Fatal("expected refund error, got nil")
	}
	store.mu.Lock()
	forgets := store.forget
	store.mu.Unlock()
	if forgets != 1 {
		t.Fatalf("forgets = %d, want 1", forgets)
	}
	failAdjust.Store(false)
	if err := p.Refund(ctx, "txn_full", 2500, "key-retry"); err != nil {
		t.Fatalf("retry Refund err = %v", err)
	}
	if got := adjustments.Load(); got != 1 {
		t.Fatalf("adjustments = %d, want 1", got)
	}
}
