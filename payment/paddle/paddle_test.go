package paddle

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaddleHQ/paddle-go-sdk/v5"

	"github.com/zenta-dev/zever/payment"
)

// signWebhook returns a Paddle-Signature header value for body.
func signWebhook(t *testing.T, secret string, body []byte, ts time.Time) string {
	t.Helper()

	stamp := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp + ":" + string(body)))

	return "ts=" + stamp + ";h1=" + hex.EncodeToString(mac.Sum(nil))
}

// openTest opens a driver pointed at srv.
func openTest(t *testing.T, srv *httptest.Server, mutate func(*payment.Options)) payment.Payment {
	t.Helper()

	opts := payment.Options{APIKey: "pdl_api_test", Endpoint: srv.URL, WebhookSecret: "whsec_test"} //nolint:gosec // test fixture, not a real credential.
	if mutate != nil {
		mutate(&opts)
	}

	p, err := New(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return p
}

// txnPayload renders a Paddle transaction data envelope.
func txnPayload(id, status, currency, total string, items []map[string]any) map[string]any {
	details := map[string]any{"totals": map[string]any{"total": total}}
	if items != nil {
		details["line_items"] = items
	}

	return map[string]any{"data": map[string]any{
		"id": id, "status": status, "currency_code": currency, "details": details,
	}}
}

// writeJSON encodes v with a 200 status.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode: %v", err)
	}
}

func TestOpenMissingAPIKey(t *testing.T) {
	t.Parallel()

	if _, err := New(payment.Options{}); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(payment.Options{APIKey: "x", MaxWebhookBytes: -1})
	if !errors.Is(err, payment.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestOpenSandboxAndDefault(t *testing.T) {
	t.Parallel()

	sandbox, err := New(payment.Options{APIKey: "x", Sandbox: true})
	if err != nil {
		t.Fatalf("sandbox Open: %v", err)
	}

	if sandbox == nil {
		t.Fatal("expected non-nil sandbox driver")
	}

	prod, err := New(payment.Options{APIKey: "x"})
	if err != nil {
		t.Fatalf("default Open: %v", err)
	}

	if prod == nil {
		t.Fatal("expected non-nil production driver")
	}
}

func TestOpenNewSDKError(t *testing.T) {
	old := newSDK
	newSDK = func(string, ...paddle.Option) (*paddle.SDK, error) {
		return nil, errors.New("boom")
	}
	defer func() { newSDK = old }()

	if _, err := New(payment.Options{APIKey: "x"}); err == nil || !strings.Contains(err.Error(), "paddle:") {
		t.Fatalf("expected wrapped constructor error, got %v", err)
	}
}

func TestCreatePayment(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var transit, customer, price, currency string

	var qty int

	mux := http.NewServeMux()
	mux.HandleFunc("/transactions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

			return
		}

		transit = r.Header.Get("X-Transit-Id")

		var body struct {
			Items []struct {
				PriceID  string `json:"price_id"`
				Quantity int    `json:"quantity"`
			} `json:"items"`
			CustomerID   string `json:"customer_id"`
			CurrencyCode string `json:"currency_code"`
		}
		if derr := json.NewDecoder(r.Body).Decode(&body); derr != nil {
			http.Error(w, derr.Error(), http.StatusBadRequest)

			return
		}

		mu.Lock()
		if len(body.Items) == 1 {
			price = body.Items[0].PriceID
			qty = body.Items[0].Quantity
		}

		customer = body.CustomerID
		currency = body.CurrencyCode
		mu.Unlock()
		writeJSON(t, w, txnPayload("txn_pay1", "ready", "USD", "100.00", nil))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := openTest(t, srv, nil)
	res, err := p.CreatePayment(t.Context(), payment.Request{
		Amount:   10000,
		Currency: "USD",
		Meta:     map[string]string{"price_id": "pri_main", "customer_id": "ctm_bob", "transit_id": "trn_123"},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	if res.ID != "txn_pay1" || res.Amount != 10000 || res.Currency != "USD" {
		t.Fatalf("unexpected result: %+v", res)
	}

	mu.Lock()
	defer mu.Unlock()
	if transit != "trn_123" {
		t.Fatalf("transit not forwarded, got %q", transit)
	}

	if customer != "ctm_bob" || price != "pri_main" || qty != 1 || currency != "USD" {
		t.Fatalf("body not forwarded: customer=%q price=%q qty=%d currency=%q", customer, price, qty, currency)
	}
}

func TestCreatePaymentValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		req  payment.Request
		want error
	}{
		{"zero amount", payment.Request{Amount: 0, Currency: "USD", Meta: map[string]string{"price_id": "pri"}}, payment.ErrInvalidAmount},
		{"missing currency", payment.Request{Amount: 100, Currency: "", Meta: map[string]string{"price_id": "pri"}}, payment.ErrMissingCurrency},
		{"missing price", payment.Request{Amount: 100, Currency: "USD"}, ErrMissingPriceID},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, oerr := New(payment.Options{APIKey: "x", Endpoint: "http://127.0.0.1:1"})
			if oerr != nil {
				t.Fatalf("Open: %v", oerr)
			}

			if _, cerr := p.CreatePayment(t.Context(), tc.req); !errors.Is(cerr, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, cerr)
			}
		})
	}
}

