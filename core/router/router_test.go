package router

import (
	"net/http"
	"testing"
)

var routerTestSeq int32 = 2000

type mockRouter struct{}

func (m *mockRouter) Handle(_ string, _ string, _ http.HandlerFunc) {}

func (m *mockRouter) Group(_ string, _ ...func(http.Handler) http.Handler) Group {
	return m
}

func (m *mockRouter) Use(_ ...func(http.Handler) http.Handler) {}

func (m *mockRouter) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {}

func TestValidMethod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		method string
		want   bool
	}{
		{name: "get upper", method: "GET", want: true},
		{name: "get lower", method: "get", want: true},
		{name: "post padded", method: "  post  ", want: true},
		{name: "put", method: "PUT", want: true},
		{name: "delete", method: "DELETE", want: true},
		{name: "patch", method: "PATCH", want: true},
		{name: "head", method: "HEAD", want: true},
		{name: "options", method: "OPTIONS", want: true},
		{name: "trace", method: "TRACE", want: true},
		{name: "connect", method: "CONNECT", want: true},
		{name: "connect lower", method: "connect", want: true},
		{name: "empty", method: "", want: false},
		{name: "blank", method: "   ", want: false},
		{name: "invalid", method: "FETCH", want: false},
		{name: "zero", method: "\x00", want: false},
		{name: "partial", method: "GE", want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidMethod(tc.method); got != tc.want {
				t.Fatalf("ValidMethod(%q) = %v, want %v", tc.method, got, tc.want)
			}
		})
	}
}
