package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/webhook"
)

// assertValidSignature checks that got is a well-formed "t=<ts>,v1=<hex>"
// envelope whose HMAC matches secret and payload, and whose timestamp is
// recent (sanity check that sign used time.Now, not a stale/zero value).
func assertValidSignature(t *testing.T, secret string, payload []byte, got string) {
	t.Helper()

	tsPart, macPart, ok := strings.Cut(got, ",")
	if !ok {
		t.Fatalf("signature %q has no ',' separator", got)
	}

	tsStr, ok := strings.CutPrefix(tsPart, "t=")
	if !ok {
		t.Fatalf("signature %q missing t= field", got)
	}

	macHex, ok := strings.CutPrefix(macPart, "v1=")
	if !ok {
		t.Fatalf("signature %q missing v1= field", got)
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		t.Fatalf("signature %q has non-numeric timestamp: %v", got, err)
	}

	if age := time.Since(time.Unix(ts, 0)); age < -time.Minute || age > time.Minute {
		t.Errorf("signature timestamp %d is not close to now (age %v)", ts, age)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte{'.'})
	mac.Write(payload)

	want := hex.EncodeToString(mac.Sum(nil))
	if macHex != want {
		t.Errorf("signature mac = %q, want %q", macHex, want)
	}
}

func openPrivate(t *testing.T, maxRetries int) webhook.Webhook {
	t.Helper()

	w, err := New(webhook.Options{AllowPrivateTargets: true, MaxRetries: maxRetries})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	return w
}

func mustAdapter(t *testing.T, w webhook.Webhook) *adapter {
	t.Helper()

	a, ok := w.(*adapter)
	if !ok {
		t.Fatalf("webhook type = %T, want *adapter", w)
	}

	return a
}

func TestRegister_empty(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	ctx := t.Context()

	if err := w.Register(ctx, "", "http://127.0.0.1/", "s"); !errors.Is(err, ErrMissingEvent) {
		t.Errorf("Register empty event err = %v, want ErrMissingEvent", err)
	}

	if err := w.Register(ctx, "e", "", "s"); !errors.Is(err, ErrMissingTarget) {
		t.Errorf("Register empty target err = %v, want ErrMissingTarget", err)
	}
}

func TestRegister_rejectPrivate(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(srv.Close)

	if err := w.Register(t.Context(), "e", srv.URL, "s"); err == nil {
		t.Error("Register private target expected error, got nil")
	}
}

func TestRegister_rejectPrivateHTTPS(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	if err := w.Register(t.Context(), "e", "https://127.0.0.1/hook", "s"); err == nil {
		t.Error("Register https private target expected error, got nil")
	}
}

func TestRegister_allowPrivateAccept(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(srv.Close)

	if err := w.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Errorf("Register allow-private err = %v", err)
	}
}

func TestUnregister_missing(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	ctx := t.Context()

	if err := w.Unregister(ctx, "nope", "http://127.0.0.1/"); !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("Unregister missing event err = %v, want ErrNotFound", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(srv.Close)

	if err := w.Register(ctx, "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Unregister(ctx, "e", srv.URL+"/other"); !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("Unregister missing target err = %v, want ErrNotFound", err)
	}
}

func TestUnregister_okAndPrune(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	ctx := t.Context()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(srv.Close)

	if err := w.Register(ctx, "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Unregister(ctx, "e", srv.URL); err != nil {
		t.Fatalf("Unregister err = %v", err)
	}

	if err := w.Deliver(ctx, "e", []byte(`{}`)); !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("Deliver after prune err = %v, want ErrNotFound", err)
	}
}

func TestDeliver_unknownEvent(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	if err := w.Deliver(t.Context(), "missing", []byte(`{}`)); !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("Deliver unknown err = %v, want ErrNotFound", err)
	}
}

func TestDeliver_successHeaders(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var gotMethod, gotCT, gotSig, gotPath string

	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotSig = r.Header.Get("X-Hub-Signature-256")
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	ctx := t.Context()
	payload := []byte(`{"a":1}`)

	if err := w.Register(ctx, "e", srv.URL, "secret"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Deliver(ctx, "e", payload); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}

	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}

	assertValidSignature(t, "secret", payload, gotSig)

	if string(gotBody) != string(payload) {
		t.Errorf("body = %q, want %q", gotBody, payload)
	}

	_ = gotPath
}

func TestDeliver_noSignatureWhenSecretEmpty(t *testing.T) {
	t.Parallel()

	var gotSig string

	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotSig = r.Header.Get("X-Hub-Signature-256")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	ctx := t.Context()

	if err := w.Register(ctx, "e", srv.URL, ""); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Deliver(ctx, "e", []byte(`{}`)); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if gotSig != "" {
		t.Errorf("signature = %q, want empty", gotSig)
	}
}

func TestDeliver_retryThenSuccess(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 3)
	ctx := t.Context()

	if err := w.Register(ctx, "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	// Speed up backoff for the test by shrinking? backoff is ~500ms; accept it.
	if err := w.Deliver(ctx, "e", []byte(`{}`)); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	if got := hits.Load(); got != 3 {
		t.Errorf("hits = %d, want 3", got)
	}
}

