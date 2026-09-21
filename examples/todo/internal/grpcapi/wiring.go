package grpcapi

import (
	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/authz"
	genapp "github.com/zenta-dev/zever/examples/todo/generated/gogen/app"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/permission/rbac"
)

// Wiring bundles one Service with the checker, policies, and interceptor
// that enforce the generated authz contract on both transports.
type Wiring struct {
	// Service is the shared business logic behind both transports.
	Service *Service
	// Checker is the rbac permission checker both transports call.
	Checker permission.Checker
	// Policies is genapp.GRPCPolicies(): the compile-time policy map the
	// HTTP routes enforce via RegisterGrpcTaskServiceRoutes and the gRPC
	// server enforces via Interceptor.
	Policies map[string]authz.Policy
	// Interceptor enforces Policies on every gRPC method.
	Interceptor grpc.UnaryServerInterceptor
}

// Build wires a shared Service with auth a and an rbac checker, returning
// the interceptor built from the SAME generated GRPCPolicies the HTTP
// routes enforce. The checker carries no allow rule for
// "grpc_task.delete", so DeleteTask is denied on both transports while
// Create/Get (auth-only policies) succeed for any verified caller.
func Build(a auth.Auth) (*Wiring, error) {
	checker, err := rbac.New(permission.Options{})
	if err != nil {
		return nil, err
	}

	svc := New()
	policies := genapp.GRPCPolicies()

	return &Wiring{
		Service:     svc,
		Checker:     checker,
		Policies:    policies,
		Interceptor: authz.UnaryServerInterceptor(a, checker, policies),
	}, nil
}
