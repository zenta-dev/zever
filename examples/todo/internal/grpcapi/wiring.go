package grpcapi

import (
	"context"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/permission"
	genapp "github.com/zenta-dev/zever/examples/todo/generated/gogen/app"
)

// deleteAllowChecker is a permission.Checker for the todo grpc proof: it
// allows any authenticated subject past the generated
// "grpc_task.delete" policy gate. Ownership (owner_field user_id) is then
// enforced by Service.DeleteTask itself, the one place where the request
// id is available on both transports — the gRPC interceptor carries no
// resource id, so a store lookup cannot live in the checker and still
// cover both sides.
type deleteAllowChecker struct{}

// Can reports whether subject may perform action on resource.
func (deleteAllowChecker) Can(_ context.Context, subject permission.Subject, action string, _ permission.Resource) (permission.Decision, error) {
	if action == "grpc_task.delete" && subject.ID != "" {
		return permission.Decision{Allowed: true, Reason: "allow"}, nil
	}
	return permission.Decision{Allowed: false, Reason: "implicit_deny"}, nil
}

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

// Build wires a shared Service with auth a and an allow-authenticated
// checker, returning the interceptor built from the SAME generated
// GRPCPolicies the HTTP routes enforce. The checker lets any authenticated
// caller past the "grpc_task.delete" gate; Service.DeleteTask then enforces
// the owner_field (user_id) comparison, so owner deletes succeed while
// non-owner deletes fail with PermissionDenied on both transports, and
// unauthenticated callers still fail with Unauthenticated on both.
func Build(a auth.Auth) (*Wiring, error) {
	checker := deleteAllowChecker{}

	svc := New()
	policies := genapp.GRPCPolicies()

	return &Wiring{
		Service:     svc,
		Checker:     checker,
		Policies:    policies,
		Interceptor: authz.UnaryServerInterceptor(a, checker, policies),
	}, nil
}