func TestDeliver_exhausted(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	w, err := New(webhook.Options{AllowPrivateTargets: true, MaxRetries: 3})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	// Shrink backoff sleeps by... backoff fixed ~500ms; use maxRetries 1? No,
	// spec demands "delivery failed after 3 attempts". Keep 3; test takes ~1s.
	ctx := t.Context()

	if regErr := w.Register(ctx, "e", srv.URL, "s"); regErr != nil {
		t.Fatalf("Register err = %v", regErr)
	}

	err = w.Deliver(ctx, "e", []byte(`{}`))
	if err == nil {
		t.Fatal("Deliver expected error, got nil")
	}

	if !strings.Contains(err.Error(), "delivery failed after 3 attempts") {
		t.Errorf("Deliver err = %q, want substring %q", err, "delivery failed after 3 attempts")
	}
}

func TestDeliver_statusCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		code    int
		wantErr bool
	}{
		{"201 ok", http.StatusCreated, false},
		{"204 ok", http.StatusNoContent, false},
		{"299 ok", 299, false},
		{"404 fail", http.StatusNotFound, true},
		{"302 fail", http.StatusFound, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.code)
			}))
			t.Cleanup(srv.Close)

			w := openPrivate(t, 1)
			ctx := t.Context()

			if err := w.Register(ctx, "e", srv.URL, ""); err != nil {
				t.Fatalf("Register err = %v", err)
			}

			err := w.Deliver(ctx, "e", []byte(`{}`))
			if (err != nil) != c.wantErr {
				t.Errorf("Deliver code %d err = %v, wantErr %v", c.code, err, c.wantErr)
			}
		})
	}
}

func TestDeliver_doError(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	ctx := t.Context()

	// Port 1 is (almost) certainly closed; dial fails covering Do-error + drain(nil).
	if err := w.Register(ctx, "e", "http://127.0.0.1:1/hook", ""); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Deliver(ctx, "e", []byte(`{}`)); err == nil {
		t.Error("Deliver expected dial error, got nil")
	}
}

