package gogen

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/naming"
)

// pkgWorkflow is the import path of the workflow battery the generated
// saga registration file binds to (workflow.TypedSagaRegistrar / TypedStep /
// StepFuncOf).
const pkgWorkflow = "github.com/zenta-dev/zever/core/workflow"

// renderSagaFile renders "<module>/saga.go" for a module declaring at
// least one saga: per-step typed input/output structs, the SagaCaller seam
// (one method per step role), one step-constructor function per step role,
// and RegisterSagas wiring every saga into a
// workflow.TypedSagaRegistrar. Steps preserve declaration order; Compensate
// is omitted when the step declares none and Pivot only appears when set.
// The file is always regenerated; app wiring (constructing a
// workflow.TypedSagaRegistrar and a SagaCaller implementation) is left to
// the caller -- see the RegisterSagas doc comment.
//
// Every struct field maps a schema scalar to a plain standard-library Go
// type (see sagaFieldType): the generated file never holds an untyped
// payload, so no "any" and no "map[string]any" appear in it. Structs are
// local to this package on purpose: a saga step may reference an RPC of a
// service declared in another module, whose request/response aliases live in
// that module's own generated package and are not importable from here.
func renderSagaFile(pkg string, m *ir.Module) ([]byte, error) {
	var body strings.Builder

	imports := map[string]bool{importContext: true, pkgWorkflow: true}

	renderSagaStructs(&body, m, imports)
	renderSagaCaller(&body, m)
	renderSagaStepFuncs(&body, m)
	renderRegisterSagas(&body, m)

	var b strings.Builder

	b.WriteString(generatedHeader)
	fmt.Fprintf(&b, "// Package %s holds the generated saga registration for the %s module.\n", pkg, pkg)
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	writeGroupedImportBlock(&b, imports, nil)
	b.WriteString("\n")
	b.WriteString(body.String())

	formatted, err := formatSource(b.String())
	if err != nil {
		return nil, fmt.Errorf("gogen: format %s/saga.go: %w\n--- source ---\n%s", moduleLabel(m), err, b.String())
	}

	return formatted, nil
}

// sagaStepRole names one callable side of a saga step: the forward RPC or,
// when declared, its compensation.
type sagaStepRole struct {
	saga     *ir.Saga
	step     *ir.SagaStep
	rpc      *ir.ServiceRPC
	compens  bool
	sagaName string
	stepName string
}

// sagaRoles lists every callable role in m in declaration order: one per
// step for execute, plus one per step declaring a compensation.
func sagaRoles(m *ir.Module) []sagaStepRole {
	var roles []sagaStepRole

	for _, saga := range m.Sagas {
		sagaName := naming.PascalCase(saga.Name)

		for _, step := range saga.Steps {
			if step.Execute.Service == nil || step.Execute.RPC == nil {
				continue
			}

			stepName := naming.PascalCase(step.Name)
			roles = append(roles, sagaStepRole{
				saga: saga, step: step, rpc: &step.Execute,
				sagaName: sagaName, stepName: stepName,
			})

			if step.Compensate != nil {
				comp := step.Compensate
				roles = append(roles, sagaStepRole{
					saga: saga, step: step, rpc: comp, compens: true,
					sagaName: sagaName, stepName: stepName,
				})
			}
		}
	}

	return roles
}

// sagaRoleTypeNames returns the generated input/output struct names and the
// SagaCaller method name for one role.
func sagaRoleTypeNames(r sagaStepRole) (in, out, method string) {
	base := r.sagaName + r.stepName
	method = base

	if r.compens {
		base += "Compensate"
		method += "Compensate"
	}

	return base + "Input", base + "Output", method
}

// renderSagaStructs emits one input/output struct pair per saga step role,
// in declaration order. The input carries one field per RPC param; the
// output mirrors the RPC's returned entity/message fields (or a page shape
// for a paginated RPC). Field types are plain standard-library Go types, so
// values cross step boundaries through encoding/json without loss.
func renderSagaStructs(b *strings.Builder, m *ir.Module, imports map[string]bool) {
	for _, r := range sagaRoles(m) {
		in, out, _ := sagaRoleTypeNames(r)

		fmt.Fprintf(b, "// %s is the typed input for saga %q step %q.\n", in, r.saga.Name, r.step.Name)
		writeSagaStruct(b, in, sagaInputFields(r.rpc.RPC), imports)
		b.WriteString("\n")

		fmt.Fprintf(b, "// %s is the typed output for saga %q step %q.\n", out, r.saga.Name, r.step.Name)
		writeSagaOutputStruct(b, out, r.rpc.RPC, imports)
		b.WriteString("\n")
	}
}

