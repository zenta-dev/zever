package grpcclient

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

func BenchmarkUnaryInterceptorMetadata(b *testing.B) {
	cfg := &config{metadata: map[string]string{"tenant": "acme", "version": "v2"}}
	interceptor := cfg.unaryInterceptor()
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
		if err := interceptor(b.Context(), "/svc/Method", nil, nil, nil, invoker); err != nil {
			b.Fatal(err)
		}
	}
}
