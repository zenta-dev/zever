package stripe

import (
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

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/zenta-dev/zever/payment"
)

// fakeStripe is scriptable state for the fake Stripe v1 API.
type fakeStripe struct {
	// mu guards all fields below.
	mu sync.Mutex
	// mode selects failure injection: "", "create402", "get404", "refund402".
	mode string
	// piID is the PaymentIntent ID returned by create.
	piID string
	// idempotencyKey captures the last Idempotency-Key header seen on create.
	idempotencyKey string
	// refunds counts refund creations.
	refunds int
}

// setMode switches failure injection.
func (f *fakeStripe) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.mode = mode
}

// getMode reads failure injection.
func (f *fakeStripe) getMode() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.mode
}

// newFake spins up an httptest server mimicking the Stripe v1 API.
func newFake(t *testing.T, f *fakeStripe) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/payment_intents", func(w http.ResponseWriter, r *http.Request) {
		if f.getMode() == "create402" {
			writeStripeError(w, http.StatusPaymentRequired, "Your card was declined.")

			return
		}

		f.mu.Lock()
		f.idempotencyKey = r.Header.Get("Idempotency-Key")
		piID := f.piID
		f.mu.Unlock()

		amount, _ := strconv.Atoi(r.FormValue("amount"))

		writeJSON(w, map[string]any{
			"id":       piID,
			"object":   "payment_intent",
			"amount":   amount,
			"currency": r.FormValue("currency"),
			"status":   "succeeded",
		})
	})

	mux.HandleFunc("/v1/payment_intents/", func(w http.ResponseWriter, r *http.Request) {
		if f.getMode() == "get404" {
			writeStripeError(w, http.StatusNotFound, "No such payment_intent.")

			return
		}

		writeJSON(w, map[string]any{
			"id":       strings.TrimPrefix(r.URL.Path, "/v1/payment_intents/"),
			"object":   "payment_intent",
			"amount":   2000,
			"currency": "usd",
			"status":   "succeeded",
		})
	})

	mux.HandleFunc("/v1/refunds", func(w http.ResponseWriter, _ *http.Request) {
		if f.getMode() == "refund402" {
			writeStripeError(w, http.StatusPaymentRequired, "Charge already refunded.")

			return
		}

		f.mu.Lock()
		f.refunds++
		f.mu.Unlock()

		writeJSON(w, map[string]any{
			"id":             "re_test_1",
			"object":         "refund",
			"amount":         500,
			"payment_intent": "pi_test_123",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

// writeJSON encodes v as JSON with status 200.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")

	raw, _ := json.Marshal(v)
	_, _ = w.Write(raw)
}

// writeStripeError encodes a Stripe-style error envelope.
func writeStripeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	writeJSON(w, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "card_error",
		},
	})
}

// mustOpen opens a driver against srv or fails the test.
func mustOpen(t *testing.T, srv *httptest.Server, mutate func(*payment.Options)) payment.Payment {
	t.Helper()

	opts := payment.Options{
		SecretKey:     "sk_test_x",
		WebhookSecret: "whsec_test",
		Endpoint:      srv.URL,
	}
	if mutate != nil {
		mutate(&opts)
	}

	p, err := New(opts)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	return p
}

// sign builds a Stripe-Signature header for payload.
func sign(payload []byte, secret string) string {
	now := time.Now()
	sig := webhook.ComputeSignature(now, payload, secret)

	return "t=" + strconv.FormatInt(now.Unix(), 10) + ",v1=" + hex.EncodeToString(sig)
}

// eventPayload builds a Stripe event envelope with the SDK API version.
func eventPayload(typ string, obj any) []byte {
	raw, _ := json.Marshal(obj)

	env := map[string]any{
		"id":          "evt_test_1",
		"object":      "event",
		"api_version": stripe.APIVersion,
		"type":        typ,
		"data": map[string]any{
			"object": json.RawMessage(raw),
		},
	}

	out, _ := json.Marshal(env)

	return out
}

// piObject builds a PaymentIntent JSON object.
func piObject(id string, status string) map[string]any {
	return map[string]any{
		"id":       id,
		"object":   "payment_intent",
		"amount":   2000,
		"currency": "usd",
		"status":   status,
	}
}

