package compile

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
)

// TestCompileValidMultiFileWithAtlasBackend mirrors
// TestCompileValidMultiFileWithQueryEngineBackend, proving Compile wires
// the atlas backend the same way: one Atlas HCL file per module, keyed
// under Outputs["atlas"].
func TestCompileValidMultiFileWithAtlasBackend(t *testing.T) {
	t.Parallel()

	userSrc := `entity User {
		id: uuid @primary
		email: string @unique
	}`

	orderSrc := `entity OrderItem {
		id: uuid @primary
		user_id: uuid
		sku: string

		belongs_to user: User @foreign_key(user_id) @on_delete(cascade)
	}`

	files := map[string]string{
		"a_user.zen":  userSrc,
		"b_order.zen": orderSrc,
	}

	result, diags := Compile(files, atlas.New())

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if len(diags) != 0 {
		t.Fatalf("expected zero diagnostics, got %d: %v", len(diags), diags)
	}

	if result.Outputs == nil {
		t.Fatalf("Outputs is nil, want non-nil")
	}

	atlasOut, ok := result.Outputs["atlas"]
	if !ok {
		t.Fatalf("Outputs missing %q key, got keys %v", "atlas", keysOf(result.Outputs))
	}

	wantKeys := []string{"schema.hcl"}

	if len(atlasOut) != len(wantKeys) {
		t.Fatalf("Outputs[%q] keys = %v, want exactly %v", "atlas", keysOf(atlasOut), wantKeys)
	}

	content := string(atlasOut["schema.hcl"])

	for _, want := range []string{
		`schema "public" {`,
		`table "users" {`,
		`table "order_items" {`,
		`foreign_key "order_items_user_id_fkey" {`,
		`on_delete   = CASCADE`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("schema.hcl missing %q:\n%s", want, content)
		}
	}
}

// TestCompileAtlasPerModuleOutput proves one Atlas HCL file is emitted per
// declared module.
func TestCompileAtlasPerModuleOutput(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"schema/billing/invoice.zen": `entity Invoice {
				id: uuid @primary
				amount_cents: int64
			}`,
		"schema/shipping/shipment.zen": `entity Shipment {
				id: uuid @primary
				tracking: string @unique
			}`,
	}

	result, diags := WithSchemaDir(files, "schema", atlas.New())
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	atlasOut := result.Outputs["atlas"]

	for _, k := range []string{"billing/schema.hcl", "shipping/schema.hcl"} {
		if _, ok := atlasOut[k]; !ok {
			t.Fatalf("Outputs[%q] missing %q, got keys %v", "atlas", k, keysOf(atlasOut))
		}
	}
}
