// Package resolver turns a parsed []*ast.File into a resolved *ir.Schema,
// reporting problems as diag.Diagnostic values rather than failing fast:
// every pass keeps checking after finding one bad declaration so a single
// Resolve call surfaces as many independent problems as possible.
package resolver

import (
	"errors"
	"sort"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// Sentinel errors wrapped by diagnostics produced anywhere in this package.
// All eight are declared here even though ErrInvalidRelation,
// ErrInvalidOption, and ErrCrossModule are first used by a later task
// (relations/services/jobs/schedules) — one file, one place errors live.
var (
	// ErrDuplicateDecl is reported when two top-level declarations (of any
	// of the five kinds) share the same name.
	ErrDuplicateDecl = errors.New("resolver: duplicate top-level declaration")
	// ErrUnresolvedReference is reported when an identifier (a field type,
	// an enum default, a relation target, ...) does not resolve to anything
	// known.
	ErrUnresolvedReference = errors.New("resolver: unresolved reference")
	// ErrInvalidRelation is reported for a malformed relation declaration.
	ErrInvalidRelation = errors.New("resolver: invalid relation")
	// ErrInvalidAttribute is reported for a malformed or unrecognized
	// attribute (entity-level, field-level, or otherwise).
	ErrInvalidAttribute = errors.New("resolver: invalid attribute")
	// ErrInvalidOption is reported for a malformed rpc/job/schedule option.
	ErrInvalidOption = errors.New("resolver: invalid rpc/job/schedule option")
	// ErrInvalidCron is reported for a malformed cron expression.
	ErrInvalidCron = errors.New("resolver: invalid cron expression")
	// ErrMixedModuleMode is reported when a compiled file set mixes bare
	// top-level declarations with module blocks. In dir-derived mode it is
	// unused but kept for backward compatibility.
	ErrMixedModuleMode = errors.New("resolver: cannot mix top-level declarations with module blocks")
	// ErrCrossModule is reported when a reference crosses a module
	// boundary.
	ErrCrossModule = errors.New("resolver: reference crosses a module boundary")
	// ErrMissingValidation is reported when a request-position field lacks @validate.
	ErrMissingValidation = errors.New("resolver: missing validation")
	// ErrMissingAuth is reported when an RPC operation lacks auth/permission.
	ErrMissingAuth = errors.New("resolver: missing auth")
	// ErrVersionDrift is reported (as a warning, never an error) when an
	// operation's HTTP path doesn't match its module's dir-derived version.
	ErrVersionDrift = errors.New("resolver: http path does not match module version")
)

// Resolve runs the resolver pipeline over files, in the order given (the
// caller is expected to have already sorted files by name for determinism).
// It always returns a non-nil *ir.Schema, with its Modules slice populated
// from Pass -1 regardless of what fails later — a bad relation in entity A
// never prevents entity B, or any service/job/schedule, from being resolved
// and checked, and a cross-module violation in one module never prevents
// another module's own declarations from resolving cleanly.
//
// Task 7 implemented Passes -1, 0, and 1: module grouping, symbol
// collection, and entity field resolution. Task 8 (this) adds Passes 2-6:
// relations and their many-to-many symmetry pass, indexes, services/RPCs,
// jobs, and schedules — without altering the passes above.
func Resolve(files []*ast.File) (*ir.Schema, diag.List) {
	return ResolveWithSchemaDir(files, "schema")
}

// ResolveWithSchemaDir runs the resolver pipeline over files with an explicit
// schemaDir for dir-derived module assignment. schemaDir defaults to "schema" when empty.
func ResolveWithSchemaDir(files []*ast.File, schemaDir string) (*ir.Schema, diag.List) {
	//nolint:prealloc // appended diagnostic count depends on error yield, unknowable upfront
	var diags diag.List

	modResult, modDiags := resolveModules(files, schemaDir)
	diags = append(diags, modDiags...)

	syms, symDiags := resolveSymbols(modResult.leafDecls)
	diags = append(diags, symDiags...)

	enumDiags := resolveEnums(syms.enumOrder, modResult.declModule)
	diags = append(diags, enumDiags...)

	enumByName := buildEnumIndex(modResult.modules)

	entityDiags := resolveEntities(syms.entityOrder, modResult.declModule, enumByName)
	diags = append(diags, entityDiags...)

	// Declare messages first: an empty *ir.Message{Name, Module, Pos} shell
	// per decl, with Fields left nil. Messages may reference each other (or
	// an entity) by name in any declaration order, so every message needs
	// stable pointer identity -- and therefore a slot in messageByName --
	// before any message's fields are actually resolved.
	declareMessages(syms.messageOrder, modResult.declModule)

	schema := &ir.Schema{Modules: moduleSlice(modResult.modules)}

	entityByName := buildEntityIndex(schema.Modules)
	messageByName := buildMessageIndex(schema.Modules)

	// Now fill in each declared message's fields, with both indexes
	// available: a field can resolve as a Ref to an entity/message (mirroring
	// resolveParam) or fall back to a scalar FieldType.
	msgFieldDiags := resolveMessageFields(syms.messageOrder, modResult.declModule, entityByName, messageByName, enumByName)
	diags = append(diags, msgFieldDiags...)

	relDiags := resolveRelationsAndIndexes(syms.entityOrder, entityByName)
	diags = append(diags, relDiags...)

	svcDiags := resolveServices(syms.serviceOrder, modResult.declModule, entityByName, messageByName, enumByName)
	diags = append(diags, svcDiags...)

	jobDiags := resolveJobs(syms.jobOrder, modResult.declModule, entityByName, messageByName, enumByName)
	diags = append(diags, jobDiags...)

	jobsByName := buildJobIndex(schema.Modules)

	schedDiags := resolveSchedules(syms.scheduleOrder, modResult.declModule, jobsByName)
	diags = append(diags, schedDiags...)

	// Hard opinions: validate-everything-in, secure-everything, module-boundary.
	diags = append(diags, validateEverythingIn(schema)...)
	diags = append(diags, secureEverything(schema)...)
	diags = append(diags, CheckCrossModule(schema)...)

	// Opt-in lint, warning-severity only: never blocks compilation.
	diags = append(diags, versionPathDrift(schema)...)

	return schema, diags
}

// moduleSlice converts a module map into a slice sorted by (version, name),
// so the empty string (the implicit module/version) always sorts first, the
// order of ir.Schema.Modules never depends on Go's randomized map order, and
// two modules sharing a name across versions (v1/iam, v2/iam) still sort
// deterministically against each other rather than relying on sort.Slice's
// unspecified tie-breaking for equal keys.
func moduleSlice(modules map[string]*ir.Module) []*ir.Module {
	out := make([]*ir.Module, 0, len(modules))

	for _, m := range modules {
		out = append(out, m)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}

		return out[i].Name < out[j].Name
	})

	return out
}
