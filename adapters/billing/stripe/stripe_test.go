package stripe

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeStripeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "card_error",
		},
	})
}

func customerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	writeJSON(w, map[string]any{
		"id":     "cus_1",
		"object": "customer",
		"name":   r.FormValue("name"),
		"email":  r.FormValue("email"),
	})
}

func subscriptionCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	_ = r.FormValue("customer")
	writeJSON(w, map[string]any{
		"id":     "sub_1",
		"object": "subscription",
		"status": "active",
	})
}

func subscriptionCancelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")
	id = strings.TrimSuffix(id, "/cancel")
	if i := strings.Index(id, "/"); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		id = "sub_1"
	}
	writeJSON(w, map[string]any{
		"id":     id,
		"object": "subscription",
		"status": "canceled",
	})
}

func invoiceMap(id, status string) map[string]any {
	return map[string]any{
		"id":         id,
		"object":     "invoice",
		"amount_due": 2000,
		"currency":   "usd",
		"status":     status,
	}
}

func listResp(data []map[string]any, hasMore bool) map[string]any {
	return map[string]any{
		"object":   "list",
		"data":     data,
		"has_more": hasMore,
		"url":      "/v1/invoices",
	}
}

func newDefaultMux(invoiceHandler http.HandlerFunc) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/customers", customerHandler)
	mux.HandleFunc("/v1/subscriptions", subscriptionCreateHandler)
	mux.HandleFunc("/v1/subscriptions/", subscriptionCancelHandler)
	if invoiceHandler != nil {
		mux.HandleFunc("/v1/invoices", invoiceHandler)
	} else {
		mux.HandleFunc("/v1/invoices", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, listResp([]map[string]any{invoiceMap("in_1", "open")}, false))
		})
	}
	return mux
}

