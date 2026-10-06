package resolver

import (
	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// resolveSagas is the saga pass's entry point: it resolves every saga decl
// (in Pass 0's source-declaration order) and appends each resolved
// *ir.Saga to its owning Module's Sagas slice, same reasoning as the other
// passes' per-module appends. It runs after services are resolved because
// every step reference resolves against the resolved service index.
func resolveSagas(
	decls []*ast.SagaDecl, declModule map[ast.Decl]*ir.Module, serviceByName map[string]*ir.Service,
) diag.List {
	var diags diag.List

	for _, decl := range decls {
		module := declModule[decl]
		if module == nil {
			// Unreachable given Pass -1/0's invariants; guarded defensively.
			continue
		}

		saga, d := resolveSagaSchema(decl, module, serviceByName)
		diags = append(diags, d...)

		module.Sagas = append(module.Sagas, saga)
	}

	return diags
}

// resolveSagaSchema resolves one saga decl: each step's execute/compensate
// RPC references, step-name uniqueness, and the at-most-one-pivot rule.
// Cross-module references are allowed by design -- composing RPCs of
// services declared in other modules is the point of a saga -- so no
// boundary check is applied here.
func resolveSagaSchema(decl *ast.SagaDecl, module *ir.Module, serviceByName map[string]*ir.Service) (*ir.Saga, diag.List) {
	var diags diag.List

	saga := &ir.Saga{Name: decl.Name, Module: module, Pos: decl.Pos, DocComment: decl.DocComment}

	seen := make(map[string]bool, len(decl.Steps))

	pivots := 0

	for _, sd := range decl.Steps {
		if seen[sd.Name] {
			diags = append(diags, diag.Wrap("resolve", sd.Pos, ErrInvalidOption,
				"saga %s has duplicate step name %q; step names must be unique within a saga", decl.Name, sd.Name))

			continue
		}

		seen[sd.Name] = true

		if sd.Pivot {
			pivots++
		}

		step, d := resolveSagaStep(sd, decl.Name, serviceByName)
		diags = append(diags, d...)

		saga.Steps = append(saga.Steps, step)
	}

	if pivots > 1 {
		diags = append(diags, diag.Wrap("resolve", decl.Pos, ErrInvalidOption,
			"saga %s has %d pivot steps; at most one step may set pivot: true", decl.Name, pivots))
	}

	return saga, diags
}

// resolveSagaStep resolves one step's execute (required) and compensate
// (optional) RPC references.
func resolveSagaStep(
	sd *ast.SagaStepDecl, sagaName string, serviceByName map[string]*ir.Service,
) (*ir.SagaStep, diag.List) {
	var diags diag.List

	if sd.Execute == nil {
		diags = append(diags, diag.Wrap("resolve", sd.Pos, ErrInvalidOption,
			"saga %s step %q is missing its required execute option", sagaName, sd.Name))
	}

	step := &ir.SagaStep{Name: sd.Name, Pivot: sd.Pivot, Pos: sd.Pos}

	if sd.Execute != nil {
		ref, d := resolveRPCRef(sd.Execute, serviceByName)
		diags = append(diags, d...)

		if ref != nil {
			step.Execute = *ref
		}
	}

	if sd.Compensate != nil {
		ref, d := resolveRPCRef(sd.Compensate, serviceByName)
		diags = append(diags, d...)

		step.Compensate = ref
	}

	return step, diags
}

// resolveRPCRef resolves one Service.RPC reference against the resolved
// service index. Service names are global across the whole compile (see
// resolveSymbols), so a reference to another module's service resolves
// exactly like a same-module one.
func resolveRPCRef(ref *ast.RPCRef, serviceByName map[string]*ir.Service) (*ir.ServiceRPC, diag.List) {
	svc, ok := serviceByName[ref.Service]
	if !ok {
		return nil, diag.List{diag.Wrap("resolve", ref.Pos, ErrUnresolvedReference,
			"saga references unknown service %q", ref.Service)}
	}

	for _, op := range svc.Operations {
		if op.Name == ref.RPC {
			return &ir.ServiceRPC{Service: svc, RPC: op, Pos: ref.Pos}, nil
		}
	}

	return nil, diag.List{diag.Wrap("resolve", ref.Pos, ErrUnresolvedReference,
		"service %s has no rpc %q", ref.Service, ref.RPC)}
}

// buildServiceIndex indexes every resolved service by name. Service names
// are globally unique (resolveSymbols rejects duplicates across all six
// declaration kinds), so a flat name -> service map is unambiguous.
func buildServiceIndex(modules []*ir.Module) map[string]*ir.Service {
	out := make(map[string]*ir.Service)

	for _, m := range modules {
		for _, svc := range m.Services {
			out[svc.Name] = svc
		}
	}

	return out
}
