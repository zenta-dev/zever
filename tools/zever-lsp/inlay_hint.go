package main

import (
	"fmt"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// Inlay hints use the protocol's native LSP 3.18 types directly:
// protocol.InlayHint with its Label union arm set via protocol.String(...).
// This file only computes hint data; the textDocument/inlayHint handler that
// serves it lives with the server shell.

// inlayHintsAt returns every inlay hint for one file.
//
// Scope is deliberately narrow, per an explicit check of both places the
// task asked about:
//
//   - Field defaults: resolver_entity.go's resolveField/
//     resolveDefaultAttribute show every field default is fully explicit --
//     set only by a literal, explicit @default(...) attribute (now() calls,
//     bool/enum idents, or a matching literal). There is no field that
//     silently acquires a default the way e.g. a struct's zero value would;
//     an Optional field with no @default just has a nil Field.Default. So
//     there is nothing non-obvious to hint there, and none is implemented.
//   - Implicit module: resolver_module.go's moduleForPath derives a
//     declaration's module purely from the file's directory under
//     schemaDir, with nothing in the entity/message/service declaration's
//     own source naming it. A file sitting directly in schemaDir (the
//     common case -- every one of this repo's own example schemas is laid
//     out this way) resolves to the implicit "public" module
//     (ir.Module.Name == ""), which is a real, otherwise-invisible fact
//     about the declaration.
//
// That module hint is real signal only in a schema that actually mixes
// public and named modules: in an all-public schema, it would fire on
// every single declaration, which is noise rather than information. So it
// is suppressed entirely unless the compiled schema has at least one named
// (non-public) module -- see schemaHasNamedModule. This gate applies to the
// module-hint pass only; errorCaseHints below is not module-structure
// noise, it applies regardless of how many modules a schema has. Each
// family can still be switched off via Settings -- see inlayHintsFor.
// inlayHintsAt returns every inlay hint for one file under the default
// settings -- the settings-free entry point the tests (and any caller that
// knows no configuration) use. The settings-aware computation lives in
// inlayHintsFor.
func inlayHintsAt(schema *ir.Schema, file *ast.File) []protocol.InlayHint {
	return inlayHintsFor(schema, file, defaultSettings())
}

// inlayHintsFor is inlayHintsAt with the settings applied: each hint family
// can be switched off individually, with Settings.ImplicitModuleHints
// removing the module pass and Settings.ErrorCaseHints the error-case pass.
func inlayHintsFor(schema *ir.Schema, file *ast.File, settings Settings) []protocol.InlayHint {
	if file == nil {
		return nil
	}

	var hints []protocol.InlayHint

	if settings.ImplicitModuleHints && schemaHasNamedModule(schema) {
		for _, decl := range flattenDecls(file) {
			switch d := decl.(type) {
			case *ast.EntityDecl:
				if hint, ok := implicitModuleHint(moduleOfEntity(schema, d.Name), d.NamePos, d.Name); ok {
					hints = append(hints, hint)
				}
			case *ast.MessageDecl:
				if hint, ok := implicitModuleHint(moduleOfMessage(schema, d.Name), d.NamePos, d.Name); ok {
					hints = append(hints, hint)
				}
			case *ast.ServiceDecl:
				if hint, ok := implicitModuleHint(moduleOfService(schema, d.Name), d.NamePos, d.Name); ok {
					hints = append(hints, hint)
				}
			default:
			}
		}
	}

	if settings.ErrorCaseHints {
		hints = append(hints, errorCaseHints(file)...)
	}

	if settings.ParamNames {
		hints = append(hints, paramNameHints(schema, file)...)
	}

	return hints
}

// errorCaseHints annotates every declared errors: {...} case name with its
// resolved HTTP status and gRPC name (resolver.ValidErrorCodes, the same
// fixed vocabulary the resolver validates against) -- unlike the module hint
// above, useful regardless of module structure; Settings.ErrorCaseHints can
// switch the whole family off.
// A name the resolver would reject as unrecognized is silently skipped: the
// resolver's own diagnostic already covers that, and there is no resolved
// code to annotate it with.
func errorCaseHints(file *ast.File) []protocol.InlayHint {
	var hints []protocol.InlayHint

	walkErrorCases(file, func(name string, pos diag.Position) {
		code, ok := resolver.ValidErrorCodes[name]
		if !ok {
			return
		}

		hints = append(hints, protocol.InlayHint{
			Position: afterIdent(pos, name),
			Label:    protocol.String(fmt.Sprintf(" (%d %s)", code.HTTPStatus(), code.GRPCName())),
		})
	})

	return hints
}

// paramNameHints annotates positional call arguments with their resolved
// `name:` parameter labels: for every schedule `dispatch: Job(...)` call it
// shows the job's parameter names ahead of each positional argument.
//
// Saga steps are deliberately absent from the walk, not an oversight: a
// step's execute/compensate options are bare `Service.RPC` references
// (parseRPCRef rejects `(` with a diagnostic), so the grammar has no call
// site there to annotate. Should the grammar ever gain step arguments, the
// shared paramNameHintsForCall helper below takes them unchanged.
//
// Parameter names come from the resolved ir.Job.Params, so a nil schema (or
// a dispatch naming no known job) yields no hints rather than guesses.
// Settings.ParamNames gates the whole family, off by default.
func paramNameHints(schema *ir.Schema, file *ast.File) []protocol.InlayHint {
	if schema == nil || file == nil {
		return nil
	}

	var hints []protocol.InlayHint

	walkDispatchCalls(file, func(call *ast.CallValue) {
		params := findJobParams(schema, call.Name)
		if len(params) == 0 {
			return
		}

		hints = append(hints, paramNameHintsForCall(params, call)...)
	})

	return hints
}

// walkDispatchCalls calls fn once for every schedule dispatch call in the
// file, in declaration order -- the call-argument counterpart of
// walkErrorCases, which walks every rpc's errors set the same way. A
// dispatch that is not a call (mid-edit) is skipped.
func walkDispatchCalls(file *ast.File, fn func(call *ast.CallValue)) {
	for _, decl := range flattenDecls(file) {
		sched, ok := decl.(*ast.ScheduleDecl)
		if !ok {
			continue
		}

		call, ok := sched.Dispatch.(*ast.CallValue)
		if !ok {
			continue
		}

		fn(call)
	}
}

// findJobParams returns the resolved parameter list of the job named name,
// or nil when no module declares it.
func findJobParams(schema *ir.Schema, name string) []*ir.Param {
	if schema == nil || name == "" {
		return nil
	}

	for _, module := range schema.Modules {
		for _, job := range module.Jobs {
			if job.Name == name {
				return job.Params
			}
		}
	}

	return nil
}

// paramNameHintsForCall renders one `name:` hint per positional argument of
// call, anchored at the argument's start so the editor shows the label
// ahead of the value. Two rust-analyzer-style noise heuristics apply: an
// argument that already carries a `name:` label is never annotated, and a
// single-argument call whose value identifier case-insensitively equals the
// parameter name (dispatch: Send(user) for a (user: string) job) is left
// alone -- the name is already visible. Positional arguments past the end
// of the parameter list, and empty parameter names, are skipped rather than
// guessed at.
func paramNameHintsForCall(params []*ir.Param, call *ast.CallValue) []protocol.InlayHint {
	if call == nil {
		return nil
	}

	var args []*ast.Arg

	for _, arg := range call.Args {
		if arg != nil {
			args = append(args, arg)
		}
	}

	var hints []protocol.InlayHint

	positional := 0

	for _, arg := range args {
		if arg.Name != "" {
			continue
		}

		paramName := ""
		if positional < len(params) && params[positional] != nil {
			paramName = params[positional].Name
		}

		positional++

		if paramName == "" {
			continue
		}

		if len(args) == 1 {
			if ident, ok := arg.Value.(*ast.IdentValue); ok && strings.EqualFold(ident.Name, paramName) {
				continue
			}
		}

		hints = append(hints, protocol.InlayHint{
			Position: lspPosition(arg.Pos),
			Label:    protocol.String(paramName + ":"),
			Kind:     protocol.InlayHintKindParameter,
		})
	}

	return hints
}

// schemaHasNamedModule reports whether schema resolved at least one module
// besides the implicit public sentinel (Name == "").
func schemaHasNamedModule(schema *ir.Schema) bool {
	if schema == nil {
		return false
	}

	for _, module := range schema.Modules {
		if module.Name != "" {
			return true
		}
	}

	return false
}

// moduleOfEntity, moduleOfMessage and moduleOfService look a declaration's
// resolved owning module up by name across the whole schema -- the
// inlay-hint-specific counterpart of definition.go's findEntity/findMessage
// (which this file does not reuse, since those return the whole IR node
// rather than just the module, and adding a service lookup there would edit
// a file this task must not touch).
func moduleOfEntity(schema *ir.Schema, name string) *ir.Module {
	if schema == nil || name == "" {
		return nil
	}

	for _, module := range schema.Modules {
		for _, entity := range module.Entities {
			if entity.Name == name {
				return entity.Module
			}
		}
	}

	return nil
}

func moduleOfMessage(schema *ir.Schema, name string) *ir.Module {
	if schema == nil || name == "" {
		return nil
	}

	for _, module := range schema.Modules {
		for _, msg := range module.Messages {
			if msg.Name == name {
				return msg.Module
			}
		}
	}

	return nil
}

func moduleOfService(schema *ir.Schema, name string) *ir.Module {
	if schema == nil || name == "" {
		return nil
	}

	for _, module := range schema.Modules {
		for _, svc := range module.Services {
			if svc.Name == name {
				return svc.Module
			}
		}
	}

	return nil
}

// implicitModuleHint builds the "(module: public)" hint for one declaration
// when its resolved module is the implicit public sentinel. module == nil
// covers both "schema didn't resolve" and "name not found" -- either way
// there is nothing trustworthy to annotate.
func implicitModuleHint(module *ir.Module, namePos diag.Position, name string) (protocol.InlayHint, bool) {
	if module == nil || module.Name != "" {
		return protocol.InlayHint{}, false
	}

	return protocol.InlayHint{Position: afterIdent(namePos, name), Label: protocol.String(" (module: public)")}, true
}

// afterIdent returns the position immediately following an identifier that
// starts at p, in LSP's 0-based line/character coordinates.
func afterIdent(p diag.Position, ident string) protocol.Position {
	end := lspPosition(p)
	end.Character += uint32(len(ident)) //nolint:gosec // len is never negative

	return end
}
