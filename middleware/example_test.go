package middleware_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/zenta-dev/zever/log/noop"
	"github.com/zenta-dev/zever/middleware"
)

// ExampleRequestLogger wraps a handler and logs one line per request.
func ExampleRequestLogger() {
	handler := middleware.RequestLogger(noop.New())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	fmt.Println(rec.Code)

	// Output: 200
}
