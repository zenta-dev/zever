package gogen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	dslparser "github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
)

const sagaFixture = `entity Item {
	id: uuid @primary
}

service InventoryService {
	rpc Reserve(item_id: uuid) -> Item {
		auth: required
	}

	rpc Release(item_id: uuid) -> Item {
		auth: required
	}
}

service PaymentService {
	rpc Charge(amount: int64) -> Item {
		auth: required
	}

	rpc Refund(amount: int64) -> Item {
		auth: required
	}
}

saga CheckoutSaga {
	step reserve {
		execute: InventoryService.Reserve
		compensate: InventoryService.Release
	}
	step charge {
		execute: PaymentService.Charge
		compensate: PaymentService.Refund
		pivot: true
	}
}`

func TestGenerateSagaFile(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, sagaFixture)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got, ok := out["app/saga.go"]
	if !ok {
		t.Fatalf("Generate missing app/saga.go; keys = %v", keysOf(out))
	}

	src := string(got)

	for _, want := range []string{
		"func RegisterSagas(reg workflow.SagaRegistrar, caller SagaCaller)",
		`reg.RegisterSaga("CheckoutSaga", []workflow.SagaStep{`,
		`{Name: "reserve", Execute: sagaStep("InventoryService", "Reserve", caller), Compensate: sagaStep("InventoryService", "Release", caller)}`,
		`{Name: "charge", Pivot: true, Execute: sagaStep("PaymentService", "Charge", caller), Compensate: sagaStep("PaymentService", "Refund", caller)}`,
		"func sagaStep(service, method string, caller SagaCaller) workflow.StepFunc",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated saga.go missing %q\n--- got ---\n%s", want, src)
		}
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "app/saga.go", got, parser.SkipObjectResolution); err != nil {
		t.Errorf("generated saga.go does not parse: %v", err)
	}
}

// TestGenerateSagaOnlyModule proves a module holding sagas but no services of
// its own still renders (just its saga.go), while a service-only module never
// gains a saga.go.
func TestGenerateSagaOnlyModule(t *testing.T) {
	t.Parallel()

	app := `entity Item {
	id: uuid @primary
}

service InventoryService {
	rpc Reserve(item_id: uuid) -> Item {
		auth: required
	}
}`

	flows := `saga FulfillSaga {
	step reserve {
		execute: InventoryService.Reserve
	}
}`

	files := []*ast.File{
		parseSchemaFile(t, "schema/app/a.zen", app),
		parseSchemaFile(t, "schema/flows/f.zen", flows),
	}

	schema, diags := resolver.ResolveWithSchemaDir(files, "schema")
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, ok := out["flows/saga.go"]; !ok {
		t.Fatalf("Generate missing flows/saga.go; keys = %v", keysOf(out))
	}

	if _, ok := out["app/saga.go"]; ok {
		t.Fatalf("service-only module app gained a saga.go; keys = %v", keysOf(out))
	}

	for _, path := range []string{"app/types.go", "app/service.go", "app/router.go", "app/grpc.go", "app/register.go"} {
		if _, ok := out[path]; !ok {
			t.Errorf("Generate missing %q", path)
		}
	}

	for path := range out {
		if strings.HasPrefix(path, "flows/") && path != "flows/saga.go" {
			t.Errorf("saga-only module rendered unexpected %q", path)
		}
	}
}

func TestGenerateSagaDeterministic(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, sagaFixture)

	first, err1 := New().Generate(schema)
	second, err2 := New().Generate(schema)

	if err1 != nil || err2 != nil {
		t.Fatalf("Generate errors: %v / %v", err1, err2)
	}

	if string(first["app/saga.go"]) != string(second["app/saga.go"]) {
		t.Fatalf("app/saga.go differs between runs:\n--- first ---\n%s\n--- second ---\n%s",
			first["app/saga.go"], second["app/saga.go"])
	}
}

func parseSchemaFile(t *testing.T, name, src string) *ast.File {
	t.Helper()

	file, diags := dslparser.New(name, []byte(src)).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse %s errors: %v", name, diags)
	}

	return file
}
