package compile

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/dsl/backend/proto"
)

// TestWithSchemaDirContextDefaultsSchemaDir pins the empty-schemaDir
// default: an empty string behaves exactly like Compile/CompileContext,
// deriving module names from the "schema" directory prefix.
func TestWithSchemaDirContextDefaultsSchemaDir(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"schema/billing/orders.zen": `entity Order {
			id: uuid @primary
		}`,
	}

	result, diags := WithSchemaDirContext(t.Context(), files, "", proto.New())
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if findModule(result.Schema, "billing") == nil {
		t.Fatal("billing module not found with empty schemaDir")
	}

	if _, ok := result.Outputs["proto"]; !ok {
		t.Fatalf("Outputs missing proto key: %v", result.Outputs)
	}
}

// TestWithSchemaDirContextUsesCallerCtx proves the ctx argument reaches a
// backend.ContextBackend: the fake backend records the marker value, and
// its plain Generate is never called.
func TestWithSchemaDirContextUsesCallerCtx(t *testing.T) {
	t.Parallel()

	cb := &fakeContextBackend{}
	ctx := context.WithValue(t.Context(), markerKey{}, "via-schemadir")

	result, diags := WithSchemaDirContext(ctx, map[string]string{"app.zen": minimalSchemaSrc}, "schema", cb)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if cb.gotMarker != "via-schemadir" {
		t.Fatalf("GenerateContext marker = %v, want via-schemadir", cb.gotMarker)
	}

	if cb.generateCalls != 0 {
		t.Fatalf("Generate called %d times, want 0", cb.generateCalls)
	}

	if _, ok := result.Outputs["fake-ctx"]; !ok {
		t.Fatalf("missing fake-ctx output: %v", result.Outputs)
	}
}

// TestWithSchemaDirContextErrorStillReturnsSchema pins the best-effort
// contract: resolution diagnostics are returned alongside a non-nil schema
// and nil Outputs.
func TestWithSchemaDirContextErrorStillReturnsSchema(t *testing.T) {
	t.Parallel()

	broken := map[string]string{
		"app.zen": `entity Broken {
			id: uuid @primary
			bad: nonexistent_type
		}`,
	}

	result, diags := WithSchemaDirContext(t.Context(), broken, "schema", proto.New())
	if !diags.HasErrors() {
		t.Fatal("expected diagnostics for broken schema, got none")
	}

	if result.Schema == nil {
		t.Fatal("Result.Schema is nil, want non-nil")
	}

	if result.Outputs != nil {
		t.Fatalf("Outputs = %v, want nil on error", result.Outputs)
	}
}

// BenchmarkWithSchemaDirContext measures the full parse-resolve-generate
// pipeline with an explicit schemaDir on the proto backend.
func BenchmarkWithSchemaDirContext(b *testing.B) {
	files := map[string]string{
		"schema/app/users.zen": `entity User {
			id: uuid @primary
			email: string @unique
		}`,
	}

	b.ReportAllocs()

	for b.Loop() {
		result, diags := WithSchemaDirContext(b.Context(), files, "schema", proto.New())
		if diags.HasErrors() {
			b.Fatalf("unexpected diagnostics: %v", diags)
		}

		if result.Schema == nil {
			b.Fatal("Result.Schema is nil")
		}
	}
}
