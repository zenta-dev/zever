// Package compile wires the DSL compiler pipeline (parser -> resolver ->
// backends) into a single top-level Compile function.
package compile

import (
	"context"
	"sort"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/backend"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/parser"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

// Result holds the outcome of a Compile call: the resolved schema (always
// non-nil once any file produced any AST) and, only when resolution had no
// errors, the per-backend generated outputs.
type Result struct {
	Schema  *ir.Schema
	Outputs map[string]map[string][]byte // backend Name() -> filename -> content
}

// Compile parses every entry of files, resolves the merged AST into an
// *ir.Schema, and (only if resolution has no errors) runs every backend over
// that schema. Files are processed in sorted-filename order so diagnostic
// order, IR slice order, and backend output bytes are all deterministic
// regardless of files' map iteration order.
//
// Outputs is nil whenever the accumulated diagnostics contain an error:
// backends assume a fully-resolved schema and must never run against one
// that isn't. A backend's own Generate error is recorded as a diagnostic and
// does not prevent other backends from running.
//
// Compile runs every backend's Generate with no cancellation hook (the
// same as always); use CompileContext to bound/cancel backends that
// implement backend.ContextBackend (currently only protogogen, which shells
// out to a subprocess).
func Compile(files map[string]string, backends ...backend.Backend) (*Result, diag.List) {
	return WithSchemaDir(files, "schema", backends...)
}

// WithSchemaDir is like Compile but with an explicit schemaDir for
// dir-derived module assignment. An empty schemaDir defaults to "schema".
func WithSchemaDir(files map[string]string, schemaDir string, backends ...backend.Backend) (*Result, diag.List) {
	return WithSchemaDirContext(context.Background(), files, schemaDir, backends...)
}

// CompileContext is Compile, but runs each backend.ContextBackend's
// GenerateContext with ctx instead of Generate -- so a caller with a real
// deadline/cancellation source (e.g. `zever generate --timeout`, or a CI
// job wanting to bound the whole compile) can actually reach the one
// backend that does I/O (protogogen shelling out to
// `go tool protoc-gen-go-grpc`) instead of that backend always fabricating
// context.Background() internally. Backends that only implement
// backend.Backend are unaffected: their plain Generate still runs the same
// as always, ctx or not.
func CompileContext(ctx context.Context, files map[string]string, backends ...backend.Backend) (*Result, diag.List) { //nolint:revive // CompileContext mirrors stdlib exec.CommandContext naming; bare Context would read as a type
	return WithSchemaDirContext(ctx, files, "schema", backends...)
}

// WithSchemaDirContext is CompileContext with an explicit schemaDir; see
// WithSchemaDir/CompileContext.
func WithSchemaDirContext(ctx context.Context, files map[string]string, schemaDir string, backends ...backend.Backend) (*Result, diag.List) {
	if schemaDir == "" {
		schemaDir = "schema"
	}

	var diags diag.List

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	sort.Strings(names)

	astFiles := make([]*ast.File, 0, len(names))

	for _, name := range names {
		p := parser.New(name, []byte(files[name]))

		file, fileDiags := p.ParseFile()
		astFiles = append(astFiles, file)
		diags = append(diags, fileDiags...)
	}

	schema, resolveDiags := resolver.ResolveWithSchemaDir(astFiles, schemaDir)
	diags = append(diags, resolveDiags...)

	result := &Result{Schema: schema}

	if diags.HasErrors() {
		return result, diags
	}

	result.Outputs = make(map[string]map[string][]byte, len(backends))

	for _, b := range backends {
		output, err := generateWithContext(ctx, b, schema)
		if err != nil {
			diags = append(diags, diag.Wrap(b.Name(), diag.Position{}, err, "backend failed: %v", err))
			continue
		}

		result.Outputs[b.Name()] = output
	}

	return result, diags
}

// generateWithContext runs b.GenerateContext(ctx, schema) when b implements
// backend.ContextBackend, else b.Generate(schema).
func generateWithContext(ctx context.Context, b backend.Backend, schema *ir.Schema) (map[string][]byte, error) {
	if cb, ok := b.(backend.ContextBackend); ok {
		return cb.GenerateContext(ctx, schema)
	}

	return b.Generate(schema)
}
