package resolver

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

// resolveSrc parses and resolves src, failing the test on any error
// diagnostic. Warnings are allowed.
func resolveSrc(t *testing.T, src string) (*ir.Schema, diag.List) {
	t.Helper()

	file, diags := parser.New("test.zen", []byte(src)).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	schema, diags := Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	return schema, diags
}

func TestResolveNilFilesYieldsEmptySchema(t *testing.T) {
	t.Parallel()

	schema, diags := Resolve(nil)
	if schema == nil {
		t.Fatal("Resolve(nil) returned nil schema, want non-nil")
	}

	if len(schema.Modules) == 0 {
		t.Fatal("Modules empty, want at least the implicit module")
	}

	if len(diags) != 0 {
		t.Fatalf("diags = %v, want none", diags)
	}
}

func TestResolveEmptyFileListYieldsNoDiags(t *testing.T) {
	t.Parallel()

	schema, diags := Resolve([]*ast.File{})
	if schema == nil || len(diags) != 0 {
		t.Fatalf("schema = %+v diags = %v, want non-nil schema and no diags", schema, diags)
	}
}

func TestResolveNilFileInListDoesNotPanic(t *testing.T) {
	t.Parallel()

	schema, diags := Resolve([]*ast.File{nil})
	if schema == nil {
		t.Fatal("schema is nil, want non-nil")
	}

	if len(diags) != 0 {
		t.Fatalf("diags = %v, want none", diags)
	}
}

func TestResolveDuplicateDeclReportsErrDuplicateDecl(t *testing.T) {
	t.Parallel()

	src := "entity Dup {\n  id: uuid\n}\nentity Dup {\n  id: uuid\n}"

	file, _ := parser.New("dup.zen", []byte(src)).ParseFile()
	_, diags := Resolve([]*ast.File{file})

	if !diags.HasErrors() {
		t.Fatal("diags has no errors, want duplicate-declaration error")
	}

	found := false
	for _, d := range diags {
		if errors.Is(d.Wrapped, ErrDuplicateDecl) {
			found = true
		}
	}

	if !found {
		t.Fatalf("no diagnostic wraps ErrDuplicateDecl: %v", diags)
	}
}

func TestResolveUnresolvedReferenceReportsErrUnresolvedReference(t *testing.T) {
	t.Parallel()

	file, _ := parser.New("unref.zen", []byte("entity A {\n  id: uuid\n  ref: Missing\n}")).ParseFile()
	_, diags := Resolve([]*ast.File{file})

	found := false
	for _, d := range diags {
		if errors.Is(d.Wrapped, ErrUnresolvedReference) {
			found = true
		}
	}

	if !found {
		t.Fatalf("no diagnostic wraps ErrUnresolvedReference: %v", diags)
	}
}

func TestResolveInvalidCronReportsErrInvalidCron(t *testing.T) {
	t.Parallel()

	src := "job J {\n  queue: default\n}\nschedule Nightly {\n  cron: \"not a cron\"\n  dispatch: J\n}"

	file, _ := parser.New("cron.zen", []byte(src)).ParseFile()
	_, diags := Resolve([]*ast.File{file})

	found := false
	for _, d := range diags {
		if errors.Is(d.Wrapped, ErrInvalidCron) {
			found = true
		}
	}

	if !found {
		t.Fatalf("no diagnostic wraps ErrInvalidCron: %v", diags)
	}
}

func TestResolveInvalidRelationReportsErrInvalidRelation(t *testing.T) {
	t.Parallel()

	src := "entity A {\n  id: uuid\n  has_many bs: B\n}"

	file, _ := parser.New("rel.zen", []byte(src)).ParseFile()
	_, diags := Resolve([]*ast.File{file})

	found := false
	for _, d := range diags {
		if errors.Is(d.Wrapped, ErrInvalidRelation) || errors.Is(d.Wrapped, ErrUnresolvedReference) {
			found = true
		}
	}

	if !found {
		t.Fatalf("no diagnostic wraps ErrInvalidRelation/ErrUnresolvedReference: %v", diags)
	}
}

func TestResolveCrossModuleReferenceReportsErrCrossModule(t *testing.T) {
	t.Parallel()

	files := []*ast.File{
		parseFileT(t, "schema/a/entities.zen", "entity A {\n  id: uuid\n}"),
		parseFileT(t, "schema/b/entities.zen", "entity B {\n  id: uuid\n  has_many as: A\n}"),
	}

	_, diags := ResolveWithSchemaDir(files, "schema")

	found := false
	for _, d := range diags {
		if errors.Is(d.Wrapped, ErrCrossModule) {
			found = true
		}
	}

	if !found {
		t.Fatalf("no diagnostic wraps ErrCrossModule: %v", diags)
	}
}

