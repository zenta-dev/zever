package compile

import (
	"bytes"
	"testing"

	"github.com/zenta-dev/zever/dsl/backend/proto"
	"github.com/zenta-dev/zever/dsl/ir"
)

// failBackend always fails Generate, recording that it was called.
type failBackend struct{ called bool }

func (b *failBackend) Name() string { return "fail" }
func (b *failBackend) Generate(*ir.Schema) (map[string][]byte, error) {
	b.called = true

	return nil, errFake
}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake backend failure" }

func TestCompileEmptyFilesMap(t *testing.T) {
	t.Parallel()

	result, diags := Compile(nil)
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}

	if result.Schema == nil {
		t.Fatal("Schema is nil, want non-nil")
	}

	if result.Outputs == nil || len(result.Outputs) != 0 {
		t.Fatalf("Outputs = %v, want non-nil empty map", result.Outputs)
	}
}

func TestCompileEmptyFileContents(t *testing.T) {
	t.Parallel()

	result, diags := Compile(map[string]string{"empty.zen": ""})
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}

	if result.Schema == nil {
		t.Fatal("Schema is nil, want non-nil")
	}
}

func TestCompileDeterministicOutputs(t *testing.T) {
	t.Parallel()

	files := loadAppFixture(t)

	first, diags1 := Compile(files, proto.New())
	second, diags2 := Compile(files, proto.New())

	if diags1.HasErrors() || diags2.HasErrors() {
		t.Fatalf("diags = %v / %v, want none", diags1, diags2)
	}

	if len(first.Outputs) != len(second.Outputs) {
		t.Fatalf("output backend count differs: %d vs %d", len(first.Outputs), len(second.Outputs))
	}

	for backend, files1 := range first.Outputs {
		files2, ok := second.Outputs[backend]
		if !ok {
			t.Fatalf("second run missing backend %q", backend)
		}

		if len(files1) != len(files2) {
			t.Fatalf("backend %q file count differs: %d vs %d", backend, len(files1), len(files2))
		}

		for name, content1 := range files1 {
			content2, ok := files2[name]
			if !ok {
				t.Fatalf("backend %q second run missing file %q", backend, name)
			}

			if !bytes.Equal(content1, content2) {
				t.Fatalf("backend %q file %q differs between runs", backend, name)
			}
		}
	}
}

func TestCompileSkipsBackendsWhenResolutionFails(t *testing.T) {
	t.Parallel()

	failing := &failBackend{}

	files := map[string]string{"bad.zen": "entity A {\n  id: nosuchtype\n}"}
	result, diags := Compile(files, failing)

	if !diags.HasErrors() {
		t.Fatal("diags has no errors, want resolution failure")
	}

	if failing.called {
		t.Fatal("backend Generate ran despite resolution errors, want skipped")
	}

	if result.Outputs != nil {
		t.Fatalf("Outputs = %v, want nil when resolution fails", result.Outputs)
	}
}

func TestCompileBackendErrorRecordedAsDiag(t *testing.T) {
	t.Parallel()

	failing := &failBackend{}

	result, diags := Compile(loadAppFixture(t), failing)
	if !diags.HasErrors() {
		t.Fatal("diags has no errors, want backend failure recorded")
	}

	if !failing.called {
		t.Fatal("backend Generate never called")
	}

	if result.Schema == nil {
		t.Fatal("Schema is nil, want non-nil")
	}

	if _, ok := result.Outputs["fail"]; ok {
		t.Fatal("Outputs contains failing backend, want it omitted")
	}
}

func TestCompileWarningsDoNotBlockBackends(t *testing.T) {
	t.Parallel()

	// A v1 module whose HTTP path lacks the "/v1/" prefix triggers the
	// warning-severity version-drift lint; warnings must not block backends.
	files := map[string]string{
		"schema/v1/iam/app.zen": "entity User {\n  id: uuid @primary\n}\nservice S {\n  rpc G(id: uuid) -> User {\n    http: GET \"/users\"\n    auth: none\n  }\n}",
	}

	result, diags := Compile(files, proto.New())
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want warnings only", diags)
	}

	if len(diags) == 0 {
		t.Fatal("expected at least one warning diagnostic")
	}

	if len(result.Outputs) == 0 {
		t.Fatal("Outputs empty, want backends to have run despite warnings")
	}
}

func TestCompileWithSchemaDirEmptyDefaultsToSchema(t *testing.T) {
	t.Parallel()

	files := map[string]string{"schema/app.zen": "entity User {\n  id: uuid @primary\n}"}

	result, diags := WithSchemaDir(files, "")
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}

	if result.Schema == nil {
		t.Fatal("Schema is nil, want non-nil")
	}
}