func openWithServer(t *testing.T, srv *httptest.Server) billing.Billing {
	t.Helper()
	b, err := New(billing.Options{SecretKey: "sk_test_123", Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return b
}

func TestOpenMissingSecretKey(t *testing.T) {
	t.Parallel()
	_, err := New(billing.Options{})
	if !errors.Is(err, ErrMissingSecretKey) {
		t.Fatalf("expected ErrMissingSecretKey, got %v", err)
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()
	_, err := New(billing.Options{SecretKey: "sk_test_123", Endpoint: "://bad"})
	if !errors.Is(err, billing.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestOpenNoEndpoint(t *testing.T) {
	t.Parallel()
	b, err := New(billing.Options{SecretKey: "sk_test_123"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil Billing")
	}
	_ = b.Close()
}

func TestFullFlow(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newDefaultMux(nil))
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	ctx := t.Context()

	cus, err := b.CreateCustomer(ctx, "Ada", "ada@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}
	if cus.ID != "cus_1" || cus.Name != "Ada" || cus.Email != "ada@example.com" {
		t.Fatalf("unexpected customer %+v", cus)
	}

	sub, err := b.CreateSubscription(ctx, cus.ID, "price_123", "")
	if err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	if sub.ID == "" || sub.CustomerID != cus.ID || sub.PlanID != "price_123" {
		t.Fatalf("unexpected subscription %+v", sub)
	}
	if sub.Status != billing.SubscriptionStatus("active") {
		t.Fatalf("unexpected status %q", sub.Status)
	}

	inv, err := b.GetInvoice(ctx, cus.ID)
	if err != nil {
		t.Fatalf("GetInvoice() error = %v", err)
	}
	if inv.ID != "in_1" {
		t.Fatalf("unexpected invoice %+v", inv)
	}

	if err := b.CancelSubscription(ctx, sub.ID); err != nil {
		t.Fatalf("CancelSubscription() error = %v", err)
	}
}

func TestIdempotencyKeySentOnCustomerAndSubscription(t *testing.T) {
	t.Parallel()

	var (
		customerKey     string
		subscriptionKey string
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, r *http.Request) {
		customerKey = r.Header.Get("Idempotency-Key")
		customerHandler(w, r)
	})
	mux.HandleFunc("/v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		subscriptionKey = r.Header.Get("Idempotency-Key")
		subscriptionCreateHandler(w, r)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() { _ = b.Close() }()
	ctx := t.Context()

	cus, err := b.CreateCustomer(ctx, "Ada", "ada@example.com", "create-cus-key-1")
	if err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}
	if customerKey != "create-cus-key-1" {
		t.Fatalf("Idempotency-Key on customer create = %q, want %q", customerKey, "create-cus-key-1")
	}

	if _, err := b.CreateSubscription(ctx, cus.ID, "price_123", "create-sub-key-1"); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	if subscriptionKey != "create-sub-key-1" {
		t.Fatalf("Idempotency-Key on subscription create = %q, want %q", subscriptionKey, "create-sub-key-1")
	}
}

func TestEmptyIdempotencyKeyDoesNotForceOurs(t *testing.T) {
	t.Parallel()

	// stripe-go itself auto-generates a random Idempotency-Key per call
	// when none is set (see stripe.go's NewIdempotencyKey fallback), so a
	// header is always present regardless. What must NOT happen is us
	// forcing a fixed/empty key that would make the SDK reuse the same
	// auto-generated value across genuinely distinct calls; assert two
	// back-to-back calls with idempotencyKey="" get different keys (the
	// SDK's own per-call randomness), proving we aren't overriding it.
	var keys []string

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		customerHandler(w, r)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() { _ = b.Close() }()

	for range 2 {
		if _, err := b.CreateCustomer(t.Context(), "Ada", "ada@example.com", ""); err != nil {
			t.Fatalf("CreateCustomer() error = %v", err)
		}
	}

	if len(keys) != 2 || keys[0] == "" || keys[1] == "" || keys[0] == keys[1] {
		t.Fatalf("Idempotency-Key headers = %v, want two distinct non-empty values", keys)
	}
}

func TestCreateSubscriptionGuards(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newDefaultMux(nil))
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	ctx := t.Context()

	if _, err := b.CreateSubscription(ctx, "", "price_1", ""); !errors.Is(err, billing.ErrMissingCustomerID) {
		t.Errorf("expected ErrMissingCustomerID, got %v", err)
	}
	if _, err := b.CreateSubscription(ctx, "cus_1", "", ""); !errors.Is(err, billing.ErrMissingPlanID) {
		t.Errorf("expected ErrMissingPlanID, got %v", err)
	}
}

func TestCancelSubscriptionGuard(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newDefaultMux(nil))
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	if err := b.CancelSubscription(t.Context(), ""); !errors.Is(err, billing.ErrMissingSubscriptionID) {
		t.Errorf("expected ErrMissingSubscriptionID, got %v", err)
	}
}

func TestGetInvoiceGuard(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newDefaultMux(nil))
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	if _, err := b.GetInvoice(t.Context(), ""); !errors.Is(err, billing.ErrMissingCustomerID) {
		t.Errorf("expected ErrMissingCustomerID, got %v", err)
	}
}

func TestCreateCustomerSDKError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, _ *http.Request) {
		writeStripeError(w, 402, "card declined")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	got, err := b.CreateCustomer(t.Context(), "Ada", "ada@example.com", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != (billing.Customer{}) {
		t.Fatalf("expected zero Customer, got %+v", got)
	}
}

func TestCreateSubscriptionSDKError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/customers", customerHandler)
	mux.HandleFunc("/v1/subscriptions", func(w http.ResponseWriter, _ *http.Request) {
		writeStripeError(w, 402, "card declined")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	got, err := b.CreateSubscription(t.Context(), "cus_1", "price_1", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != (billing.Subscription{}) {
		t.Fatalf("expected zero Subscription, got %+v", got)
	}
}

func TestCancelSubscriptionSDKError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/subscriptions/", func(w http.ResponseWriter, _ *http.Request) {
		writeStripeError(w, 404, "no such subscription")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	if err := b.CancelSubscription(t.Context(), "sub_missing"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetInvoiceSkipsDraftAndVoid(t *testing.T) {
	t.Parallel()
	mux := newDefaultMux(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, listResp([]map[string]any{
			invoiceMap("in_draft", "draft"),
			invoiceMap("in_void", "void"),
			invoiceMap("in_open", "open"),
		}, false))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	inv, err := b.GetInvoice(t.Context(), "cus_1")
	if err != nil {
		t.Fatalf("GetInvoice() error = %v", err)
	}
	if inv.ID != "in_open" {
		t.Fatalf("expected in_open, got %+v", inv)
	}
	if inv.Status != billing.InvoiceStatus("open") {
		t.Fatalf("unexpected status %q", inv.Status)
	}
}

func TestGetInvoiceOnlyDraftsNotFound(t *testing.T) {
	t.Parallel()
	mux := newDefaultMux(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, listResp([]map[string]any{
			invoiceMap("in_d1", "draft"),
			invoiceMap("in_v1", "void"),
		}, false))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	_, err := b.GetInvoice(t.Context(), "cus_1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var nf *billing.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %T: %v", err, err)
	}
	if nf.Resource != "invoice" || nf.ID != "cus_1" {
		t.Fatalf("unexpected NotFoundError %+v", nf)
	}
	if !errors.Is(err, billing.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetInvoicePagination(t *testing.T) {
	t.Parallel()
	var requests atomic.Int64
	var secondStartingAfter atomic.Value
	secondStartingAfter.Store("")
	mux := newDefaultMux(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		q := r.URL.Query()
		_ = q.Get("limit")
		sa := q.Get("starting_after")
		if n == 2 {
			secondStartingAfter.Store(sa)
			writeJSON(w, listResp([]map[string]any{invoiceMap("in_open", "open")}, false))
			return
		}
		data := make([]map[string]any, 0, 100)
		for i := 0; i < 100; i++ {
			data = append(data, invoiceMap(fmt.Sprintf("in_draft_%d", i), "draft"))
		}
		writeJSON(w, listResp(data, true))
	})
	// Override to serve paginated logic that also works if SDK fetches more than 2 pages:
	_ = mux
	srv := httptest.NewServer(mux)
	defer srv.Close()
	// Rebuild with proper page-2 detection via starting_after for robustness:
	// (keep server above; handler already branches on request count)
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	inv, err := b.GetInvoice(t.Context(), "cus_1")
	if err != nil {
		t.Fatalf("GetInvoice() error = %v", err)
	}
	if inv.ID != "in_open" {
		t.Fatalf("expected in_open, got %+v", inv)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("expected 2 requests, got %d", got)
	}
	sa, ok := secondStartingAfter.Load().(string)
	if !ok || sa == "" {
		t.Fatal("expected starting_after on second request, got empty")
	}
}

func TestGetInvoiceIteratorMidPageError(t *testing.T) {
	t.Parallel()
	var requests atomic.Int64
	mux := newDefaultMux(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			writeJSON(w, listResp([]map[string]any{invoiceMap("in_d1", "draft")}, true))
			return
		}
		writeStripeError(w, 500, "internal error")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	b := openWithServer(t, srv)
	defer func() {
		_ = b.Close()
	}()
	got, err := b.GetInvoice(t.Context(), "cus_1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != (billing.Invoice{}) {
		t.Fatalf("expected zero Invoice, got %+v", got)
	}
}

func TestCloseNoop(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(newDefaultMux(nil))
	defer srv.Close()
	b := openWithServer(t, srv)
	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