// sagaInputFields returns the RPC's params as struct fields.
func sagaInputFields(op *ir.Operation) []*ir.Field {
	var fields []*ir.Field

	for _, p := range op.Params {
		if p.IsRef() {
			fields = append(fields, &ir.Field{Name: p.Name, Ref: p.Ref})
			continue
		}

		fields = append(fields, &ir.Field{Name: p.Name, Type: p.Type})
	}

	return fields
}

// writeSagaStruct emits one struct type with one field per schema field.
func writeSagaStruct(b *strings.Builder, name string, fields []*ir.Field, imports map[string]bool) {
	fmt.Fprintf(b, "type %s struct {\n", name)

	for _, f := range fields {
		fmt.Fprintf(b, "\t%s %s `json:\"%s\"`\n", pbFieldName(f.Name), sagaFieldType(f, imports), f.Name)
	}

	b.WriteString("}\n")
}

// writeSagaOutputStruct emits the output struct for op: a page shape (items
// plus next cursor) when op is paginated, otherwise one field per field of
// the returned entity/message. A nil return type yields an empty struct.
func writeSagaOutputStruct(b *strings.Builder, name string, op *ir.Operation, imports map[string]bool) {
	if op.Paginated && op.Returns != nil {
		item := name + "Item"
		fmt.Fprintf(b, "type %s struct {\n", name)
		fmt.Fprintf(b, "\tItems []%s `json:\"items\"`\n", item)
		b.WriteString("\tNextCursor string `json:\"next_cursor\"`\n")
		b.WriteString("}\n\n")
		fmt.Fprintf(b, "// %s is one page item of %s.\n", item, name)
		writeSagaStruct(b, item, sagaReturnFields(op.Returns), imports)

		return
	}

	writeSagaStruct(b, name, sagaReturnFields(op.Returns), imports)
}

// sagaReturnFields returns the fields of the entity/message op returns, or
// nil when op returns nothing.
func sagaReturnFields(ref *ir.TypeRef) []*ir.Field {
	if ref == nil {
		return nil
	}

	if ref.Entity != nil {
		return ref.Entity.Fields
	}

	if ref.Message != nil {
		return ref.Message.Fields
	}

	return nil
}

// sagaFieldType maps one schema field to the plain standard-library Go type
// the generated saga structs use. Named enums bind as strings (their value
// set, not a shared enum type, which would need a cross-package import);
// entity/message references bind as json.RawMessage (an opaque JSON
// payload); timestamps bind as time.Time. imports gains "time" or
// "encoding/json" when the mapping needs it. Optional scalars bind as
// pointers so absence survives the JSON round-trip between steps.
func sagaFieldType(f *ir.Field, imports map[string]bool) string {
	if f.Ref != nil {
		imports[importJSON] = true

		return "json.RawMessage"
	}

	var base string

	switch f.Type.Scalar {
	case ir.TUUID, ir.TString, ir.TDate, ir.TEnum:
		base = "string"
	case ir.TInt32:
		base = "int32"
	case ir.TInt64:
		base = "int64"
	case ir.TFloat32:
		base = "float32"
	case ir.TFloat64:
		base = "float64"
	case ir.TBool:
		base = "bool"
	case ir.TTimestamp:
		imports[importTime] = true
		base = "time.Time"
	case ir.TBytes:
		base = "[]byte"
	case ir.TJSON:
		imports[importJSON] = true
		base = "json.RawMessage"
	default:
		base = "string"
	}

	if f.Optional {
		switch base {
		case "[]byte", "json.RawMessage":
		default:
			return "*" + base
		}
	}

	return base
}

