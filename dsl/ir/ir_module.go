package ir

import "github.com/zenta-dev/zever/internal/dsl/diag"

// Module groups every declaration that belongs together for the "database
// ownership is per-module, no cross-module joins" rule (design doc §10).
// Name is derived from the file's immediate subdirectory under schemaDir:
// schema/billing/invoices/x.zen -> "billing", schema/app.zen -> "". Name
// == "" is the public sentinel (files directly under schemaDir) and is not
// a real named module.
type Module struct {
	Name string
	// Version is the dir-derived API version segment (e.g. "v1"), or "" for
	// a schema laid out without one: schema/v1/iam/*.zen -> Version "v1",
	// Name "iam"; schema/billing/*.zen -> Version "", Name "billing". See
	// resolver.versionAndModuleForPath for the exact derivation rule.
	Version   string
	Entities  []*Entity
	Messages  []*Message
	Services  []*Service
	Jobs      []*Job
	Schedules []*Schedule
	Enums     []*Enum
	Pos       diag.Position // zero Position for dir-derived modules
}

// Enum is a named, reusable set of string values declared at the top level
// with `enum Name { value1, value2, ... }`. Unlike Entity/Message/Service,
// enums are global: a field in any module may reference an enum declared in
// any other module (see resolver_enum.go), so Module here only records where
// the declaration was written, not a visibility boundary.
type Enum struct {
	Name       string
	Values     []string
	Module     *Module
	Pos        diag.Position
	DocComment string
}

// Schema represents the complete resolved schema.
type Schema struct {
	Modules []*Module
}
