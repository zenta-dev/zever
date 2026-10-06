package twilio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	twilioclient "github.com/twilio/twilio-go/client"
)

// benchStub serves canned Twilio Messages responses.
func benchStub(b *testing.B, status int, respBody string) *httptest.Server {
	b.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
}

// benchNotifier builds a notifier whose HTTP traffic is redirected at srv.
func benchNotifier(b *testing.B, srv *httptest.Server) *twilioNotifier {
	b.Helper()
	n, err := New(validOptions())
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	tn, ok := n.(*twilioNotifier)
	if !ok {
		b.Fatalf("New returned %T, want *twilioNotifier", n)
	}
	bc, ok := tn.client.Client.(*twilioclient.Client)
	if !ok || bc.HTTPClient == nil {
		b.Fatalf("notifier client is %T, want *twilioclient.Client with HTTPClient", tn.client.Client)
	}
	prev := bc.HTTPClient.Transport
	if prev == nil {
		prev = http.DefaultTransport
	}
	bc.HTTPClient.Transport = &rewriteTransport{base: srv.URL, rt: prev}
	return tn
}

// BenchmarkNotify measures the full send path (validation, request build,
// HTTP round-trip, response decode) against an in-process stub server.
func BenchmarkNotify(b *testing.B) {
	srv := benchStub(b, http.StatusCreated, `{"sid":"SM123","status":"queued"}`)
	defer srv.Close()
	n := benchNotifier(b, srv)
	defer n.Close()
	in := validSMS()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := n.Notify(b.Context(), in); err != nil {
			b.Fatalf("Notify() = %v, want nil", err)
		}
	}
}

// BenchmarkNotifyParallel measures concurrent sends through one notifier.
func BenchmarkNotifyParallel(b *testing.B) {
	srv := benchStub(b, http.StatusCreated, `{"sid":"SM123","status":"queued"}`)
	defer srv.Close()
	n := benchNotifier(b, srv)
	defer n.Close()
	in := validSMS()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := n.Notify(b.Context(), in); err != nil {
				b.Errorf("Notify() = %v, want nil", err)
				return
			}
		}
	})
}

// BenchmarkClose measures the idempotent close.
func BenchmarkClose(b *testing.B) {
	n, err := New(validOptions())
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := n.Close(); err != nil {
			b.Fatalf("Close() = %v, want nil", err)
		}
	}
}

// BenchmarkNew measures constructing the notifier and its HTTP client.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		n, err := New(validOptions())
		if err != nil {
			b.Fatalf("New() = %v", err)
		}
		if n == nil {
			b.Fatal("New() = nil")
		}
	}
}

// BenchmarkLimitedTransport measures the response-body limiting wrapper.
func BenchmarkLimitedTransport(b *testing.B) {
	lt := &limitedTransport{base: http.DefaultTransport}
	req, err := http.NewRequestWithContext(b.Context(), http.MethodGet, "http://127.0.0.1:1/", nil)
	if err != nil {
		b.Fatalf("NewRequest() = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		resp, err := lt.RoundTrip(req)
		if err == nil && resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}
}

// BenchmarkDeadlineTimeout measures the ctx-deadline clamp decision.
func BenchmarkDeadlineTimeout(b *testing.B) {
	n, err := New(validOptions())
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	defer n.Close()
	tn, ok := n.(*twilioNotifier)
	if !ok {
		b.Fatalf("New() = %T, want *twilioNotifier", n)
	}
	ctx, cancel := context.WithDeadline(b.Context(), time.Now().Add(time.Hour))
	defer cancel()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, clamped := tn.deadlineTimeout(ctx); !clamped {
			b.Fatal("deadlineTimeout() = unclamped, want clamped")
		}
	}
}

// BenchmarkApiForCall measures acquiring the per-call API handle.
func BenchmarkApiForCall(b *testing.B) {
	n, err := New(validOptions())
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	defer n.Close()
	tn, ok := n.(*twilioNotifier)
	if !ok {
		b.Fatalf("New() = %T, want *twilioNotifier", n)
	}
	ctx, cancel := context.WithDeadline(b.Context(), time.Now().Add(time.Hour))
	defer cancel()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		api, err := tn.apiForCall(ctx)
		if err != nil {
			b.Fatalf("apiForCall() = %v", err)
		}
		if api == nil {
			b.Fatal("apiForCall() = nil")
		}
	}
}