// renderSagaCaller emits the SagaCaller interface, the single seam the
// generated file needs from the host application: one method per saga step
// role, each taking that role's generated input struct and filling its
// generated output struct. Wire it to shared/grpcclient or an in-process
// dispatcher.
func renderSagaCaller(b *strings.Builder, m *ir.Module) {
	b.WriteString("// SagaCaller invokes the saga steps declared in this module; wire it to\n")
	b.WriteString("// shared/grpcclient or an in-process dispatcher. One method per step keeps\n")
	b.WriteString("// every call typed: no untyped payloads cross this seam.\n")
	b.WriteString("type SagaCaller interface {\n")

	for _, r := range sagaRoles(m) {
		in, out, method := sagaRoleTypeNames(r)
		fmt.Fprintf(b, "\t%s(ctx context.Context, in %s, out *%s) error\n", method, in, out)
	}

	b.WriteString("}\n\n")
}

// renderSagaStepFuncs emits one step-constructor function per saga step
// role, each adapting its SagaCaller method to a typed
// workflow.StepFuncOf over the role's generated structs.
func renderSagaStepFuncs(b *strings.Builder, m *ir.Module) {
	seen := map[string]bool{}

	for _, r := range sagaRoles(m) {
		in, out, method := sagaRoleTypeNames(r)
		fn := sagaStepFuncName(r)

		if seen[fn] {
			continue
		}

		seen[fn] = true

		fmt.Fprintf(b, "// %s adapts SagaCaller.%s to a typed workflow step function.\n", fn, method)
		fmt.Fprintf(b, "func %s(caller SagaCaller) workflow.StepFuncOf[%s, %s] {\n", fn, in, out)
		fmt.Fprintf(b, "\treturn func(ctx context.Context, input %s) (%s, error) {\n", in, out)
		fmt.Fprintf(b, "\t\tvar out %s\n", out)
		fmt.Fprintf(b, "\t\tif err := caller.%s(ctx, input, &out); err != nil {\n", method)
		fmt.Fprintf(b, "\t\t\treturn %s{}, err\n", out)
		b.WriteString("\t\t}\n")
		b.WriteString("\t\treturn out, nil\n")
		b.WriteString("\t}\n")
		b.WriteString("}\n\n")
	}
}

// renderRegisterSagas emits RegisterSagas, registering every saga declared
// in this module with reg in declaration order. It is deliberately not
// called from RegisterModule: the host application owns saga wiring (it
// must construct a workflow.TypedSagaRegistrar and a SagaCaller first), so
// this file documents the seam rather than forcing it into the service-only
// registration path.
func renderRegisterSagas(b *strings.Builder, m *ir.Module) {
	b.WriteString("// RegisterSagas registers every saga declared in this module.\n")
	b.WriteString("func RegisterSagas(reg workflow.TypedSagaRegistrar, caller SagaCaller) {\n")

	for _, saga := range m.Sagas {
		sagaName := naming.PascalCase(saga.Name)
		fmt.Fprintf(b, "\treg.RegisterTypedSaga(%q, []workflow.TypedStep{\n", saga.Name)

		for _, step := range saga.Steps {
			if step.Execute.Service == nil || step.Execute.RPC == nil {
				continue
			}

			stepName := naming.PascalCase(step.Name)
			execFn := sagaStepFuncName(sagaStepRole{sagaName: sagaName, stepName: stepName})

			if step.Compensate != nil {
				compFn := sagaStepFuncName(sagaStepRole{sagaName: sagaName, stepName: stepName, compens: true})
				fmt.Fprintf(b, "\t\tworkflow.NewTypedStep(%q, %s(caller), %s(caller), %t),\n",
					step.Name, execFn, compFn, step.Pivot)
				continue
			}

			fmt.Fprintf(b, "\t\tworkflow.NewTypedStepWithoutCompensate(%q, %s(caller), %t),\n",
				step.Name, execFn, step.Pivot)
		}

		b.WriteString("\t})\n")
	}

	b.WriteString("}\n")
}

// sagaStepFuncName returns the unexported constructor name for one role,
// e.g. "checkoutSagaReserveExecute" or "checkoutSagaReserveCompensate".
func sagaStepFuncName(r sagaStepRole) string {
	name := lowerFirst(r.sagaName) + r.stepName

	if r.compens {
		return name + "Compensate"
	}

	return name + "Execute"
}

// lowerFirst lowercases the first byte of s for unexported identifiers.
// Saga and step names are resolver-validated ASCII identifiers, so a byte
// operation is enough.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}

	c := s[0]
	if c >= 'A' && c <= 'Z' {
		c += 'a' - 'A'
	}

	return string(c) + s[1:]
}