func TestCreatePaymentMismatch(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/transactions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, txnPayload("txn_mis1", "ready", "USD", "2500", nil))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := openTest(t, srv, nil)
	_, err := p.CreatePayment(t.Context(), payment.Request{
		Amount: 1000, Currency: "USD", Meta: map[string]string{"price_id": "pri_main"},
	})
	if err == nil {
		t.Fatal("expected mismatch error")
	}

	var mismatch payment.AmountMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected AmountMismatchError, got %v", err)
	}

	if mismatch.Expected != 1000 || mismatch.Actual != 2500 {
		t.Fatalf("unexpected mismatch fields: %+v", mismatch)
	}
}

func TestCreatePaymentErrors(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/transactions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Items []struct {
				PriceID string `json:"price_id"`
			} `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		switch {
		case len(body.Items) == 1 && body.Items[0].PriceID == "pri_boom":
			http.Error(w, "bad request", http.StatusBadRequest)
		case len(body.Items) == 1 && body.Items[0].PriceID == "pri_garbage":
			writeJSON(t, w, txnPayload("txn_g", "ready", "USD", "not-a-number", nil))
		default:
			writeJSON(t, w, txnPayload("txn_ok", "ready", "USD", "100.00", nil))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := openTest(t, srv, nil)

	if _, err := p.CreatePayment(t.Context(), payment.Request{Amount: 1, Currency: "USD", Meta: map[string]string{"price_id": "pri_boom"}}); err == nil || !strings.Contains(err.Error(), "paddle: create") {
		t.Fatalf("expected create error, got %v", err)
	}

	if _, err := p.CreatePayment(t.Context(), payment.Request{Amount: 1, Currency: "USD", Meta: map[string]string{"price_id": "pri_garbage"}}); err == nil || !strings.Contains(err.Error(), "parse total") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestGetPayment(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/transactions/", func(w http.ResponseWriter, r *http.Request) {
		switch id := strings.TrimPrefix(r.URL.Path, "/transactions/"); id {
		case "txn_pay1":
			writeJSON(t, w, txnPayload("txn_pay1", "paid", "USD", "2500", nil))
		case "txn_empty":
			writeJSON(t, w, txnPayload("txn_empty", "paid", "EUR", "", nil))
		case "txn_garbage":
			writeJSON(t, w, txnPayload("txn_garbage", "paid", "USD", "nope", nil))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := openTest(t, srv, nil)

	got, err := p.GetPayment(t.Context(), "txn_pay1")
	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}

	if got.Status != payment.PaymentStatus("paid") || got.Amount != 2500 {
		t.Fatalf("unexpected get result: %+v", got)
	}

	empty, err := p.GetPayment(t.Context(), "txn_empty")
	if err != nil {
		t.Fatalf("GetPayment empty total: %v", err)
	}

	if empty.Amount != 0 || empty.Currency != "EUR" {
		t.Fatalf("expected zero amount in EUR, got %+v", empty)
	}

	if _, err := p.GetPayment(t.Context(), "txn_garbage"); err == nil || !strings.Contains(err.Error(), "parse total") {
		t.Fatalf("expected parse error, got %v", err)
	}

	if _, err := p.GetPayment(t.Context(), "txn_nope"); err == nil || !strings.Contains(err.Error(), "paddle: get") {
		t.Fatalf("expected get error, got %v", err)
	}

	if _, err := p.GetPayment(t.Context(), ""); !errors.Is(err, payment.ErrMissingPaymentID) {
		t.Fatalf("expected ErrMissingPaymentID, got %v", err)
	}
}

func TestParsePaddleAmount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"10", 10, false}, // no-dot branch passes minor units through verbatim.
		{"abc", 0, true},
		{"100.00", 10000, false},
		{"99.99", 9999, false},
		{"99.999", 9999, false}, // extra precision truncates toward zero.
		{"1.5", 150, false},
		{".99", 99, false},
		{"-.50", -50, false},
		{"-5.25", -525, false},
		{"12.xy", 0, true},
		{"xx.00", 0, true},
	}

	for _, tc := range cases {
		t.Run("amount_"+tc.in, func(t *testing.T) {
			t.Parallel()

			got, err := parsePaddleAmount(tc.in)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.in)
			}

			if !tc.wantErr && (err != nil || got != tc.want) {
				t.Fatalf("parsePaddleAmount(%q) = %d, %v; want %d", tc.in, got, err, tc.want)
			}
		})
	}
}

func TestRefund(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var action, atype, txnID, itemID, itemType, itemAmount string

	var adjustFail bool

	items := map[string][]map[string]any{
		"txn_full":    {{"id": "txnitm_1", "totals": map[string]any{"total": "2500"}}},
		"txn_partial": {{"id": "txnitm_1", "totals": map[string]any{"total": "2500"}}},
		"txn_multi": {
			{"id": "txnitm_1", "totals": map[string]any{"total": "1500"}},
			{"id": "txnitm_2", "totals": map[string]any{"total": "1000"}},
		},
		"txn_badtotal": {{"id": "txnitm_1", "totals": map[string]any{"total": "2500"}}},
		"txn_baditem":  {{"id": "txnitm_1", "totals": map[string]any{"total": "garbage"}}},
		"txn_small":    {{"id": "txnitm_1", "totals": map[string]any{"total": "500"}}},
	}
	totals := map[string]string{
		"txn_full": "2500", "txn_partial": "2500", "txn_multi": "2500",
		"txn_badtotal": "garbage", "txn_baditem": "2500", "txn_small": "2500",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/transactions/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/transactions/")
		total, ok := totals[id]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)

			return
		}

		writeJSON(t, w, txnPayload(id, "paid", "USD", total, items[id]))
	})
	mux.HandleFunc("/adjustments", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fail := adjustFail
		mu.Unlock()
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)

			return
		}

		var body struct {
			Action        string `json:"action"`
			Type          string `json:"type"`
			TransactionID string `json:"transaction_id"`
			Items         []struct {
				ItemID string  `json:"item_id"`
				Type   string  `json:"type"`
				Amount *string `json:"amount"`
			} `json:"items"`
		}
		if derr := json.NewDecoder(r.Body).Decode(&body); derr != nil {
			http.Error(w, derr.Error(), http.StatusBadRequest)

			return
		}

		mu.Lock()
		defer mu.Unlock()
		action, atype, txnID = body.Action, body.Type, body.TransactionID
		if len(body.Items) == 1 && body.Items[0].Amount != nil {
			itemID, itemType, itemAmount = body.Items[0].ItemID, body.Items[0].Type, *body.Items[0].Amount
		}

		writeJSON(t, w, map[string]any{"data": map[string]any{"id": "adj_1"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := openTest(t, srv, nil)
	ctx := t.Context()

	if err := p.Refund(ctx, "txn_full", 2500); err != nil {
		t.Fatalf("full Refund: %v", err)
	}

	mu.Lock()
	if action != "refund" || atype != "full" || txnID != "txn_full" {
		mu.Unlock()
		t.Fatalf("unexpected full adjustment: %q %q %q", action, atype, txnID)
	}
	mu.Unlock()

	if err := p.Refund(ctx, "txn_partial", 1000); err != nil {
		t.Fatalf("partial Refund: %v", err)
	}

	mu.Lock()
	if atype != "partial" || itemID != "txnitm_1" || itemType != "partial" || itemAmount != "1000" {
		mu.Unlock()
		t.Fatalf("unexpected partial adjustment: %q %q %q %q", atype, itemID, itemType, itemAmount)
	}
	mu.Unlock()

	if err := p.Refund(ctx, "txn_x", 0); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}

	if err := p.Refund(ctx, "", 100); !errors.Is(err, payment.ErrMissingPaymentID) {
		t.Fatalf("expected ErrMissingPaymentID, got %v", err)
	}

	if err := p.Refund(ctx, "txn_nope", 100); err == nil || !strings.Contains(err.Error(), "get transaction") {
		t.Fatalf("expected get error, got %v", err)
	}

	if err := p.Refund(ctx, "txn_badtotal", 100); err == nil || !strings.Contains(err.Error(), "parse total") {
		t.Fatalf("expected total parse error, got %v", err)
	}

	overErr := p.Refund(ctx, "txn_full", 99999)
	var over payment.AmountMismatchError
	if !errors.As(overErr, &over) || over.Expected != 2500 || over.Actual != 99999 {
		t.Fatalf("expected over-total mismatch, got %v", overErr)
	}

	if err := p.Refund(ctx, "txn_baditem", 100); err == nil || !strings.Contains(err.Error(), "parse line item") {
		t.Fatalf("expected item parse error, got %v", err)
	}

	itemErr := p.Refund(ctx, "txn_small", 1000)
	var itemOver payment.AmountMismatchError
	if !errors.As(itemErr, &itemOver) || itemOver.Expected != 500 || itemOver.Actual != 1000 {
		t.Fatalf("expected over-item mismatch, got %v", itemErr)
	}

	if err := p.Refund(ctx, "txn_multi", 1000); !errors.Is(err, ErrMultiItemPartial) {
		t.Fatalf("expected ErrMultiItemPartial, got %v", err)
	}

	mu.Lock()
	adjustFail = true
	mu.Unlock()
	if err := p.Refund(ctx, "txn_full", 2500); err == nil || !strings.Contains(err.Error(), "paddle: refund") {
		t.Fatalf("expected adjustment error, got %v", err)
	}
}

func TestWebhookEvent(t *testing.T) {
	t.Parallel()

	const secret = "whsec_test"

	body := []byte(`{"event_id":"evt_1","event_type":"transaction.paid","occurred_at":"2026-08-30T12:00:00Z","data":{"id":"txn_wh1","status":"paid","currency_code":"USD","details":{"totals":{"total":"100.00"}}}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	p := openTest(t, srv, nil)
	ev, err := p.WebhookEvent(t.Context(), body, signWebhook(t, secret, body, time.Now()))
	if err != nil {
		t.Fatalf("WebhookEvent: %v", err)
	}

	if ev.Type != "transaction.paid" {
		t.Fatalf("expected transaction.paid, got %s", ev.Type)
	}

	if ev.Object.ID != "txn_wh1" || ev.Object.Amount != 10000 || ev.Object.Currency != "USD" || ev.Object.Status != payment.PaymentStatus("paid") {
		t.Fatalf("unexpected object: %+v", ev.Object)
	}
}

func TestWebhookEventErrors(t *testing.T) {
	t.Parallel()

	const secret = "whsec_test"

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	p := openTest(t, srv, nil)
	ctx := t.Context()
	now := time.Now()

	small := openTest(t, srv, func(o *payment.Options) { o.MaxWebhookBytes = 64 })
	big := make([]byte, 65)
	var sizeErr payment.SizeLimitError
	if werr := func() error {
		_, err := small.WebhookEvent(ctx, big, "x")
		return err
	}(); !errors.As(werr, &sizeErr) || sizeErr.Limit != 64 || sizeErr.Size != 65 {
		t.Fatalf("expected SizeLimitError, got %v", werr)
	}

	if _, werr := p.WebhookEvent(ctx, []byte("{}"), signWebhook(t, "whsec_wrong", []byte("{}"), now)); !errors.Is(werr, payment.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", werr)
	}

	if _, werr := p.WebhookEvent(ctx, []byte("{}"), "ts=0;h1=deadbeef"); !errors.Is(werr, payment.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for expired sig, got %v", werr)
	}

	if _, werr := p.WebhookEvent(ctx, []byte("{}"), "bogus"); !errors.Is(werr, payment.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for malformed sig, got %v", werr)
	}

	bare := openTest(t, srv, func(o *payment.Options) { o.WebhookSecret = "" })
	if _, werr := bare.WebhookEvent(ctx, []byte("{}"), "x"); !errors.Is(werr, payment.ErrMissingWebhookSecret) {
		t.Fatalf("expected ErrMissingWebhookSecret, got %v", werr)
	}

	var nctx context.Context

	valid := []byte(`{"event_type":"t","data":{"id":"x"}}`)
	if _, werr := p.WebhookEvent(nctx, valid, signWebhook(t, secret, valid, now)); werr == nil || !strings.Contains(werr.Error(), "paddle: webhook") {
		t.Fatalf("expected request error, got %v", werr)
	}

	corrupt := []byte(`{"event_type":`)
	if _, werr := p.WebhookEvent(ctx, corrupt, signWebhook(t, secret, corrupt, now)); werr == nil || !strings.Contains(werr.Error(), "unmarshal") {
		t.Fatalf("expected unmarshal error, got %v", werr)
	}

	badTotal := []byte(`{"event_type":"t","data":{"id":"x","details":{"totals":{"total":"garbage"}}}}`)
	if _, werr := p.WebhookEvent(ctx, badTotal, signWebhook(t, secret, badTotal, now)); werr == nil || !strings.Contains(werr.Error(), "parse total") {
		t.Fatalf("expected total parse error, got %v", werr)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	if err := openTest(t, srv, nil).Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestConcurrentGet(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/transactions/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, txnPayload(strings.TrimPrefix(r.URL.Path, "/transactions/"), "paid", "USD", "2500", nil))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := openTest(t, srv, nil)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, gerr := p.GetPayment(t.Context(), "txn_c"); gerr != nil {
				t.Errorf("GetPayment: %v", gerr)
			}
		}()
	}

	wg.Wait()
}
