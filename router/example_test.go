package router_test

import (
	"net/http"

	"github.com/zenta-dev/zever/router"
	"github.com/zenta-dev/zever/router/stdhttp"
)

// ExampleOpen opens the stdhttp router and registers a route.
func ExampleOpen() {
	_ = router.Register(router.AdapterStdHTTP, stdhttp.New)

	r, err := router.Open(router.AdapterStdHTTP, router.Options{AppName: "example"})
	if err != nil {
		return
	}

	r.Handle(http.MethodGet, "/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {})
	_ = r
}
