package grpcclient

import (
	"net/url"
	"strings"
	"testing"

	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"
)

// stubResolverClientConn records UpdateState calls for resolver tests.
type stubResolverClientConn struct {
	state resolver.State
}

// UpdateState records the reported state.
func (s *stubResolverClientConn) UpdateState(state resolver.State) error {
	s.state = state
	return nil
}

// ReportError is a no-op.
func (s *stubResolverClientConn) ReportError(error) {}

// NewAddress is a no-op.
func (s *stubResolverClientConn) NewAddress([]resolver.Address) {}

// ParseServiceConfig returns an empty result.
func (s *stubResolverClientConn) ParseServiceConfig(string) *serviceconfig.ParseResult {
	return &serviceconfig.ParseResult{}
}

func TestStaticResolverBuilderScheme(t *testing.T) {
	t.Parallel()

	b := &staticResolverBuilder{addrs: map[string][]string{"users": {"127.0.0.1:50051"}}}
	if got := b.Scheme(); got != "static" {
		t.Fatalf("Scheme() = %q, want %q", got, "static")
	}
}

func TestStaticResolverBuilderResolvesConfigured(t *testing.T) {
	t.Parallel()

	b := &staticResolverBuilder{addrs: map[string][]string{
		"users": {"10.0.0.1:50051", "10.0.0.2:50051"},
	}}
	target := resolver.Target{URL: url.URL{Scheme: "static", Path: "/users"}}
	cc := &stubResolverClientConn{}

	r, err := b.Build(target, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r == nil {
		t.Fatal("Build returned nil resolver")
	}

	want := []string{"10.0.0.1:50051", "10.0.0.2:50051"}
	if len(cc.state.Addresses) != len(want) {
		t.Fatalf("got %d addresses, want %d", len(cc.state.Addresses), len(want))
	}
	for i, a := range cc.state.Addresses {
		if a.Addr != want[i] {
			t.Errorf("address[%d] = %q, want %q", i, a.Addr, want[i])
		}
	}
}

func TestStaticResolverBuilderUnknownName(t *testing.T) {
	t.Parallel()

	b := &staticResolverBuilder{addrs: map[string][]string{"users": {"127.0.0.1:50051"}}}
	target := resolver.Target{URL: url.URL{Scheme: "static", Path: "/missing"}}
	cc := &stubResolverClientConn{}

	_, err := b.Build(target, cc, resolver.BuildOptions{})
	if err == nil {
		t.Fatal("Build succeeded for unknown name, want error")
	}
	if !strings.Contains(err.Error(), `unknown name "missing"`) {
		t.Fatalf("error = %q, want unknown name mention", err.Error())
	}
}

func TestStaticResolverBuilderEmptyAddressList(t *testing.T) {
	t.Parallel()

	b := &staticResolverBuilder{addrs: map[string][]string{"users": {}}}
	target := resolver.Target{URL: url.URL{Scheme: "static", Path: "/users"}}
	cc := &stubResolverClientConn{}

	if _, err := b.Build(target, cc, resolver.BuildOptions{}); err == nil {
		t.Fatal("Build succeeded for empty address list, want error")
	}
}

func TestStaticResolverNoOp(t *testing.T) {
	t.Parallel()

	r := &staticResolver{}
	r.ResolveNow(resolver.ResolveNowOptions{})
	r.Close()
}
