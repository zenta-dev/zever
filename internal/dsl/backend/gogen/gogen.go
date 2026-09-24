// Package gogen implements the "gogen" backend.Backend: it renders a
// resolved *ir.Schema into the Go APPLICATION layer -- a business-logic
// Service interface, HTTP routing (via the router battery), and gRPC
// server wiring -- one set of five files per ir.Module that declares at
// least one service, at "<module_snake>/{types,service,router,grpc,register}.go"
// (the implicit unnamed module renders to "app/...").
//
// # Wire types are the real generated protobuf messages, not plain structs
//
// gogen no longer declares its own request/response Go structs. Every
// operation's request type, and every operation's return type (an entity or,
// for a `paginated: true` operation, a synthesized "<Op>Response" wrapper),
// is a real protobuf message the sibling protogogen backend generates for
// the exact same schema (Phase 1 of the "real gRPC by default" effort) --
// types.go only declares a Go type ALIAS onto each ("type CreateTaskRequest
// = pb.CreateTaskRequest"), never a new struct. This is what lets router.go
// encode/decode HTTP bodies with protojson (protobuf's canonical JSON
// mapping -- lowerCamelCase field names, a deliberate, accepted change from
// this backend's earlier plain-struct JSON shape) and lets grpc.go's
// generated adapter type satisfy protogogen's real
// "<Service>Server"/"Unimplemented<Service>Server" interface directly, with
// no separate fake message types of its own.
//
// This is still the counterpart to the zenorm backend, which renders
// the DATA layer (per-module query-builder Go source at
// "zen/gen/<module>/<module>.go") -- but gogen's generated
// application layer no longer imports zenorm's entity types at all
// (Operation.Returns' Go shape now comes from protogogen instead); a
// developer's own Service implementation is free to use zenorm's
// generated types internally, but that is business logic, entirely outside
// gogen's own generated files.
//
// # The Service interface IS the business-logic extension point
//
// This is not a separate mechanism from code generation: the generated
// Service interface in service.go is, by construction, the only place a
// developer plugs in business logic. router.go and grpc.go only decode a
// request, call the Service method, and map the returned error to a wire
// status -- they never contain business logic themselves, so regenerating
// them is always safe. The developer's own implementation of the interface
// lives in their own package, entirely outside gogen's output tree, which
// is what makes "regenerate the always-generated files without touching
// hand-written code" (safe regeneration) an architectural property of the
// output layout rather than something enforced by in-file markers.
//
// This IS a breaking change to that extension point's shape: a Service
// method's request parameter and return value are now the real protobuf
// message types (by pointer, e.g. "*CreateTaskRequest"/"*Task") instead of
// gogen's old plain value-typed structs -- an existing hand-written
// implementation must be updated to match.
package gogen

import (
	"fmt"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// Backend renders a resolved schema to per-module Go application-layer
// source files.
type Backend struct {
	pbImportRoot string
	// pbImportFlat selects which formula pbGoPackage uses to turn
	// pbImportRoot into a per-module import path. false (New()'s default)
	// applies the legacy "<root>/zeverv1" or "<root>/zever/<module>"
	// convention kept from the predecessor repo this backend was ported
	// from -- no zever gen package exists yet, so the default root is
	// override-required for any real project (see defaultPBImportRoot).
	// true (set by NewWithPBImportRoot) applies a flat
	// "<root>/<module>" (or bare "<root>" for the implicit module) formula
	// instead, matching what protogogen's OWN output layout actually is
	// everywhere, including outside this repo: protogogen compiles with
	// protoc's "paths=source_relative" (see
	// internal/dsl/backend/protogogen/request.go's codeGeneratorParameter),
	// so a module's generated package lands at "<out>/protogogen/<module>/"
	// -- mirroring its .proto source path exactly, never nested under an
	// extra "zever/" segment. The "/zever/"-prefixed formula is a legacy
	// repo-specific naming choice, not a property of how protogogen itself
	// lays out output, so an external caller's override must not have it
	// applied on their behalf.
	pbImportFlat bool
}

// New returns a new gogen Backend that assumes protogogen's generated
// message packages live under defaultPBImportRoot. No zever gen package
// exists yet, so this default is override-required for any real project:
// prefer NewWithPBImportRoot pointing at the project's actual protogogen
// output.
func New() *Backend {
	return &Backend{pbImportRoot: defaultPBImportRoot}
}

// NewWithPBImportRoot returns a new gogen Backend that imports each
// module's protogogen-generated message package from "<pbImportRoot>/
// <module>" (a named module) or bare "<pbImportRoot>" (the implicit
// unnamed module) instead of New()'s default root and "/zever/"-prefixed
// formula -- see Backend.pbImportFlat's doc comment for why this formula,
// not the default one, is the correct one for an override: it matches
// protogogen's actual "paths=source_relative" output layout, which every
// external caller's protogogen output follows, regardless of where they
// point pbImportRoot. Use this when the target project places its
// protogogen output somewhere other than under defaultPBImportRoot --
// Generate(schema) is never told the calling project's module path or
// --out layout, so an exact downstream import path can't be derived in
// general.
func NewWithPBImportRoot(pbImportRoot string) *Backend {
	if pbImportRoot == "" {
		pbImportRoot = defaultPBImportRoot
	}

	return &Backend{pbImportRoot: pbImportRoot, pbImportFlat: true}
}

// Name returns the backend's identifier.
func (b *Backend) Name() string {
	return "gogen"
}

// Generate renders "<module_snake>/{types,service,router,grpc}.go" for
// every schema.Modules entry holding at least one service. A module with no
// services has nothing to generate: an empty Service interface and a
// router.go/grpc.go with no handlers would compile but serve no purpose,
// mirroring zenorm's own "skip modules with nothing to generate" rule.
func (b *Backend) Generate(schema *ir.Schema) (map[string][]byte, error) {
	out := make(map[string][]byte)

	for _, m := range schema.Modules {
		if len(m.Services) == 0 {
			continue
		}

		pkg, dir := moduleNaming(m)
		data := newModuleModel(m, pkg, b.pbImportRoot, b.pbImportFlat)

		files, err := renderModuleFiles(pkg, dir, data)
		if err != nil {
			return nil, fmt.Errorf("gogen: module %s: %w", moduleLabel(m), err)
		}

		for path, content := range files {
			out[path] = content
		}
	}

	return out, nil
}

// moduleLabel names a module for error messages, standing in a readable
// phrase for the implicit unnamed module.
func moduleLabel(m *ir.Module) string {
	if m == nil || m.Name == "" {
		return "application"
	}

	return m.Name
}
