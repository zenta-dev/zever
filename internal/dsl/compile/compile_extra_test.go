package compile

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// stubBackend is a fake backend.Backend for exercising Compile's backend
// loop without any real code generator.
type stubBackend struct {
	name string
	out  map[string][]byte
	err  error
}

func (s stubBackend) Name() string { return s.name }

func (s stubBackend) Generate(*ir.Schema) (map[string][]byte, error) { return s.out, s.err }

const extraUserSrc = `entity User {
	id: uuid @primary
	email: string @unique
}`

func TestCompileEmptySchemaDirDefaultsToSchema(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"schema/billing/entities.zen": `entity Invoice {
			id: uuid @primary
			amount_cents: int64
		}`,
	}

	result, diags := WithSchemaDir(files, "")
	if diags.HasErrors() {
		t.Fatalf("WithSchemaDir with empty schemaDir: %v", diags)
	}

	mod := findModule(result.Schema, "billing")
	if mod == nil {
		t.Fatalf("billing module not found; empty schemaDir did not default to %q", "schema")
	}
}

func TestCompileRunsBackendsAndCollectsOutputs(t *testing.T) {
	t.Parallel()

	files := map[string]string{"a_user.zen": extraUserSrc}
	want := map[string][]byte{"out.txt": []byte("hello")}

	result, diags := Compile(files, stubBackend{name: "stub", out: want})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	got, ok := result.Outputs["stub"]
	if !ok {
		t.Fatalf("Outputs missing %q key, got keys %v", "stub", keysOf(result.Outputs))
	}

	if string(got["out.txt"]) != "hello" {
		t.Fatalf("Outputs[stub][out.txt] = %q, want %q", got["out.txt"], "hello")
	}
}

func TestCompileBackendErrorRecordedOtherBackendsStillRun(t *testing.T) {
	t.Parallel()

	files := map[string]string{"a_user.zen": extraUserSrc}
	boom := errors.New("boom")

	result, diags := Compile(files,
		stubBackend{name: "bad", err: boom},
		stubBackend{name: "good", out: map[string][]byte{"ok.txt": []byte("ok")}},
	)

	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics recording the backend failure, got none")
	}

	if _, ok := result.Outputs["bad"]; ok {
		t.Fatalf("failed backend %q must not have an Outputs entry", "bad")
	}

	if _, ok := result.Outputs["good"]; !ok {
		t.Fatalf("successful backend %q missing from Outputs; one backend's failure must not stop the rest", "good")
	}
}
