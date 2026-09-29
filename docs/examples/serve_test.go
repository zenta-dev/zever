package examples_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	slogadapter "github.com/zenta-dev/zever/adapters/log/slog"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/router"
)

// ExampleServe_buildHandlerOnly mirrors the routing doc sample without
// listening on a port: wire the stdhttp driver plus an slog logger,
// register a route and a versioned group, then serve two in-process
// requests.
func Example_serveBuildHandlerOnly() {
	logger := slogadapter.NewWithWriter(log.Options{}, io.Discard)
	r, err := routerstdhttp.New(router.Options{AppName: "docs-example", Logger: logger})
	if err != nil {
		fmt.Println("open error")
		return
	}

	r.Handle("GET", "/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "user %s", router.Param(req, "id"))
	})
	api := r.Group("/v1")
	api.Handle("GET", "/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "v1 user %s", router.Param(req, "id"))
	})

	for _, path := range []string{"/users/42", "/v1/users/42"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		fmt.Println(rec.Body.String())
	}
	// Output:
	// user 42
	// v1 user 42
}
