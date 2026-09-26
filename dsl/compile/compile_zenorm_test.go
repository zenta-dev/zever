package compile

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/backend/zenorm"
)

// TestCompileValidMultiFileWithZenormBackend mirrors
// TestCompileValidMultiFileWithProtoBackend, proving Compile wires the
// zenorm backend the same way: one generated Go source file per module --
// entities from separate .zen files still share the implicit module, so
// they merge into one package -- keyed under Outputs["zenorm"].
func TestCompileValidMultiFileWithZenormBackend(t *testing.T) {
	t.Parallel()

	userSrc := `entity User {
		id: uuid @primary
		email: string @unique
	}`

	orderSrc := `entity OrderItem {
		id: uuid @primary
		sku: string
	}`

	files := map[string]string{
		"a_user.zen":  userSrc,
		"b_order.zen": orderSrc,
	}

	result, diags := Compile(files, zenorm.New())

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if len(diags) != 0 {
		t.Fatalf("expected zero diagnostics, got %d: %v", len(diags), diags)
	}

	if result.Outputs == nil {
		t.Fatalf("Outputs is nil, want non-nil")
	}

	zeOut, ok := result.Outputs["zenorm"]
	if !ok {
		t.Fatalf("Outputs missing %q key, got keys %v", "zenorm", keysOf(result.Outputs))
	}

	wantKeys := []string{"orm/gen/app/app.go"}

	if len(zeOut) != len(wantKeys) {
		t.Fatalf("Outputs[%q] keys = %v, want exactly %v", "zenorm", keysOf(zeOut), wantKeys)
	}

	for _, k := range wantKeys {
		if _, ok := zeOut[k]; !ok {
			t.Fatalf("Outputs[%q] missing %q, got keys %v", "zenorm", k, keysOf(zeOut))
		}
	}
}
