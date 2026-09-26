package paddle

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/PaddleHQ/paddle-go-sdk/v5"

	"github.com/zenta-dev/zever/core/billing"
)

func openTest(t *testing.T, url string) billing.Billing {
	t.Helper()

	b, err := New(billing.Options{APIKey: "pdl_test_123", Endpoint: url})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return b
}

func TestOpen_missingAPIKey_returnsErrMissingAPIKey(t *testing.T) {
	t.Parallel()

	_, err := New(billing.Options{})
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("Open err = %v, want ErrMissingAPIKey", err)
	}
}

func TestOpen_invalidOptions_wrapsErrInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(billing.Options{APIKey: "k", Endpoint: "example.com/hook"})
	if !errors.Is(err, billing.ErrInvalidOptions) {
		t.Fatalf("Open err = %v, want ErrInvalidOptions", err)
	}

	if !strings.Contains(err.Error(), "paddle: ") {
		t.Fatalf("Open err %q missing %q", err.Error(), "paddle: ")
	}
}

func TestOpen_defaults_makeNoCall(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		sandbox bool
	}{
		{"sandbox", true},
		{"production", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := New(billing.Options{APIKey: "pdl_test_123", Sandbox: tc.sandbox})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			if err := b.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		})
	}
}

func TestOpen_newSDKError_wrapped(t *testing.T) {
	old := newSDK
	defer func() { newSDK = old }()

	sentinel := errors.New("boom")
	newSDK = func(string, ...paddle.Option) (*paddle.SDK, error) {
		return nil, sentinel
	}

	_, err := New(billing.Options{APIKey: "k"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "paddle: ") {
		t.Fatalf("Open err %q missing %q", err.Error(), "paddle: ")
	}
}

func flowMux(t *testing.T) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			Email string  `json:"email"`
			Name  *string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		data := map[string]any{"id": "ctm_123", "email": body.Email}
		if body.Name != nil {
			data["name"] = *body.Name
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	})

	mux.HandleFunc("/transactions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": "txn_123", "status": "ready", "customer_id": "ctm_123",
				"currency_code": "USD", "collection_mode": "automatic",
				"items":   []map[string]any{{"quantity": 1}},
				"details": map[string]any{"totals": map[string]any{"total": "1999"}},
			}})
		case http.MethodGet:
			if got := r.URL.Query().Get("customer_id"); got != "ctm_123" {
				t.Errorf("expected customer_id ctm_123, got %s", got)
			}

			if got := r.URL.Query().Get("per_page"); got != "1" {
				t.Errorf("expected per_page 1, got %s", got)
			}

			if got := r.URL.Query()["status"]; !slices.Equal(got, []string{"billed", "paid", "completed"}) {
				t.Errorf("expected status billed,paid,completed, got %v", got)
			}

			if got := r.URL.Query().Get("order_by"); got != "billed_at[DESC]" {
				t.Errorf("expected order_by billed_at[DESC], got %s", got)
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{
					"id": "txn_inv1", "status": "billed", "customer_id": "ctm_123",
					"currency_code": "USD",
					"details":       map[string]any{"totals": map[string]any{"total": "4200"}},
				}},
				"meta": map[string]any{"pagination": map[string]any{"per_page": 1}},
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/transactions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": r.PathValue("id"), "status": "canceled",
		}})
	})

	return mux
}

func TestBillingFlow_full(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(flowMux(t))
	defer srv.Close()

	b := openTest(t, srv.URL)
	ctx := t.Context()

	c, err := b.CreateCustomer(ctx, "Bob & Co", "bob+co@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	if c.ID != "ctm_123" || c.Name != "Bob & Co" || c.Email != "bob+co@example.com" {
		t.Fatalf("unexpected customer: %+v", c)
	}

	nameless, err := b.CreateCustomer(ctx, "", "anon@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer nameless: %v", err)
	}

	if nameless.Name != "" {
		t.Fatalf("nameless customer name = %q, want empty", nameless.Name)
	}

	s, err := b.CreateSubscription(ctx, c.ID, "pri_1", "")
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}

	if s.ID != "txn_123" || s.Status != billing.SubscriptionActive || s.CustomerID != c.ID || s.PlanID != "pri_1" {
		t.Fatalf("unexpected subscription: %+v", s)
	}

	if cancelErr := b.CancelSubscription(ctx, s.ID); cancelErr != nil {
		t.Fatalf("CancelSubscription: %v", cancelErr)
	}

	inv, err := b.GetInvoice(ctx, "ctm_123")
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}

	if inv.ID != "txn_inv1" || inv.AmountDue != 4200 || inv.Currency != "USD" || inv.Status != billing.InvoiceStatus("billed") {
		t.Fatalf("unexpected invoice: %+v", inv)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSubscriptionStatus_table(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   paddle.TransactionStatus
		want billing.SubscriptionStatus
	}{
		{paddle.TransactionStatusReady, billing.SubscriptionActive},
		{paddle.TransactionStatusBilled, billing.SubscriptionActive},
		{paddle.TransactionStatusPaid, billing.SubscriptionActive},
		{paddle.TransactionStatusCompleted, billing.SubscriptionActive},
		{paddle.TransactionStatusPastDue, billing.SubscriptionPastDue},
		{paddle.TransactionStatusCanceled, billing.SubscriptionCanceled},
		{paddle.TransactionStatus("trialing"), billing.SubscriptionStatus("trialing")},
	} {
		t.Run(string(tc.in), func(t *testing.T) {
			t.Parallel()

			if got := subscriptionStatus(tc.in); got != tc.want {
				t.Fatalf("subscriptionStatus(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestGuards(t *testing.T) {
	t.Parallel()

	b := openTest(t, "http://127.0.0.1:9")
	ctx := t.Context()

	t.Run("create subscription missing customer", func(t *testing.T) {
		t.Parallel()

		if _, err := b.CreateSubscription(ctx, "", "pri_1", ""); !errors.Is(err, billing.ErrMissingCustomerID) {
			t.Fatalf("err = %v, want ErrMissingCustomerID", err)
		}
	})

	t.Run("create subscription missing plan", func(t *testing.T) {
		t.Parallel()

		if _, err := b.CreateSubscription(ctx, "ctm_1", "", ""); !errors.Is(err, billing.ErrMissingPlanID) {
			t.Fatalf("err = %v, want ErrMissingPlanID", err)
		}
	})

	t.Run("cancel missing id", func(t *testing.T) {
		t.Parallel()

		if err := b.CancelSubscription(ctx, ""); !errors.Is(err, billing.ErrMissingSubscriptionID) {
			t.Fatalf("err = %v, want ErrMissingSubscriptionID", err)
		}
	})

	t.Run("invoice missing customer", func(t *testing.T) {
		t.Parallel()

		if _, err := b.GetInvoice(ctx, ""); !errors.Is(err, billing.ErrMissingCustomerID) {
			t.Fatalf("err = %v, want ErrMissingCustomerID", err)
		}
	})
}

func TestCreateCustomer_sdkError_returnsZero(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"type":"request_error","code":"invalid","detail":"bad"}}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	got, err := openTest(t, srv.URL).CreateCustomer(t.Context(), "A", "a@example.com", "")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "paddle: create customer: ") {
		t.Fatalf("err %q missing prefix", err.Error())
	}

	if got != (billing.Customer{}) {
		t.Fatalf("customer = %+v, want zero", got)
	}
}

func TestCreateSubscription_sdkError_returnsZero(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"type":"request_error","code":"invalid","detail":"bad"}}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	got, err := openTest(t, srv.URL).CreateSubscription(t.Context(), "ctm_1", "pri_1", "")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "paddle: create subscription: ") {
		t.Fatalf("err %q missing prefix", err.Error())
	}

	if got != (billing.Subscription{}) {
		t.Fatalf("subscription = %+v, want zero", got)
	}
}

