package serve

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/router"
)

// stubGRPCModule is a Module declaring one gRPC policy.
type stubGRPCModule struct {
	stubModule
	policies map[string]authz.Policy
}

// GRPCPolicies returns the module's full-method policy map.
func (m *stubGRPCModule) GRPCPolicies() map[string]authz.Policy {
	return m.policies
}

// testGRPCServer is the interface the stub test service implements.
type testGRPCServer interface {
	Nope(context.Context, *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error)
}

// testGRPCImpl serves the stub test service: reachable only through the
// interceptor chain, proving allow-path behavior.
type testGRPCImpl struct{}

func (testGRPCImpl) Nope(context.Context, *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

// testMethodHandler decodes a HealthCheckRequest and runs the interceptor
// chain for method, ending in the stub implementation.
func testMethodHandler(method string) func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		req := &grpc_health_v1.HealthCheckRequest{}
		if err := dec(req); err != nil {
			return nil, err
		}

		if interceptor == nil {
			impl, ok := srv.(testGRPCServer)
			if !ok {
				return nil, status.Error(codes.Internal, "serve: test service has no implementation")
			}

			return impl.Nope(ctx, req)
		}

		info := &grpc.UnaryServerInfo{Server: srv, FullMethod: method}

		return interceptor(ctx, req, info, func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*grpc_health_v1.HealthCheckRequest)
			if !ok {
				return nil, status.Error(codes.Internal, "serve: test service got wrong request type")
			}

			impl, ok := srv.(testGRPCServer)
			if !ok {
				return nil, status.Error(codes.Internal, "serve: test service has no implementation")
			}

			return impl.Nope(ctx, typed)
		})
	}
}

// testServiceDesc registers Yep (policy-allowlisted in the test) and Nope
// (no policy: denied fail-closed).
var testServiceDesc = grpc.ServiceDesc{
	ServiceName: "test.v1.Test",
	HandlerType: (*testGRPCServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Yep", Handler: testMethodHandler("/test.v1.Test/Yep")},
		{MethodName: "Nope", Handler: testMethodHandler("/test.v1.Test/Nope")},
	},
	Streams: []grpc.StreamDesc{},
}

// grpcInvoke calls one test-service method, returning the status code.
func grpcInvoke(t *testing.T, addr, method string) codes.Code {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	req := &grpc_health_v1.HealthCheckRequest{}
	reply := &grpc_health_v1.HealthCheckResponse{}

	err = conn.Invoke(ctx, method, req, reply)

	return status.Code(err)
}

// grpcHealthCheck dials addr and returns the health serving status.
func grpcHealthCheck(t *testing.T, addr string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Health.Check: %v", err)
	}

	return resp.Status
}

// TestGRPCServesHealthAndDeniesByDefault proves the opt-in gRPC listener
// serves the health check plus the policy-allowlisted method, and denies
// the unlisted method fail-closed.
func TestGRPCServesHealthAndDeniesByDefault(t *testing.T) {
	mod := &stubGRPCModule{
		stubModule: stubModule{name: "app", mountFn: func(_ router.Router, c *container.Container) error {
			gs, err := c.GRPC()
			if err != nil {
				return err
			}
			gs.RegisterService(&testServiceDesc, testGRPCImpl{})

			return nil
		}},
		policies: map[string]authz.Policy{"/test.v1.Test/Yep": {}},
	}

	heard := make(chan string, 2)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)

	go func() {
		errCh <- runServe(ctx, mustFullProviders(t, nil), func(*container.Container) []Module {
			return []Module{mod}
		},
			WithPort("0"),
			WithGRPCAddr("127.0.0.1:0"),
			WithEnvOverride(config.Env{}),
			withListenHook(func(_, addr string) { heard <- addr }),
		)
	}()

	// The HTTP listener binds first, the gRPC listener second.
	_ = waitBoundAddr(t, heard, errCh)
	grpcAddr := dialAddr(t, waitBoundAddr(t, heard, errCh))

	if got := grpcHealthCheck(t, grpcAddr); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("Health.Check = %v, want SERVING", got)
	}

	if got := grpcInvoke(t, grpcAddr, "/test.v1.Test/Yep"); got != codes.OK {
		t.Fatalf("Yep code = %v, want OK", got)
	}

	if got := grpcInvoke(t, grpcAddr, "/test.v1.Test/Nope"); got != codes.PermissionDenied {
		t.Fatalf("Nope code = %v, want PermissionDenied", got)
	}

	cancel()

	timeout, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runServe: %v", err)
		}
	case <-timeout.Done():
		t.Fatal("timed out waiting for shutdown")
	}
}
