// Package backend defines the single extension point every DSL code
// generation target (proto, zenorm, atlas, gogen, openapi, and protogogen;
// no typescript or flatbuffers backends exist) implements against the
// resolved *ir.Schema.
package backend

import (
	"context"

	"github.com/zenta-dev/zever/dsl/ir"
)

// Backend generates one or more output files from a resolved schema. Name
// identifies the backend (e.g. "proto"), and Generate returns a map from
// output-relative file path to file content.
type Backend interface {
	Name() string
	Generate(schema *ir.Schema) (map[string][]byte, error)
}

// ContextBackend is an optional extension a Backend implements when its
// Generate does real I/O that should honor cancellation/deadlines (e.g.
// protogogen shelling out to `go tool protoc-gen-go-grpc`) -- every other
// backend is pure/CPU-bound and has no need for it. compile.CompileContext/
// WithSchemaDirContext check for this interface via type assertion and call
// GenerateContext when present, falling back to the plain Generate
// otherwise; a Backend that only implements Backend keeps working
// unchanged.
type ContextBackend interface {
	Backend
	GenerateContext(ctx context.Context, schema *ir.Schema) (map[string][]byte, error)
}
