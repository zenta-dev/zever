package gogen

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/dsl/ir"
)

// pkgWorkflow is the import path of the workflow battery the generated
// saga registration file binds to (workflow.SagaRegistrar / SagaStep /
// StepFunc).
const pkgWorkflow = "github.com/zenta-dev/zever/core/workflow"

// renderSagaFile renders "<module>/saga.go" for a module declaring at
// least one saga: the SagaCaller seam, one RegisterSaga call per saga (in
// declaration order), and the shared sagaStep helper. Steps preserve
// declaration order; Compensate is omitted when the step declares none and
// Pivot only appears when set. The file is always regenerated; app wiring
// (constructing a workflow.SagaRegistrar and a SagaCaller implementation)
// is left to the caller -- see the RegisterSagas doc comment.
func renderSagaFile(pkg string, m *ir.Module) ([]byte, error) {
	var body strings.Builder

	renderSagaCaller(&body)
	renderRegisterSagas(&body, m)
	renderSagaStepHelper(&body)

	var b strings.Builder

	b.WriteString(generatedHeader)
	fmt.Fprintf(&b, "// Package %s holds the generated saga registration for the %s module.\n", pkg, pkg)
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n\n")
	fmt.Fprintf(&b, "\t%q\n", pkgWorkflow)
	b.WriteString(")\n\n")
	b.WriteString(body.String())

	formatted, err := formatSource(b.String())
	if err != nil {
		return nil, fmt.Errorf("gogen: format %s/saga.go: %w\n--- source ---\n%s", moduleLabel(m), err, b.String())
	}

	return formatted, nil
}

// renderSagaCaller emits the SagaCaller interface, the single seam the
// generated file needs from the host application: one method that invokes
// an RPC on a service by name. Wire it to shared/grpcclient or an
// in-process dispatcher; in/out are JSON-marshalable.
func renderSagaCaller(b *strings.Builder) {
	b.WriteString("// SagaCaller invokes an RPC on a service by name; wire it to shared/grpcclient\n")
	b.WriteString("// or an in-process dispatcher. in/out are JSON-marshalable.\n")
	b.WriteString("type SagaCaller interface {\n")
	b.WriteString("\tCall(ctx context.Context, service, method string, in any, out any) error\n")
	b.WriteString("}\n\n")
}

// renderRegisterSagas emits RegisterSagas, registering every saga declared
// in this module with reg in declaration order. It is deliberately not
// called from RegisterModule: the host application owns saga wiring (it
// must construct a workflow.SagaRegistrar and a SagaCaller first), so this
// file documents the seam rather than forcing it into the service-only
// registration path.
func renderRegisterSagas(b *strings.Builder, m *ir.Module) {
	b.WriteString("// RegisterSagas registers every saga declared in this module.\n")
	b.WriteString("func RegisterSagas(reg workflow.SagaRegistrar, caller SagaCaller) {\n")

	for _, saga := range m.Sagas {
		fmt.Fprintf(b, "\treg.RegisterSaga(%q, []workflow.SagaStep{\n", saga.Name)

		for _, step := range saga.Steps {
			if step.Execute.Service == nil || step.Execute.RPC == nil {
				continue
			}

			fmt.Fprintf(b, "\t\t{Name: %q", step.Name)

			if step.Pivot {
				b.WriteString(", Pivot: true")
			}

			fmt.Fprintf(b, ", Execute: sagaStep(%q, %q, caller)",
				step.Execute.Service.Name, step.Execute.RPC.Name)

			if step.Compensate != nil {
				fmt.Fprintf(b, ", Compensate: sagaStep(%q, %q, caller)",
					step.Compensate.Service.Name, step.Compensate.RPC.Name)
			}

			b.WriteString("},\n")
		}

		b.WriteString("\t})\n")
	}

	b.WriteString("}\n\n")
}

// renderSagaStepHelper emits sagaStep, the StepFunc adapter that calls one
// RPC through the SagaCaller. Emitted once per file no matter how many
// sagas or steps reference it.
func renderSagaStepHelper(b *strings.Builder) {
	b.WriteString("// sagaStep adapts one Service.RPC reference to a workflow.StepFunc:\n")
	b.WriteString("// it invokes the RPC through caller, decoding the JSON-marshalable\n")
	b.WriteString("// response into a map for the next step's input.\n")
	b.WriteString("func sagaStep(service, method string, caller SagaCaller) workflow.StepFunc {\n")
	b.WriteString("\treturn func(ctx context.Context, input any) (any, error) {\n")
	b.WriteString("\t\tvar out map[string]any\n")
	b.WriteString("\t\tif err := caller.Call(ctx, service, method, input, &out); err != nil {\n")
	b.WriteString("\t\t\treturn nil, err\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\treturn out, nil\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
}
