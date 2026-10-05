package http

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/webhook"
)

func TestEdgeRegisterMalformedTarget(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)
	ctx := t.Context()

	for _, target := range []string{"://bad", "ftp://example.com/hook", "http://", "not a url"} {
		err := w.Register(ctx, "e", target, "s")
		if err == nil {
			t.Fatalf("Register target %q expected error, got nil", target)
		}

		if !strings.Contains(err.Error(), "http: reject target") {
			t.Fatalf("Register target %q err = %v, want reject target prefix", target, err)
		}
	}
}

func TestEdgeRegisterDuplicateTargetOverwrite(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var gotSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotSig = r.Header.Get("X-Hub-Signature-256")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	ctx := t.Context()
	payload := []byte(`{"a":1}`)

	if err := w.Register(ctx, "e", srv.URL, "first"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Register(ctx, "e", srv.URL, "second"); err != nil {
		t.Fatalf("re-Register err = %v", err)
	}

	if err := w.Deliver(ctx, "e", payload); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	assertValidSignature(t, "second", payload, gotSig)
}

func TestEdgeUnregisterEmptyEvent(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)

	if err := w.Unregister(t.Context(), "", "http://127.0.0.1/"); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Unregister empty event err = %v, want ErrNotFound", err)
	}
}

func TestEdgeDeliverEmptyPayload(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	ctx := t.Context()

	if err := w.Register(ctx, "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Deliver(ctx, "e", nil); err != nil {
		t.Fatalf("Deliver nil payload err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(gotBody) != 0 {
		t.Fatalf("body = %q, want empty", gotBody)
	}
}

func TestEdgeDeliverLargePayload(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	ctx := t.Context()

	if err := w.Register(ctx, "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = 'x'
	}

	if err := w.Deliver(ctx, "e", payload); err != nil {
		t.Fatalf("Deliver 1MiB err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(gotBody) != len(payload) {
		t.Fatalf("body = %d bytes, want %d", len(gotBody), len(payload))
	}
}

func TestEdgeDeliverMultiTargetErrorJoin(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	ctx := t.Context()

	if err := w.Register(ctx, "e", srv.URL+"/a", "s"); err != nil {
		t.Fatalf("Register a err = %v", err)
	}

	if err := w.Register(ctx, "e", srv.URL+"/b", "s"); err != nil {
		t.Fatalf("Register b err = %v", err)
	}

	err := w.Deliver(ctx, "e", []byte(`{}`))
	if err == nil {
		t.Fatal("Deliver expected joined error, got nil")
	}

	if !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("Deliver err = %v, want ErrDeliveryFailed", err)
	}

	if got := strings.Count(err.Error(), "delivery failed after 1 attempts"); got != 2 {
		t.Fatalf("Deliver err = %v, want 2 joined attempt errors", err)
	}
}

func TestEdgeSingleAttemptErrorBodyCap(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("b", 2048)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	ctx := t.Context()

	if err := w.Register(ctx, "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	err := w.Deliver(ctx, "e", []byte(`{}`))
	if err == nil {
		t.Fatal("Deliver expected error, got nil")
	}

	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("Deliver err = %v, want ErrUnexpectedStatus", err)
	}

	if !strings.Contains(err.Error(), body[:64]) {
		t.Fatalf("Deliver err = %v, want body prefix", err)
	}

	if strings.Contains(err.Error(), body[512:]) {
		t.Fatalf("Deliver err = %v, body not capped at 512 bytes", err)
	}
}

func TestEdgeDrainForErrorNilSafe(t *testing.T) {
	t.Parallel()

	a := mustAdapter(t, openPrivate(t, 1))

	if got := a.drainForError(nil); got != "" {
		t.Fatalf("drainForError(nil) = %q, want empty", got)
	}

	if got := a.drainForError(&http.Response{}); got != "" {
		t.Fatalf("drainForError(empty) = %q, want empty", got)
	}
}

func TestEdgeSignEmptyPayload(t *testing.T) {
	t.Parallel()

	a, b := signAt("s", nil, 1_700_000_000), signAt("s", nil, 1_700_000_000)
	if a != b {
		t.Fatalf("signAt empty payload not deterministic: %q vs %q", a, b)
	}

	if !strings.HasPrefix(a, "t=1700000000,v1=") {
		t.Fatalf("signAt empty payload = %q, want envelope prefix", a)
	}

	if other := signAt("s", []byte(`{}`), 1_700_000_000); a == other {
		t.Fatal("signAt empty payload must differ from non-empty payload")
	}
}

func TestEdgeCloseIdempotent(t *testing.T) {
	t.Parallel()

	w := openPrivate(t, 1)

	if err := w.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
}

func TestEdgeConcurrentUnregisterDeliver(t *testing.T) {
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

			event := fmt.Sprintf("edge%d", i%4)
			target := fmt.Sprintf("%s/%d", srv.URL, i)
			_ = w.Register(t.Context(), event, target, "s")
			_ = w.Deliver(t.Context(), event, []byte(`{}`))
			_ = w.Unregister(t.Context(), event, target)
		}()
	}

	wg.Wait()
}
