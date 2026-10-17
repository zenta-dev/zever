package gogen

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// TestExplicitPublicOperationGetsEmptyGRPCPolicy proves an `auth: none`
// operation is listed in GRPCPolicies() with an empty authz.Policy, so a
// strict interceptor (which denies methods without an entry) lets exactly
// the explicitly public operations through.
func TestExplicitPublicOperationGetsEmptyGRPCPolicy(t *testing.T) {
	file := compileSchema(t, `entity Order {
	id: uuid @primary
}

service OrderService {
	rpc GetOrder(id: uuid) -> Order {
		http: GET "/orders/{id}"
		auth: none
	}
}`)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	grpc := string(out["app/grpc.go"])
	if !strings.Contains(grpc, "pb.OrderService_GetOrder_FullMethodName: {},") {
		t.Fatalf("GRPCPolicies() missing explicit public entry for GetOrder:\n%s", grpc)
	}
}