func TestStripe_Open_guards(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    payment.Options
		wantErr error
	}{
		{
			name:    "missing secret",
			opts:    payment.Options{WebhookSecret: "whsec_test"},
			wantErr: ErrMissingSecretKey,
		},
		{
			name:    "missing webhook secret",
			opts:    payment.Options{SecretKey: "sk_test_x"},
			wantErr: ErrMissingWebhookSecret,
		},
		{
			name: "invalid options",
			opts: payment.Options{
				SecretKey:       "sk_test_x",
				WebhookSecret:   "whsec_test",
				MaxWebhookBytes: -5,
			},
			wantErr: payment.ErrInvalidOptions,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := New(tc.opts); !errors.Is(err, tc.wantErr) {
				t.Fatalf("New() err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestStripe_CreatePayment_roundtrip(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	got, err := p.CreatePayment(t.Context(), payment.Request{
		Amount:   2000,
		Currency: "USD",
		Meta:     map[string]string{"idempotency_key": "key-123", "order": "1"},
	})
	if err != nil {
		t.Fatalf("CreatePayment() err = %v", err)
	}

	want := payment.Result{ID: "pi_test_123", Status: payment.PaymentSucceeded, Amount: 2000, Currency: "usd"}
	if got != want {
		t.Fatalf("CreatePayment() = %+v, want %+v", got, want)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.idempotencyKey != "key-123" {
		t.Fatalf("Idempotency-Key = %q, want %q", f.idempotencyKey, "key-123")
	}
}

func TestStripe_CreatePayment_autoIdempotencyKey(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	if _, err := p.CreatePayment(t.Context(), payment.Request{Amount: 500, Currency: "eur"}); err != nil {
		t.Fatalf("CreatePayment() err = %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// The SDK auto-generates a per-request key when none is supplied.
	if f.idempotencyKey == "" {
		t.Fatal("Idempotency-Key empty, want SDK-generated key")
	}
}

func TestStripe_CreatePayment_guards(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	cases := []struct {
		name    string
		req     payment.Request
		wantErr error
	}{
		{"zero amount", payment.Request{Amount: 0, Currency: "usd"}, payment.ErrInvalidAmount},
		{"negative amount", payment.Request{Amount: -1, Currency: "usd"}, payment.ErrInvalidAmount},
		{"missing currency", payment.Request{Amount: 100}, payment.ErrMissingCurrency},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := p.CreatePayment(t.Context(), tc.req)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CreatePayment() err = %v, want %v", err, tc.wantErr)
			}

			if got != (payment.Result{}) {
				t.Fatalf("CreatePayment() = %+v, want zero Result", got)
			}
		})
	}
}

func TestStripe_CreatePayment_sdkError(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	f.setMode("create402")
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	got, err := p.CreatePayment(t.Context(), payment.Request{Amount: 2000, Currency: "usd"})
	if err == nil {
		t.Fatal("CreatePayment() err = nil, want SDK error")
	}

	if !strings.HasPrefix(err.Error(), "stripe: create:") {
		t.Fatalf("CreatePayment() err = %v, want stripe: create: prefix", err)
	}

	if got != (payment.Result{}) {
		t.Fatalf("CreatePayment() = %+v, want zero Result", got)
	}
}

func TestStripe_Refund_roundtrip(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	if err := p.Refund(t.Context(), "pi_test_123", 500); err != nil {
		t.Fatalf("Refund() err = %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.refunds != 1 {
		t.Fatalf("refunds = %d, want 1", f.refunds)
	}
}

func TestStripe_Refund_guards(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	if err := p.Refund(t.Context(), "pi_test_123", 0); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Fatalf("Refund() err = %v, want ErrInvalidAmount", err)
	}

	if err := p.Refund(t.Context(), "", 500); !errors.Is(err, payment.ErrMissingPaymentID) {
		t.Fatalf("Refund() err = %v, want ErrMissingPaymentID", err)
	}
}

func TestStripe_Refund_sdkError(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	f.setMode("refund402")
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	err := p.Refund(t.Context(), "pi_test_123", 500)
	if err == nil {
		t.Fatal("Refund() err = nil, want SDK error")
	}

	if !strings.HasPrefix(err.Error(), "stripe: refund:") {
		t.Fatalf("Refund() err = %v, want stripe: refund: prefix", err)
	}
}

func TestStripe_GetPayment_roundtrip(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	got, err := p.GetPayment(t.Context(), "pi_test_123")
	if err != nil {
		t.Fatalf("GetPayment() err = %v", err)
	}

	want := payment.Result{ID: "pi_test_123", Status: payment.PaymentSucceeded, Amount: 2000, Currency: "usd"}
	if got != want {
		t.Fatalf("GetPayment() = %+v, want %+v", got, want)
	}
}

func TestStripe_GetPayment_guards(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	got, err := p.GetPayment(t.Context(), "")
	if !errors.Is(err, payment.ErrMissingPaymentID) {
		t.Fatalf("GetPayment() err = %v, want ErrMissingPaymentID", err)
	}

	if got != (payment.Result{}) {
		t.Fatalf("GetPayment() = %+v, want zero Result", got)
	}
}

func TestStripe_GetPayment_sdkError(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	f.setMode("get404")
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	got, err := p.GetPayment(t.Context(), "pi_missing")
	if err == nil {
		t.Fatal("GetPayment() err = nil, want SDK error")
	}

	if !strings.HasPrefix(err.Error(), "stripe: get:") {
		t.Fatalf("GetPayment() err = %v, want stripe: get: prefix", err)
	}

	if got != (payment.Result{}) {
		t.Fatalf("GetPayment() = %+v, want zero Result", got)
	}
}

func TestStripe_WebhookEvent_table(t *testing.T) {
	t.Parallel()

	const secret = "whsec_test"

	chargeLinked := map[string]any{
		"id":              "ch_123",
		"object":          "charge",
		"amount":          2000,
		"amount_refunded": 200,
		"currency":        "usd",
		"status":          "succeeded",
		"payment_intent":  map[string]any{"id": "pi_456", "object": "payment_intent"},
		"refunds": map[string]any{
			"object": "list",
			"data": []any{
				map[string]any{"id": "re_1", "object": "refund", "amount": 500},
			},
		},
	}

	chargeBare := map[string]any{
		"id":              "ch_789",
		"object":          "charge",
		"amount":          2000,
		"amount_refunded": 200,
		"currency":        "usd",
		"status":          "succeeded",
	}

	cases := []struct {
		name     string
		typ      string
		obj      any
		signWith string
		check    func(t *testing.T, p payment.Payment, raw []byte)
		checkErr func(t *testing.T, err error)
	}{
		{
			name: "payment intent succeeded",
			typ:  "payment_intent.succeeded",
			obj:  piObject("pi_123", "succeeded"),
			check: func(t *testing.T, p payment.Payment, raw []byte) {
				t.Helper()

				got, err := p.WebhookEvent(t.Context(), raw, sign(raw, secret))
				if err != nil {
					t.Fatalf("WebhookEvent() err = %v", err)
				}

				want := payment.Event{
					Type:   "payment_intent.succeeded",
					Object: payment.Result{ID: "pi_123", Status: payment.PaymentSucceeded, Amount: 2000, Currency: "usd"},
				}
				if got != want {
					t.Fatalf("WebhookEvent() = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "payment intent failed",
			typ:  "payment_intent.payment_failed",
			obj:  piObject("pi_124", "requires_payment_method"),
			check: func(t *testing.T, p payment.Payment, raw []byte) {
				t.Helper()

				got, err := p.WebhookEvent(t.Context(), raw, sign(raw, secret))
				if err != nil {
					t.Fatalf("WebhookEvent() err = %v", err)
				}

				if got.Type != "payment_intent.payment_failed" {
					t.Fatalf("WebhookEvent() type = %q", got.Type)
				}

				if got.Object.ID != "pi_124" || got.Object.Amount != 2000 {
					t.Fatalf("WebhookEvent() object = %+v", got.Object)
				}
			},
		},
		{
			name: "charge refunded with pi link and tail refund",
			typ:  "charge.refunded",
			obj:  chargeLinked,
			check: func(t *testing.T, p payment.Payment, raw []byte) {
				t.Helper()

				got, err := p.WebhookEvent(t.Context(), raw, sign(raw, secret))
				if err != nil {
					t.Fatalf("WebhookEvent() err = %v", err)
				}

				want := payment.Event{
					Type:   "charge.refunded",
					Object: payment.Result{ID: "pi_456", Status: payment.PaymentSucceeded, Amount: 500, Currency: "usd"},
				}
				if got != want {
					t.Fatalf("WebhookEvent() = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "charge refunded without pi link",
			typ:  "charge.refunded",
			obj:  chargeBare,
			check: func(t *testing.T, p payment.Payment, raw []byte) {
				t.Helper()

				got, err := p.WebhookEvent(t.Context(), raw, sign(raw, secret))
				if err != nil {
					t.Fatalf("WebhookEvent() err = %v", err)
				}

				want := payment.Event{
					Type:   "charge.refunded",
					Object: payment.Result{ID: "ch_789", Status: payment.PaymentSucceeded, Amount: 200, Currency: "usd"},
				}
				if got != want {
					t.Fatalf("WebhookEvent() = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "unknown type preserves type with empty object",
			typ:  "customer.created",
			obj:  map[string]any{"id": "cus_1", "object": "customer"},
			check: func(t *testing.T, p payment.Payment, raw []byte) {
				t.Helper()

				got, err := p.WebhookEvent(t.Context(), raw, sign(raw, secret))
				if err != nil {
					t.Fatalf("WebhookEvent() err = %v", err)
				}

				want := payment.Event{Type: "customer.created"}
				if got != want {
					t.Fatalf("WebhookEvent() = %+v, want %+v", got, want)
				}
			},
		},
		{
			name:     "bad signature",
			typ:      "payment_intent.succeeded",
			obj:      piObject("pi_123", "succeeded"),
			signWith: "whsec_wrong",
			checkErr: func(t *testing.T, err error) {
				t.Helper()

				// The client verification path surfaces stripe's own
				// sentinel, which is distinct from webhook.ErrNoValidSignature.
				if !errors.Is(err, stripe.ErrWebhookNoValidSignature) {
					t.Fatalf("WebhookEvent() err = %v, want ErrWebhookNoValidSignature", err)
				}
			},
		},
		{
			// Valid as a generic map but not as a PaymentIntent, so the
			// failure surfaces from the adapter's own decode step.
			name: "corrupt inner json",
			typ:  "payment_intent.succeeded",
			obj: map[string]any{
				"id":       "pi_123",
				"object":   "payment_intent",
				"amount":   "bogus",
				"currency": "usd",
				"status":   "succeeded",
			},
			checkErr: func(t *testing.T, err error) {
				t.Helper()

				if !strings.HasPrefix(err.Error(), "stripe: webhook: unmarshal:") {
					t.Fatalf("WebhookEvent() err = %v, want unmarshal prefix", err)
				}
			},
		},
		{
			name: "corrupt charge json",
			typ:  "charge.refunded",
			obj: map[string]any{
				"id":       "ch_1",
				"object":   "charge",
				"amount":   "bogus",
				"currency": "usd",
				"status":   "succeeded",
			},
			checkErr: func(t *testing.T, err error) {
				t.Helper()

				if !strings.HasPrefix(err.Error(), "stripe: webhook: unmarshal:") {
					t.Fatalf("WebhookEvent() err = %v, want unmarshal prefix", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeStripe{piID: "pi_test_123"}
			srv := newFake(t, f)
			p := mustOpen(t, srv, nil)

			raw := eventPayload(tc.typ, tc.obj)

			key := secret
			if tc.signWith != "" {
				key = tc.signWith
			}

			if tc.checkErr != nil {
				_, err := p.WebhookEvent(t.Context(), raw, sign(raw, key))
				if err == nil {
					t.Fatal("WebhookEvent() err = nil, want error")
				}

				tc.checkErr(t, err)

				return
			}

			tc.check(t, p, raw)
		})
	}
}

func TestStripe_WebhookEvent_oversized(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, func(o *payment.Options) {
		o.MaxWebhookBytes = 32
	})

	raw := eventPayload("payment_intent.succeeded", piObject("pi_123", "succeeded"))

	_, err := p.WebhookEvent(t.Context(), raw, sign(raw, "whsec_test"))
	if err == nil {
		t.Fatal("WebhookEvent() err = nil, want SizeLimitError")
	}

	var sizeErr *payment.SizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("WebhookEvent() err = %v, want SizeLimitError", err)
	}

	if sizeErr.Size != len(raw) || sizeErr.Limit != 32 {
		t.Fatalf("SizeLimitError = %+v, want size %d limit 32", sizeErr, len(raw))
	}

	if !errors.Is(err, payment.ErrWebhookTooLarge) {
		t.Fatalf("WebhookEvent() err = %v, want ErrWebhookTooLarge", err)
	}
}

func TestStripe_WebhookEvent_missingSecret(t *testing.T) {
	t.Parallel()

	d := &driver{webhookSecret: "", maxWebhookBytes: payment.DefaultMaxWebhookBytes}

	_, err := d.WebhookEvent(t.Context(), []byte("{}"), "t=1,v1=abc")
	if !errors.Is(err, ErrMissingWebhookSecret) {
		t.Fatalf("WebhookEvent() err = %v, want ErrMissingWebhookSecret", err)
	}
}

func TestStripe_WebhookEvent_defensiveMaxBytes(t *testing.T) {
	t.Parallel()

	d := &driver{maxWebhookBytes: -1}

	_, err := d.WebhookEvent(t.Context(), []byte("{}"), "t=1,v1=abc")
	if err == nil {
		t.Fatal("WebhookEvent() err = nil, want invalid max size")
	}

	if !strings.Contains(err.Error(), "invalid max size -1") {
		t.Fatalf("WebhookEvent() err = %v, want invalid max size", err)
	}
}

func TestStripe_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		writeJSON(w, map[string]any{"id": "pi_slow"})
	}))
	t.Cleanup(srv.Close)

	// Only BackendConfig matters for timeouts; the stored httpClient is inert.
	httpClient := &http.Client{Timeout: 50 * time.Millisecond}

	cfg := &stripe.BackendConfig{HTTPClient: httpClient}
	cfg.URL = stripe.String(srv.URL)

	client := stripe.NewClient("sk_test_x", stripe.WithBackends(stripe.NewBackendsWithConfig(cfg)))
	d := &driver{client: client, httpClient: httpClient, webhookSecret: "whsec_test", maxWebhookBytes: payment.DefaultMaxWebhookBytes}

	_, err := d.CreatePayment(t.Context(), payment.Request{Amount: 100, Currency: "usd"})
	if err == nil {
		t.Fatal("CreatePayment() err = nil, want timeout")
	}

	if !strings.HasPrefix(err.Error(), "stripe: create:") {
		t.Fatalf("CreatePayment() err = %v, want stripe: create: prefix", err)
	}
}

func TestStripe_ConcurrentDriversNoKeyBleed(t *testing.T) {
	t.Parallel()

	fA := &fakeStripe{piID: "pi_A"}
	srvA := newFake(t, fA)
	pA := mustOpen(t, srvA, nil)

	fB := &fakeStripe{piID: "pi_B"}
	srvB := newFake(t, fB)
	pB := mustOpen(t, srvB, nil)

	const workers = 8

	var wg sync.WaitGroup

	errs := make([]error, workers*2)
	got := make([]payment.Result, workers*2)

	for i := range workers {
		wg.Add(2)

		go func(i int) {
			defer wg.Done()

			res, err := pA.CreatePayment(t.Context(), payment.Request{Amount: 100, Currency: "usd"})
			errs[i*2] = err
			got[i*2] = res
		}(i)

		go func(i int) {
			defer wg.Done()

			res, err := pB.CreatePayment(t.Context(), payment.Request{Amount: 100, Currency: "usd"})
			errs[i*2+1] = err
			got[i*2+1] = res
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("CreatePayment() [%d] err = %v", i, err)
		}
	}

	for i, res := range got {
		want := "pi_A"
		if i%2 == 1 {
			want = "pi_B"
		}

		if res.ID != want {
			t.Fatalf("CreatePayment() [%d] = %q, want %q", i, res.ID, want)
		}
	}
}

func TestStripe_Close(t *testing.T) {
	t.Parallel()

	f := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, f)
	p := mustOpen(t, srv, nil)

	if err := p.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}
}

func TestStripe_Open_defaultEndpoint(t *testing.T) {
	t.Parallel()

	p, err := New(payment.Options{SecretKey: "sk_test_x", WebhookSecret: "whsec_test"})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}
}
