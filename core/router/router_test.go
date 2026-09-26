package router

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

var routerTestSeq int32 = 2000

func freshRouterAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", atomic.AddInt32(&routerTestSeq, 1)))
}

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

func TestRegister_nil_factory_fails(t *testing.T) {
	t.Parallel()
	a := freshRouterAdapter()
	err := Register(a, nil)
	if err == nil {
		t.Fatal("Register nil factory expected error, got nil")
	}
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("err = %v, want ErrNilFactory", err)
	}
	if !strings.Contains(err.Error(), a.String()) {
		t.Fatalf("err = %q, want adapter name", err.Error())
	}
}

func TestRegister_duplicate_fails(t *testing.T) {
	t.Parallel()
	a := freshRouterAdapter()
	factory := func(Options) (Router, error) { return &mockRouter{}, nil }
	if err := Register(a, factory); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	err := Register(a, factory)
	if err == nil {
		t.Fatal("duplicate Register expected error, got nil")
	}
	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("err = %v, want ErrDuplicateAdapter", err)
	}
	var de *DuplicateAdapterError
	if !errors.As(err, &de) {
		t.Fatalf("err type = %T, want *DuplicateAdapterError", err)
	}
	if de.Adapter != a {
		t.Fatalf("Adapter = %v, want %v", de.Adapter, a)
	}
}

func TestRegister_success(t *testing.T) {
	t.Parallel()
	a := freshRouterAdapter()
	if err := Register(a, func(Options) (Router, error) { return &mockRouter{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	r, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if r == nil {
		t.Fatal("Open returned nil")
	}
}

func TestOpen_unknown_adapter_fails(t *testing.T) {
	t.Parallel()
	a := freshRouterAdapter()
	got, err := Open(a, Options{})
	if err == nil {
		t.Fatal("Open unknown expected error, got nil")
	}
	if got != nil {
		t.Fatalf("Open unknown got = %v, want nil", got)
	}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err type = %T, want *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("Adapter = %v, want %v", ue.Adapter, a)
	}
}

func TestOpen_validate_before_lookup_fail_closed(t *testing.T) {
	t.Parallel()
	a := freshRouterAdapter()
	bad := Options{AppName: strings.Repeat("x", 65)}
	_, err := Open(a, bad)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions (fail-closed)", err)
	}
	if errors.Is(err, ErrUnknownAdapter) {
		t.Fatal("should not be ErrUnknownAdapter when opts invalid")
	}
}

func TestOpen_factory_error_wrapped(t *testing.T) {
	t.Parallel()
	a := freshRouterAdapter()
	sentinel := errors.New("boom")
	factory := func(Options) (Router, error) { return nil, sentinel }
	if err := Register(a, factory); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != nil {
		t.Fatalf("got = %v, want nil on factory error", got)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapped sentinel", err)
	}
	if !strings.HasPrefix(err.Error(), "router: open ") {
		t.Fatalf("err = %q, want %q prefix", err.Error(), "router: open ")
	}
}

func TestOpen_registered_success(t *testing.T) {
	t.Parallel()
	a := freshRouterAdapter()
	want := &mockRouter{}
	if err := Register(a, func(Options) (Router, error) { return want, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{AppName: "shop"})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if got == nil {
		t.Fatal("Open returned nil")
	}
	mock, ok := got.(*mockRouter)
	if !ok {
		t.Fatalf("expected *mockRouter, got %T", got)
	}
	var _ Router = mock
	var _ Group = mock
	got.Handle("GET", "/users/:id", func(http.ResponseWriter, *http.Request) {})
	got.Use(func(next http.Handler) http.Handler { return next })
	g := got.Group("/api", func(next http.Handler) http.Handler { return next })
	if g == nil {
		t.Fatal("Group returned nil")
	}
	g.Handle("POST", "/items", func(http.ResponseWriter, *http.Request) {})
	g.Use(func(next http.Handler) http.Handler { return next })
	if gg := g.Group("/v1"); gg == nil {
		t.Fatal("nested Group returned nil")
	}
}

func TestOpen_concurrent_Register_Open(t *testing.T) {
	t.Parallel()
	adapters := make([]Adapter, 8)
	for i := range adapters {
		adapters[i] = freshRouterAdapter()
	}
	var wg sync.WaitGroup
	for _, a := range adapters {
		wg.Add(1)
		go func(a Adapter) {
			defer wg.Done()
			_ = Register(a, func(Options) (Router, error) { return &mockRouter{}, nil })
			_, _ = Open(a, Options{})
		}(a)
	}
	wg.Wait()
	for _, a := range adapters {
		a := a
		r, err := Open(a, Options{})
		if err != nil {
			t.Fatalf("Open adapter %v err = %v", a, err)
		}
		if r == nil {
			t.Fatalf("Open adapter %v got nil", a)
		}
	}
}