func TestCancelSubscription_sdkError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"type":"request_error","code":"invalid","detail":"bad"}}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	err := openTest(t, srv.URL).CancelSubscription(t.Context(), "txn_1")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "paddle: cancel subscription: ") {
		t.Fatalf("err %q missing prefix", err.Error())
	}
}

func TestGetInvoice_listError_returnsZero(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	got, err := openTest(t, srv.URL).GetInvoice(t.Context(), "ctm_1")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "paddle: list transactions: ") {
		t.Fatalf("err %q missing prefix", err.Error())
	}

	if got != (billing.Invoice{}) {
		t.Fatalf("invoice = %+v, want zero", got)
	}
}

func TestGetInvoice_iterError_returnsZero(t *testing.T) {
	t.Parallel()

	var srv *httptest.Server

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("after") != "" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{},
			"meta": map[string]any{"pagination": map[string]any{
				"per_page": 1, "has_more": true,
				"next": srv.URL + "/transactions?after=txn_1",
			}},
		})
	}))
	defer srv.Close()

	_, err := openTest(t, srv.URL).GetInvoice(t.Context(), "ctm_1")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "paddle: list transactions: ") {
		t.Fatalf("err %q missing prefix", err.Error())
	}
}

func TestGetInvoice_parseTotalError_returnsZero(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"id": "txn_bad", "status": "billed", "customer_id": "ctm_1",
				"currency_code": "USD",
				"details":       map[string]any{"totals": map[string]any{"total": "not-a-number!!"}},
			}},
			"meta": map[string]any{"pagination": map[string]any{"per_page": 1}},
		})
	}))
	defer srv.Close()

	got, err := openTest(t, srv.URL).GetInvoice(t.Context(), "ctm_1")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, billing.ErrMalformedAmount) {
		t.Fatalf("err = %v, want ErrMalformedAmount", err)
	}

	if !strings.Contains(err.Error(), "paddle: parse total: ") {
		t.Fatalf("err %q missing prefix", err.Error())
	}

	if got != (billing.Invoice{}) {
		t.Fatalf("invoice = %+v, want zero", got)
	}
}

func TestGetInvoice_noInvoices_returnsNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{},
			"meta": map[string]any{"pagination": map[string]any{"per_page": 1}},
		})
	}))
	defer srv.Close()

	_, err := openTest(t, srv.URL).GetInvoice(t.Context(), "ctm_none")
	if err == nil {
		t.Fatal("expected error")
	}

	var nf *billing.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err %T is not *NotFoundError", err)
	}

	if nf.Resource != "invoice" || nf.ID != "ctm_none" {
		t.Fatalf("not found = %+v, want invoice/ctm_none", nf)
	}
}

func TestParsePaddleTotal_table(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		s        string
		currency string
		want     int64
		wantErr  error
	}{
		{"empty", "", "USD", 0, billing.ErrEmptyAmount},
		{"dotted", "10.00", "usd", 1000, nil},
		{"minor", "1000", "USD", 1000, nil},
		{"garbage", "not-a-number", "USD", 0, billing.ErrMalformedAmount},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePaddleTotal(tc.s, tc.currency)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("err = %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestClose_noop(t *testing.T) {
	t.Parallel()

	if err := openTest(t, "http://127.0.0.1:9").Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestConcurrent_20(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(flowMux(t))
	defer srv.Close()

	b := openTest(t, srv.URL)

	var wg sync.WaitGroup

	errs := make(chan error, 20)

	for range 20 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := b.CreateCustomer(t.Context(), "C", "c@example.com", "")
			errs <- err
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("CreateCustomer: %v", err)
		}
	}
}
