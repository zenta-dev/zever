package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/shared/codec"
)

func TestTimeout_FastHandlerUnaffected(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Custom", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	wrapped := Timeout(time.Second)(handler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
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

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	<-started
	close(release)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("Code = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}

	var body errorBody
	if err := codec.Unmarshal(rec.Body.Bytes(), &body); err != nil {
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

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
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

	resp, err := interceptor(t.Context(), "hello", &grpc.UnaryServerInfo{}, handler)
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

	_, err := interceptor(t.Context(), "req", &grpc.UnaryServerInfo{}, handler)

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

	_, err := interceptor(t.Context(), "req", &grpc.UnaryServerInfo{}, handler)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestTimeout_HandlerImplicitStatusOK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		handler  http.Handler
		wantBody string
	}{
		{
			name: "write without WriteHeader",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("ok"))
			}),
			wantBody: "ok",
		},
		{
			name: "headers only",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Custom", "yes")
			}),
			wantBody: "",
		},
	}

	for _, tt := range tests {
		wrapped := Timeout(time.Second)(tt.handler)

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: Code = %d, want %d", tt.name, rec.Code, http.StatusOK)
		}
		if rec.Body.String() != tt.wantBody {
			t.Errorf("%s: Body = %q, want %q", tt.name, rec.Body.String(), tt.wantBody)
		}
		if tt.name == "headers only" {
			if got := rec.Header().Get("X-Custom"); got != "yes" {
				t.Errorf("%s: X-Custom header = %q, want yes", tt.name, got)
			}
		}
	}
}

func TestTimeout_HandlerPanicPropagatesToCaller(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	wrapped := Timeout(time.Second)(handler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	defer func() {
		if got := recover(); got != "boom" {
			t.Fatalf("recover() = %v, want boom re-panicked in the caller goroutine", got)
		}
	}()

	wrapped.ServeHTTP(rec, req)

	t.Fatal("ServeHTTP returned, want panic")
}

func TestTimeoutUnaryServerInterceptor_HandlerPanicBecomesInternal(t *testing.T) {
	t.Parallel()

	handler := func(context.Context, any) (any, error) {
		panic("boom")
	}

	_, err := TimeoutUnaryServerInterceptor(time.Second)(t.Context(), nil, &grpc.UnaryServerInfo{}, handler)
	if got := status.Code(err); got != codes.Internal {
		t.Fatalf("code = %v, want Internal (err=%v)", got, err)
	}
}

// TestTimeout_LatePanicSwallowed pins that a handler panic landing after the
// deadline neither propagates to the caller nor corrupts the timeout
// response: the client already has its 504, and there is no caller left to
// receive the panic.
func TestTimeout_LatePanicSwallowed(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		defer close(finished)
		<-release
		panic("late boom")
	})

	wrapped := Timeout(20 * time.Millisecond)(handler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("Code = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}

	<-started
	close(release)
	<-finished
}

// TestTimeoutWriter_DiscardAndIdempotentHeader pins the discard contract the
// timeout path relies on: writes after the deadline are dropped (never racing
// the 504), the first WriteHeader wins, and a header after the deadline is
// ignored.
func TestTimeoutWriter_DiscardAndIdempotentHeader(t *testing.T) {
	t.Parallel()

	t.Run("write after timeout discarded", func(t *testing.T) {
		t.Parallel()

		tw := &timeoutWriter{header: make(http.Header), timedOut: true}
		n, err := tw.Write([]byte("late"))
		if err != nil {
			t.Fatalf("Write error = %v", err)
		}
		if n != len("late") {
			t.Errorf("Write n = %d, want %d", n, len("late"))
		}
		if tw.buf.Len() != 0 {
			t.Errorf("buffered %d bytes, want 0 (discarded after timeout)", tw.buf.Len())
		}
	})

	t.Run("first WriteHeader wins", func(t *testing.T) {
		t.Parallel()

		tw := &timeoutWriter{header: make(http.Header)}
		tw.WriteHeader(http.StatusCreated)
		tw.WriteHeader(http.StatusTeapot)
		if tw.code != http.StatusCreated {
			t.Errorf("code = %d, want %d (first wins)", tw.code, http.StatusCreated)
		}
	})

	t.Run("WriteHeader after timeout ignored", func(t *testing.T) {
		t.Parallel()

		tw := &timeoutWriter{header: make(http.Header), timedOut: true}
		tw.WriteHeader(http.StatusCreated)
		if tw.code != 0 {
			t.Errorf("code = %d, want 0 (ignored after timeout)", tw.code)
		}
	})
}