func TestResolveDirDerivedModuleAssignment(t *testing.T) {
	t.Parallel()

	files := []*ast.File{
		parseFileT(t, "schema/billing/invoices.zen", "entity Invoice {\n  id: uuid @primary\n}"),
		parseFileT(t, "schema/app.zen", "entity User {\n  id: uuid @primary\n}"),
	}

	schema, diags := ResolveWithSchemaDir(files, "schema")
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}

	names := map[string]bool{}
	for _, m := range schema.Modules {
		names[m.Name] = true
	}

	if !names["billing"] {
		t.Fatalf("modules = %v, want a \"billing\" module", names)
	}

	if !names[""] {
		t.Fatalf("modules = %v, want the implicit \"\" module", names)
	}
}

func TestResolveDeterministicAcrossCalls(t *testing.T) {
	t.Parallel()

	src := "entity User {\n  id: uuid @primary\n  has_many orders: Order\n}\nentity Order {\n  id: uuid\n  belongs_to user: User\n}\n"

	file, _ := parser.New("det.zen", []byte(src)).ParseFile()

	first, _ := Resolve([]*ast.File{file})
	second, _ := Resolve([]*ast.File{file})

	if len(first.Modules) != len(second.Modules) {
		t.Fatalf("module count differs: %d vs %d", len(first.Modules), len(second.Modules))
	}

	for i := range first.Modules {
		a, b := first.Modules[i], second.Modules[i]
		if a.Name != b.Name || a.Version != b.Version {
			t.Fatalf("module %d identity differs: %+v vs %+v", i, a, b)
		}

		if len(a.Entities) != len(b.Entities) {
			t.Fatalf("module %d entity count differs", i)
		}

		for j := range a.Entities {
			if len(a.Entities[j].Fields) != len(b.Entities[j].Fields) {
				t.Fatalf("entity %d field count differs", j)
			}
		}
	}
}

func TestResolveBestEffortReportsMultipleIndependentErrors(t *testing.T) {
	t.Parallel()

	src := "entity A {\n  id: nosuchtype\n}\nentity B {\n  id: uuid\n  ref: Missing\n}\nservice S {\n  rpc G() -> AlsoMissing\n}"

	file, _ := parser.New("multi.zen", []byte(src)).ParseFile()
	schema, diags := Resolve([]*ast.File{file})

	if !diags.HasErrors() {
		t.Fatal("diags has no errors, want several")
	}

	if schema == nil {
		t.Fatal("schema is nil, want non-nil even with errors")
	}

	errCount := 0
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			errCount++
		}
	}

	if errCount < 2 {
		t.Fatalf("error diags = %d, want at least 2 independent errors", errCount)
	}
}

func TestResolveEmptyASTFileYieldsNoDiags(t *testing.T) {
	t.Parallel()

	_, diags := Resolve([]*ast.File{{Name: "empty.zen"}})
	if len(diags) != 0 {
		t.Fatalf("diags = %v, want none", diags)
	}
}

func TestResolveMissingAuthReportsErrMissingAuth(t *testing.T) {
	t.Parallel()

	src := "entity User {\n  id: uuid @primary\n}\nservice S {\n  rpc G() -> User {\n  }\n}"

	file, _ := parser.New("auth.zen", []byte(src)).ParseFile()
	_, diags := Resolve([]*ast.File{file})

	found := false
	for _, d := range diags {
		if errors.Is(d.Wrapped, ErrMissingAuth) {
			found = true
		}
	}

	if !found {
		t.Fatalf("no diagnostic wraps ErrMissingAuth: %v", diags)
	}
}

func TestResolveRenamedFromFieldResolves(t *testing.T) {
	t.Parallel()

	src := "entity User {\n  id: uuid @primary\n  login: string @renamed_from(\"email\")\n}"

	schema, diags := resolveSrc(t, src)
	if len(diags) != 0 {
		t.Fatalf("diags = %v, want none", diags)
	}

	var user *ir.Entity
	for _, m := range schema.Modules {
		for _, e := range m.Entities {
			if e.Name == "User" {
				user = e
			}
		}
	}

	if user == nil {
		t.Fatal("User entity not found")
	}

	f := user.FieldByName("login")
	if f == nil {
		t.Fatal("login field not found")
	}

	if f.RenamedFrom == nil || *f.RenamedFrom != "email" {
		t.Fatalf("RenamedFrom = %v, want \"email\"", f.RenamedFrom)
	}
}

func parseFileT(t *testing.T, name, src string) *ast.File {
	t.Helper()

	file, diags := parser.New(name, []byte(src)).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse %s: %v", name, diags)
	}

	return file
}
