// Package atlas implements the "atlas" backend.Backend: it renders a
// resolved *ir.Schema into Atlas (atlasgo.io) HCL schema files — one
// "schema.hcl" per ir.Module — describing every entity as a table with
// columns, a primary key, indexes, and foreign keys.
//
// The HCL is hand-emitted as text rather than produced through the
// ariga.io/atlas Go API. This backend only ever needs to write Atlas's
// well-documented, stable input file format, never to call into Atlas's
// engine, so it stays dependency-free like the rest of internal/dsl.
//
// This package also exposes a deliberately simpler sibling renderer
// (render_ddl.go) that turns the same IR into plain "CREATE TABLE IF NOT
// EXISTS" DDL for the `zever db migrate` bootstrap path. That DDL path is
// NOT Atlas-style migration planning: it has no schema diffing, no ALTER,
// no down migrations, and no drift detection. See RenderSchemaDDL.
package atlas

import (
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// Backend renders a resolved schema to Atlas HCL schema files.
type Backend struct{}

// New returns a new atlas Backend.
func New() *Backend {
	return &Backend{}
}

// Name returns the backend's identifier.
func (b *Backend) Name() string {
	return "atlas"
}

// Generate renders one "<module>/schema.hcl" file per schema.Modules entry
// (the implicit unnamed module renders to "schema.hcl" at the output root).
// Messages never emit tables — they are transport-only DTOs and are explicitly skipped.
func (b *Backend) Generate(schema *ir.Schema) (map[string][]byte, error) {
	out := make(map[string][]byte, len(schema.Modules))

	for _, m := range schema.Modules {
		// Explicit guard: messages never emit tables.
		_ = m.Messages

		path, content, err := renderModuleFile(m)
		if err != nil {
			return nil, err
		}

		out[path] = content
	}

	return out, nil
}
