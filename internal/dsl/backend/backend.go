// Package backend defines the single extension point every DSL code
// generation target (proto, zenorm, and atlas now; typescript and
// flatbuffers later) implements against the resolved *ir.Schema.
package backend

import "github.com/zenta-dev/zever/internal/dsl/ir"

// Backend generates one or more output files from a resolved schema. Name
// identifies the backend (e.g. "proto"), and Generate returns a map from
// output-relative file path to file content.
type Backend interface {
	Name() string
	Generate(schema *ir.Schema) (map[string][]byte, error)
}
