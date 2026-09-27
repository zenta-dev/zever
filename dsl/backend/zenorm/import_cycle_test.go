package zenorm

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestModuleEntitiesShareOneImportFreePackage proves this backend emits
// every entity of one module in exactly one Go file/package, referenced by
// bare in-package identifier, with the only external import being the orm
// core package.
func TestModuleEntitiesShareOneImportFreePackage(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique
	}

	entity Order {
		id: uuid @primary
		user_id: uuid
		amount_cents: int64
	}`

	out, err := New().Generate(compileSchema(t, src))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(out) != 1 {
		t.Fatalf("Generate produced %d files (%v), want exactly 1", len(out), keys(out))
	}

	const path = "orm/gen/app/app.go"

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, content, parser.AllErrors)
	if err != nil {
		t.Fatalf("generated %s does not parse: %v\n--- content ---\n%s", path, err, content)
	}

	assertOnlyOrmImport(t, file, content)
	assertNoQualifiedEntityRefs(t, fset, file, content)
	assertDeclaresBothEntities(t, file, content)
}

// ormRuntimeImport is the only github.com/zenta-dev/zever import a
// generated module package is ever allowed to carry in this phase.
const ormRuntimeImport = `"github.com/zenta-dev/zever/orm"`

func assertOnlyOrmImport(t *testing.T, file *ast.File, content []byte) {
	t.Helper()

	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}

		if imp.Path.Value == ormRuntimeImport {
			continue
		}

		if strings.HasPrefix(imp.Path.Value, `"github.com/zenta-dev/zever/`) {
			t.Fatalf("generated module package imports a non-orm zever package %s -- "+
				"this phase's codegen must depend only on the orm core package:\n%s", imp.Path.Value, content)
		}
	}
}

// assertNoQualifiedEntityRefs walks every selector expression in the file
// and fails if any of them qualifies an entity-derived identifier with a
// package name other than an allowed runtime import.
func assertNoQualifiedEntityRefs(t *testing.T, fset *token.FileSet, file *ast.File, content []byte) {
	t.Helper()

	allowedQualifiers := map[string]bool{"orm": true, "time": true, "fmt": true, "json": true}

	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Obj != nil {
			return true
		}

		if allowedQualifiers[ident.Name] {
			return true
		}

		t.Fatalf("generated code at %s references %s.%s through a package qualifier; "+
			"same-module entities must be referenced by bare identifier:\n%s",
			fset.Position(sel.Pos()), ident.Name, sel.Sel.Name, content)

		return false
	})
}

func assertDeclaresBothEntities(t *testing.T, file *ast.File, content []byte) {
	t.Helper()

	found := map[string]bool{}

	ast.Inspect(file, func(n ast.Node) bool {
		if ts, ok := n.(*ast.TypeSpec); ok {
			found[ts.Name.Name] = true
		}

		if fd, ok := n.(*ast.FuncDecl); ok && fd.Recv == nil {
			found[fd.Name.Name] = true
		}

		if vs, ok := n.(*ast.ValueSpec); ok {
			for _, name := range vs.Names {
				found[name.Name] = true
			}
		}

		return true
	})

	for _, want := range []string{"User", "Order", "Users", "Orders"} {
		if !found[want] {
			t.Fatalf("generated single-package output is missing top-level %s:\n%s", want, content)
		}
	}
}
