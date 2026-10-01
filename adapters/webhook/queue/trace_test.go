package queue

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProcessMessage_RequeuePreservesTrace verifies the consume-side gap is
// closed for webhooks: a delivery carrying a producer traceparent is
// extracted on consume and re-injected on the requeue PushDelayed, so the
// redelivered message keeps the same trace.
func TestProcessMessage_RequeuePreservesTrace(t *testing.T) {
	sq := newStubQueue()
	a := newTestAdapter(sq)

	const secret = "s3cret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	const event = "order.created"

	a.regs[event] = map[string]registration{
		srv.URL: {target: srv.URL, secret: secret},
	}

	const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	payload := []byte(`{"id":1}`)
	msg := testMessage("webhook:"+event, payload, map[string]string{
		"X-Webhook-Target":    srv.URL,
		"X-Webhook-Signature": signPayload(secret, payload),
		"traceparent":         traceparent,
	}, 1)

	a.processMessage(event, msg)

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.delayed) != 1 {
		t.Fatalf("delayed pushes = %d, want 1 (requeue after 500)", len(sq.delayed))
	}

	got := sq.delayed[0].push.headers["traceparent"]
	if got != traceparent {
		t.Errorf("requeued traceparent = %q, want %q", got, traceparent)
	}

	if got := sq.delayed[0].push.headers["X-Webhook-Attempt"]; got != "2" {
		t.Errorf("requeued X-Webhook-Attempt = %q, want %q", got, "2")
	}
}

// TestProcessMessage_DeadLetterPreservesTrace verifies the dead-letter push
// after exhausted retries keeps the producer traceparent.
func TestProcessMessage_DeadLetterPreservesTrace(t *testing.T) {
	sq := newStubQueue()
	a := newTestAdapter(sq)
	a.maxRetries = 1

	const secret = "s3cret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	const event = "order.created"

	a.regs[event] = map[string]registration{
		srv.URL: {target: srv.URL, secret: secret},
	}

	const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	payload := []byte(`{"id":1}`)
	msg := testMessage("webhook:"+event, payload, map[string]string{
		"X-Webhook-Target":    srv.URL,
		"X-Webhook-Signature": signPayload(secret, payload),
		"traceparent":         traceparent,
	}, 1)

	a.processMessage(event, msg)

	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.pushes) != 1 {
		t.Fatalf("dead-letter pushes = %d, want 1", len(sq.pushes))
	}

	got := sq.pushes[0].headers["traceparent"]
	if got != traceparent {
		t.Errorf("dead-letter traceparent = %q, want %q", got, traceparent)
	}
}
