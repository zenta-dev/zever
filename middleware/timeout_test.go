package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTimeout_FastHandlerUnaffected(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Custom", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	wrapped := Timeout(time.Second)(handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Code = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("X-Custom"); got != "yes" {
		t.Fatalf("X-Custom header = %q, want yes", got)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("Body = %q, want ok", rec.Body.String())
	}
}

func TestTimeout_SlowHandlerCutOff(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)

		select {
		case <-release:
		case <-r.Context().Done():
		}

		// The handler keeps running past the deadline and tries to write;
		// this must not race with, or corrupt, the timeout response below.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("too late"))
	})

	wrapped := Timeout(20 * time.Millisecond)(handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	<-started
	close(release)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("Code = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error != "request timed out" {
		t.Fatalf("body.Error = %q, want %q", body.Error, "request timed out")
	}
}

func TestTimeout_HandlerRespectsCtxDeadline(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		// Handler cooperates: stops promptly instead of writing late.
	})

	wrapped := Timeout(15 * time.Millisecond)(handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("Code = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}
}

func TestTimeoutUnaryServerInterceptor_FastHandlerUnaffected(t *testing.T) {
	t.Parallel()

	interceptor := TimeoutUnaryServerInterceptor(time.Second)

	handler := func(_ context.Context, req any) (any, error) {
		return req, nil
	}

	resp, err := interceptor(context.Background(), "hello", &grpc.UnaryServerInfo{}, handler)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if resp != "hello" {
		t.Fatalf("resp = %v, want hello", resp)
	}
}

func TestTimeoutUnaryServerInterceptor_SlowHandlerCutOff(t *testing.T) {
	t.Parallel()

	interceptor := TimeoutUnaryServerInterceptor(20 * time.Millisecond)

	started := make(chan struct{})

	handler := func(ctx context.Context, _ any) (any, error) {
		close(started)
		<-ctx.Done()

		return "too late", nil
	}

	_, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, handler)

	<-started

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("err = %v, want a gRPC status error", err)
	}
	if st.Code() != codes.DeadlineExceeded {
		t.Fatalf("code = %v, want %v", st.Code(), codes.DeadlineExceeded)
	}
}

func TestTimeoutUnaryServerInterceptor_HandlerErrorPassedThrough(t *testing.T) {
	t.Parallel()

	interceptor := TimeoutUnaryServerInterceptor(time.Second)

	wantErr := errors.New("boom")
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, wantErr
	}

	_, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, handler)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
