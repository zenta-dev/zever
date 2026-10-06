package resolver

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
)

func sagaRef(service, rpc string) *ast.RPCRef {
	return &ast.RPCRef{Service: service, RPC: rpc}
}

func sagaStepDecl(name string, execute, compensate *ast.RPCRef, pivot bool) *ast.SagaStepDecl {
	return &ast.SagaStepDecl{Name: name, Execute: execute, Compensate: compensate, Pivot: pivot}
}

func findSaga(t *testing.T, schema *ir.Schema, name string) *ir.Saga {
	t.Helper()

	for _, m := range schema.Modules {
		for _, s := range m.Sagas {
			if s.Name == name {
				return s
			}
		}
	}

	t.Fatalf("saga %q not found in resolved schema", name)

	return nil
}

func sagaFixture() []ast.Decl {
	item := entityWithIDField("Item")
	inventory := serviceDecl("InventoryService",
		rpcDecl("Reserve", "Item", nil),
		rpcDecl("Release", "Item", nil))
	payment := serviceDecl("PaymentService",
		rpcDecl("Charge", "Item", nil),
		rpcDecl("Refund", "Item", nil))
	saga := &ast.SagaDecl{Name: "CheckoutSaga", Steps: []*ast.SagaStepDecl{
		sagaStepDecl("reserve", sagaRef("InventoryService", "Reserve"), sagaRef("InventoryService", "Release"), false),
		sagaStepDecl("charge", sagaRef("PaymentService", "Charge"), nil, true),
	}}

	return []ast.Decl{item, inventory, payment, saga}
}

func TestResolveSagaValid(t *testing.T) {
	t.Parallel()

	schema, diags := Resolve([]*ast.File{{Name: "a.zen", Decls: sagaFixture()}})
	mustNotHaveErrors(t, diags)

	saga := findSaga(t, schema, "CheckoutSaga")
	if len(saga.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(saga.Steps))
	}

	reserve := saga.Steps[0]
	if reserve.Execute.Service == nil || reserve.Execute.Service.Name != "InventoryService" {
		t.Fatalf("step[0].Execute.Service = %+v, want InventoryService", reserve.Execute.Service)
	}

	if reserve.Execute.RPC == nil || reserve.Execute.RPC.Name != "Reserve" {
		t.Fatalf("step[0].Execute.RPC = %+v, want Reserve", reserve.Execute.RPC)
	}

	if reserve.Compensate == nil || reserve.Compensate.RPC == nil || reserve.Compensate.RPC.Name != "Release" {
		t.Fatalf("step[0].Compensate = %+v, want Release", reserve.Compensate)
	}

	if reserve.Pivot {
		t.Fatalf("step[0].Pivot = true, want false")
	}

	charge := saga.Steps[1]
	if !charge.Pivot {
		t.Fatalf("step[1].Pivot = false, want true")
	}

	if charge.Compensate != nil {
		t.Fatalf("step[1].Compensate = %+v, want nil", charge.Compensate)
	}
}

func TestResolveSagaMissingExecute(t *testing.T) {
	t.Parallel()

	saga := &ast.SagaDecl{Name: "S", Steps: []*ast.SagaStepDecl{{Name: "x"}}}
	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{saga}}}

	_, diags := Resolve(files)
	mustHaveErr(t, diags, ErrInvalidOption)
}

func TestResolveSagaUnknownService(t *testing.T) {
	t.Parallel()

	saga := &ast.SagaDecl{Name: "S", Steps: []*ast.SagaStepDecl{
		sagaStepDecl("x", sagaRef("Nope", "Op"), nil, false),
	}}
	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{saga}}}

	_, diags := Resolve(files)
	mustHaveErr(t, diags, ErrUnresolvedReference)
}