func TestDeliver_ctxCancelDuringBackoff(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	w, err := New(webhook.Options{AllowPrivateTargets: true, MaxRetries: 3})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	ctx := t.Context()

	if regErr := w.Register(ctx, "e", srv.URL, ""); regErr != nil {
		t.Fatalf("Register err = %v", regErr)
	}

	cancelCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	err = w.Deliver(cancelCtx, "e", []byte(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Deliver err = %v, want context.DeadlineExceeded", err)
	}
}

func TestValidateIfNeeded_blocked(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	a := mustAdapter(t, w)
	if err := a.validateIfNeeded(t.Context(), "https://127.0.0.1/hook"); err == nil {
		t.Error("validateIfNeeded expected blocked error, got nil")
	} else if !strings.Contains(err.Error(), "blocked private target") {
		t.Errorf("validateIfNeeded err = %q, want blocked private target", err)
	}
}

func TestValidateIfNeeded_allowPrivate(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	a := mustAdapter(t, w)

	if err := a.validateIfNeeded(t.Context(), "https://127.0.0.1/hook"); err != nil {
		t.Errorf("validateIfNeeded allow-private err = %v", err)
	}
}

func TestValidateIfNeeded_publicOK(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	a := mustAdapter(t, w)
	// 8.8.8.8 is a public IP literal: no DNS, no network.
	if err := a.validateIfNeeded(t.Context(), "https://8.8.8.8/hook"); err != nil {
		t.Errorf("validateIfNeeded public err = %v", err)
	}
}

func TestDeliver_blockedPrivateAtDelivery(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	ctx := t.Context()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if err := w.Register(ctx, "e", srv.URL, ""); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	// Flip to strict after register: Deliver must re-validate at dial time.
	mustAdapter(t, w).allowPrivate = false

	err := w.Deliver(ctx, "e", []byte(`{}`))
	if err == nil {
		t.Fatal("Deliver expected blocked error, got nil")
	}

	if !strings.Contains(err.Error(), "blocked private target") {
		t.Errorf("Deliver err = %q, want blocked private target", err)
	}
}

func TestSingleAttempt_buildRequestError(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	a := mustAdapter(t, w)

	err := a.singleAttempt(t.Context(), registration{target: "://bad"}, []byte(`{}`))
	if err == nil {
		t.Error("singleAttempt bad target expected error, got nil")
	} else if !strings.Contains(err.Error(), "create request") {
		t.Errorf("singleAttempt err = %q, want create request", err)
	}
}

func TestBackoff_table(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	a := mustAdapter(t, w)

	inRange := func(d, lo, hi time.Duration) bool { return d >= lo && d <= hi }

	if got := a.backoff(0); !inRange(got, 500*time.Millisecond, 750*time.Millisecond) {
		t.Errorf("backoff(0) = %v, want [500ms,750ms]", got)
	}

	if got := a.backoff(1); !inRange(got, 500*time.Millisecond, 750*time.Millisecond) {
		t.Errorf("backoff(1) = %v, want [500ms,750ms]", got)
	}

	if got := a.backoff(5); !inRange(got, 2500*time.Millisecond, 2750*time.Millisecond) {
		t.Errorf("backoff(5) = %v, want [2500ms,2750ms]", got)
	}

	lo1000, hi1000 := 500*time.Second, 500*time.Second+250*time.Millisecond
	if got := a.backoff(1000); !inRange(got, lo1000, hi1000) {
		t.Errorf("backoff(1000) = %v, want [500s,500.25s]", got)
	}

	// Absurd attempts overflow the multiplication; both maxDelay guards fire.
	if got := a.backoff(1 << 40); got != 72*time.Hour {
		t.Errorf("backoff(1<<40) = %v, want exactly 72h", got)
	}
}

func TestDrain(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	a := mustAdapter(t, w)

	a.drain(nil)
	a.drain(&http.Response{})
	a.drain(&http.Response{Body: io.NopCloser(strings.NewReader("hello"))})
}

func TestSign_determinism(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"x":1}`)
	// signAt pins the timestamp so two calls with the same inputs are
	// byte-identical; sign() alone would legitimately vary across the
	// second boundary since it stamps time.Now().
	a, b := signAt("s", payload, 1_700_000_000), signAt("s", payload, 1_700_000_000)

	if a != b {
		t.Errorf("signAt not deterministic: %q vs %q", a, b)
	}

	if !strings.HasPrefix(a, "t=1700000000,v1=") {
		t.Errorf("signAt = %q, want t=1700000000,v1= prefix", a)
	}

	if c := signAt("other", payload, 1_700_000_000); a == c {
		t.Error("sign with different secret should differ")
	}

	assertValidSignature(t, "s", payload, sign("s", payload))
}

func TestBuildRequest_errorsAndHeaders(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	a := mustAdapter(t, w)
	ctx := t.Context()

	if _, err := a.buildRequest(ctx, registration{target: "://bad"}, []byte(`{}`)); err == nil {
		t.Error("buildRequest bad target expected error, got nil")
	} else if !strings.Contains(err.Error(), "create request") {
		t.Errorf("buildRequest err = %q, want create request", err)
	}

	req, err := a.buildRequest(ctx, registration{target: "http://127.0.0.1/", secret: ""}, []byte(`{}`))
	if err != nil {
		t.Fatalf("buildRequest err = %v", err)
	}

	if req.Header.Get("X-Hub-Signature-256") != "" {
		t.Errorf("empty secret should omit signature, got %q", req.Header.Get("X-Hub-Signature-256"))
	}

	req, err = a.buildRequest(ctx, registration{target: "http://127.0.0.1/", secret: "s"}, []byte(`{}`))
	if err != nil {
		t.Fatalf("buildRequest err = %v", err)
	}

	if req.Header.Get("X-Hub-Signature-256") == "" {
		t.Error("secret should set signature, got empty")
	}
}

func TestSleepWithContext(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	a := mustAdapter(t, w)

	if err := a.sleepWithContext(t.Context(), time.Millisecond); err != nil {
		t.Errorf("sleepWithContext err = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := a.sleepWithContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepWithContext canceled err = %v, want context.Canceled", err)
	}
}

func TestOpen_defaultsAndInvalid(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	a := mustAdapter(t, w)
	if a.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", a.timeout)
	}

	if a.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", a.maxRetries)
	}

	if a.allowPrivate {
		t.Error("allowPrivate = true, want false")
	}

	if _, err := New(webhook.Options{Timeout: -1}); err == nil {
		t.Error("Open negative timeout expected error, got nil")
	} else if !errors.Is(err, webhook.ErrInvalidOptions) {
		t.Errorf("Open err = %v, want ErrInvalidOptions", err)
	}

	if _, err := New(webhook.Options{MaxRetries: -2}); err == nil {
		t.Error("Open negative retries expected error, got nil")
	} else if !errors.Is(err, webhook.ErrInvalidOptions) {
		t.Errorf("Open err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpen_customValues(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{Timeout: 2 * time.Second, MaxRetries: 5, AllowPrivateTargets: true})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	a := mustAdapter(t, w)
	if a.timeout != 2*time.Second {
		t.Errorf("timeout = %v, want 2s", a.timeout)
	}

	if a.maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", a.maxRetries)
	}

	if !a.allowPrivate {
		t.Error("allowPrivate = false, want true")
	}

	if a.client == nil {
		t.Error("client should be non-nil")
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	if err := w.Close(); err != nil {
		t.Errorf("Close err = %v", err)
	}
}

func TestConcurrent_registerDeliver(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)

	var wg sync.WaitGroup

	for i := range 20 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			event := fmt.Sprintf("e%d", i%4)
			if err := w.Register(t.Context(), event, fmt.Sprintf("%s/%d", srv.URL, i), ""); err != nil {
				t.Errorf("Register err = %v", err)
				return
			}

			_ = w.Deliver(t.Context(), event, []byte(`{}`))
		}()
	}

	wg.Wait()
}
