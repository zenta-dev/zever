package grpcclient

import (
	"context"
	"net/url"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"
)

func BenchmarkUnaryInterceptorMetadata(b *testing.B) {
	cfg := &config{metadata: map[string]string{"tenant": "acme", "version": "v2"}}
	interceptor := cfg.unaryInterceptor()
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := interceptor(b.Context(), "/svc/Method", nil, nil, nil, invoker); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnaryInterceptorNoOptions(b *testing.B) {
	cfg := &config{}
	interceptor := cfg.unaryInterceptor()
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := interceptor(b.Context(), "/svc/Method", nil, nil, nil, invoker); err != nil {
			b.Fatal(err)
		}
	}
}

// stubResolverConn is a test double resolver.ClientConn recording updates.
type stubResolverConn struct {
	states int
}

// UpdateState records the update.
func (s *stubResolverConn) UpdateState(resolver.State) error {
	s.states++
	return nil
}

// ReportError is a no-op.
func (s *stubResolverConn) ReportError(error) {}

// NewAddress is a no-op.
func (s *stubResolverConn) NewAddress([]resolver.Address) {}

// NewServiceConfig is a no-op.
func (s *stubResolverConn) NewServiceConfig(string) {}

// ParseServiceConfig is a no-op.
func (s *stubResolverConn) ParseServiceConfig(string) *serviceconfig.ParseResult { return nil }

// BenchmarkResolverBuild measures resolving a static target name against the
// address map.
func BenchmarkResolverBuild(b *testing.B) {
	builder := &staticResolverBuilder{addrs: map[string][]string{
		"users": {"127.0.0.1:50051", "127.0.0.2:50051"},
	}}
	target := resolver.Target{URL: url.URL{Scheme: "static", Path: "/users"}}
	cc := &stubResolverConn{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := builder.Build(target, cc, resolver.BuildOptions{}); err != nil {
			b.Fatalf("Build() error = %v", err)
		}
	}
}