func TestResolveSagaUnknownRPC(t *testing.T) {
	t.Parallel()

	item := entityWithIDField("Item")
	svc := serviceDecl("InventoryService", rpcDecl("Reserve", "Item", nil))
	saga := &ast.SagaDecl{Name: "S", Steps: []*ast.SagaStepDecl{
		sagaStepDecl("x", sagaRef("InventoryService", "Nonexistent"), nil, false),
	}}
	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{item, svc, saga}}}

	_, diags := Resolve(files)
	mustHaveErr(t, diags, ErrUnresolvedReference)
}

func TestResolveSagaDuplicateStep(t *testing.T) {
	t.Parallel()

	item := entityWithIDField("Item")
	svc := serviceDecl("InventoryService", rpcDecl("Reserve", "Item", nil))
	saga := &ast.SagaDecl{Name: "S", Steps: []*ast.SagaStepDecl{
		sagaStepDecl("x", sagaRef("InventoryService", "Reserve"), nil, false),
		sagaStepDecl("x", sagaRef("InventoryService", "Reserve"), nil, false),
	}}
	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{item, svc, saga}}}

	_, diags := Resolve(files)
	mustHaveErr(t, diags, ErrInvalidOption)
}

func TestResolveSagaTwoPivots(t *testing.T) {
	t.Parallel()

	item := entityWithIDField("Item")
	svc := serviceDecl("InventoryService", rpcDecl("Reserve", "Item", nil))
	saga := &ast.SagaDecl{Name: "S", Steps: []*ast.SagaStepDecl{
		sagaStepDecl("a", sagaRef("InventoryService", "Reserve"), nil, true),
		sagaStepDecl("b", sagaRef("InventoryService", "Reserve"), nil, true),
	}}
	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{item, svc, saga}}}

	_, diags := Resolve(files)
	mustHaveErr(t, diags, ErrInvalidOption)
}

func TestResolveSagaCrossModule(t *testing.T) {
	t.Parallel()

	item := entityWithIDField("Item")
	inventory := serviceDecl("InventoryService", rpcDecl("Reserve", "Item", nil))
	saga := &ast.SagaDecl{Name: "CheckoutSaga", Steps: []*ast.SagaStepDecl{
		sagaStepDecl("reserve", sagaRef("InventoryService", "Reserve"), nil, false),
		sagaStepDecl("charge", sagaRef("PaymentService", "Charge"), nil, true),
	}}

	payment := entityWithIDField("Payment")
	payments := serviceDecl("PaymentService", rpcDecl("Charge", "Payment", nil))

	files := []*ast.File{
		{Name: "schema/a/a.zen", Decls: []ast.Decl{item, inventory, saga}},
		{Name: "schema/b/b.zen", Decls: []ast.Decl{payment, payments}},
	}

	schema, diags := ResolveWithSchemaDir(files, "schema")
	mustNotHaveErrors(t, diags)

	got := findSaga(t, schema, "CheckoutSaga")
	if got.Module == nil || got.Module.Name != "a" {
		t.Fatalf("saga module = %+v, want a", got.Module)
	}

	charge := got.Steps[1]
	if charge.Execute.Service == nil || charge.Execute.Service.Name != "PaymentService" {
		t.Fatalf("cross-module step service = %+v, want PaymentService", charge.Execute.Service)
	}

	if charge.Execute.Service.Module == nil || charge.Execute.Service.Module.Name != "b" {
		t.Fatalf("cross-module step service module = %+v, want b", charge.Execute.Service.Module)
	}
}

func TestResolveSagaDuplicateDeclName(t *testing.T) {
	t.Parallel()

	item := entityWithIDField("Item")
	svc := serviceDecl("InventoryService", rpcDecl("Reserve", "Item", nil))
	// A saga sharing a name with an entity collides in the flat namespace.
	saga := &ast.SagaDecl{Name: "Item", Steps: []*ast.SagaStepDecl{
		sagaStepDecl("x", sagaRef("InventoryService", "Reserve"), nil, false),
	}}
	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{item, svc, saga}}}

	_, diags := Resolve(files)
	mustHaveErr(t, diags, ErrDuplicateDecl)
}
