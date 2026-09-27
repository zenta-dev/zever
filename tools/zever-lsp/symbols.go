package main

import (
	"sort"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

// scalarNames maps resolved scalar types back to their DSL spelling.
var scalarNames = map[ir.ScalarType]string{
	ir.TUUID:      "uuid",
	ir.TString:    "string",
	ir.TInt32:     "int32",
	ir.TInt64:     "int64",
	ir.TFloat32:   "float32",
	ir.TFloat64:   "float64",
	ir.TBool:      "bool",
	ir.TTimestamp: "timestamp",
	ir.TDate:      "date",
	ir.TBytes:     "bytes",
	ir.TJSON:      "json",
	ir.TEnum:      "enum",
}

// scalarName renders a scalar type as it appears in source.
func scalarName(t ir.ScalarType) string {
	if name, ok := scalarNames[t]; ok {
		return name
	}

	return "unknown"
}

// fieldTypeName renders a full field type, including enum members.
func fieldTypeName(t ir.FieldType) string {
	name := scalarName(t.Scalar)
	if t.Scalar != ir.TEnum || len(t.EnumValues) == 0 {
		return name
	}

	var sb strings.Builder

	sb.WriteString(name)
	sb.WriteByte('(')

	for i, v := range t.EnumValues {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(v)
	}

	sb.WriteByte(')')

	return sb.String()
}

// relationKindName renders a resolved relation kind as its DSL keyword.
func relationKindName(k ir.RelationKind) string {
	switch k {
	case ir.HasMany:
		return "has_many"
	case ir.HasOne:
		return "has_one"
	case ir.BelongsTo:
		return "belongs_to"
	case ir.ManyToMany:
		return "many_to_many"
	default:
		return "relation"
	}
}

// astRelationKindName renders an unresolved relation kind as its DSL keyword.
func astRelationKindName(k ast.RelationKind) string {
	switch k {
	case ast.HasMany:
		return "has_many"
	case ast.HasOne:
		return "has_one"
	case ast.BelongsTo:
		return "belongs_to"
	case ast.ManyToMany:
		return "many_to_many"
	default:
		return "relation"
	}
}

// symbolsFromSchema builds the documentSymbol tree for one file out of the
// fully resolved schema, which is the richest source available.
func symbolsFromSchema(schema *ir.Schema, path string) protocol.DocumentSymbolSlice {
	if schema == nil {
		return nil
	}

	symbols := make(protocol.DocumentSymbolSlice, 0)

	for _, module := range schema.Modules {
		for _, entity := range module.Entities {
			if entity.Pos.File != path {
				continue
			}

			symbols = append(symbols, entitySymbol(entity))
		}

		for _, service := range module.Services {
			if service.Pos.File != path {
				continue
			}

			symbols = append(symbols, serviceSymbol(service))
		}

		for _, job := range module.Jobs {
			if job.Pos.File != path {
				continue
			}

			symbols = append(symbols, leafSymbol(job.Name, protocol.SymbolKindFunction,
				"queue "+job.Queue, identRange(job.Pos, job.Name)))
		}

		for _, schedule := range module.Schedules {
			if schedule.Pos.File != path {
				continue
			}

			symbols = append(symbols, leafSymbol(schedule.Name, protocol.SymbolKindEvent,
				schedule.Cron, identRange(schedule.Pos, schedule.Name)))
		}
	}

	return symbols
}

// entitySymbol builds the document symbol for one resolved entity, with its fields and relations as children.
func entitySymbol(entity *ir.Entity) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(entity.Fields)+len(entity.Relations))

	for _, field := range entity.Fields {
		children = append(children, leafSymbol(field.Name, protocol.SymbolKindField,
			fieldTypeName(field.Type), identRange(field.Pos, field.Name)))
	}

	for _, rel := range entity.Relations {
		detail := relationKindName(rel.Kind)
		if rel.Target != nil {
			detail += " " + rel.Target.Name
		}

		children = append(children, leafSymbol(rel.FieldName, protocol.SymbolKindField,
			detail, identRange(rel.Pos, rel.FieldName)))
	}

	return containerSymbol(entity.Name, protocol.SymbolKindClass, "entity",
		identRange(entity.Pos, entity.Name), children)
}

// serviceSymbol builds the document symbol for one resolved service, with its RPCs as children.
func serviceSymbol(service *ir.Service) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(service.Operations))

	for _, rpc := range service.Operations {
		detail := ""
		if rpc.Returns != nil {
			detail = "-> " + rpc.Returns.Name()
		}

		children = append(children, leafSymbol(rpc.Name, protocol.SymbolKindMethod,
			detail, identRange(rpc.Pos, rpc.Name)))
	}

	return containerSymbol(service.Name, protocol.SymbolKindInterface, "service",
		identRange(service.Pos, service.Name), children)
}

// symbolsFromAST is the best-effort fallback used while the workspace does
// not resolve: it reports whatever the parser managed to recover.
func symbolsFromAST(file *ast.File) protocol.DocumentSymbolSlice {
	symbols := make(protocol.DocumentSymbolSlice, 0)

	for _, decl := range flattenDecls(file) {
		switch d := decl.(type) {
		case *ast.EntityDecl:
			symbols = append(symbols, astEntitySymbol(d))
		case *ast.ServiceDecl:
			symbols = append(symbols, astServiceSymbol(d))
		case *ast.JobDecl:
			symbols = append(symbols, leafSymbol(d.Name, protocol.SymbolKindFunction,
				d.Queue, identRange(d.NamePos, d.Name)))
		case *ast.ScheduleDecl:
			symbols = append(symbols, leafSymbol(d.Name, protocol.SymbolKindEvent,
				d.Cron, identRange(d.NamePos, d.Name)))
		}
	}

	return symbols
}

// astEntitySymbol builds the document symbol for one parsed entity declaration, with its fields and relations as children.
func astEntitySymbol(entity *ast.EntityDecl) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(entity.Fields)+len(entity.Relations))

	for _, field := range entity.Fields {
		detail := ""
		if field.Type != nil {
			detail = field.Type.Name
		}

		children = append(children, leafSymbol(field.Name, protocol.SymbolKindField,
			detail, identRange(field.NamePos, field.Name)))
	}

	for _, rel := range entity.Relations {
		children = append(children, leafSymbol(rel.FieldName, protocol.SymbolKindField,
			astRelationKindName(rel.Kind)+" "+rel.Target, identRange(rel.FieldNamePos, rel.FieldName)))
	}

	return containerSymbol(entity.Name, protocol.SymbolKindClass, "entity",
		identRange(entity.NamePos, entity.Name), children)
}

// astServiceSymbol builds the document symbol for one parsed service declaration, with its RPCs as children.
func astServiceSymbol(service *ast.ServiceDecl) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(service.RPCs))

	for _, rpc := range service.RPCs {
		detail := ""
		if rpc.Returns != "" {
			detail = "-> " + rpc.Returns
		}

		children = append(children, leafSymbol(rpc.Name, protocol.SymbolKindMethod,
			detail, identRange(rpc.NamePos, rpc.Name)))
	}

	return containerSymbol(service.Name, protocol.SymbolKindInterface, "service",
		identRange(service.NamePos, service.Name), children)
}

// leafSymbol builds a childless document symbol anchored at rng.
func leafSymbol(name string, kind protocol.SymbolKind, detail string, rng protocol.Range) protocol.DocumentSymbol {
	symbol := protocol.DocumentSymbol{
		Name:           name,
		Kind:           kind,
		Range:          rng,
		SelectionRange: rng,
	}

	if detail != "" {
		symbol.Detail = &detail
	}

	return symbol
}

// containerSymbol builds a document symbol with children, widening its range to enclose them.
func containerSymbol(
	name string,
	kind protocol.SymbolKind,
	detail string,
	rng protocol.Range,
	children []protocol.DocumentSymbol,
) protocol.DocumentSymbol {
	symbol := leafSymbol(name, kind, detail, rng)

	// The container's range must enclose every child, otherwise editors will
	// not nest them in the outline.
	full := rng

	for _, child := range children {
		if child.Range.End.Line > full.End.Line {
			full.End = child.Range.End
		}
	}

	symbol.Range = full
	symbol.Children = children

	return symbol
}

// workspaceSymbols returns every entity/field/job/schedule/service/RPC across
// every file the workspace knows about, filtered by a case-insensitive
// substring match against query (empty query returns everything, per the
// workspace/symbol spec's convention).
//
// The result is a flat SymbolInformation list, the document-symbol response's
// legacy sibling that workspace/symbol still speaks.
func workspaceSymbols(schema *ir.Schema, files map[string]string, query string) protocol.SymbolInformationSlice {
	var all protocol.SymbolInformationSlice

	if schema != nil {
		all = flatSymbolsFromSchema(schema)
	} else {
		paths := make([]string, 0, len(files))
		for path := range files {
			paths = append(paths, path)
		}

		sort.Strings(paths)

		for _, path := range paths {
			file, _ := parser.New(path, []byte(files[path])).ParseFile()
			// Proof: Parser.ParseFile always returns a non-nil *ast.File (it
			// allocates the file up front and returns it alongside
			// diagnostics), so the old nil-file guard was dead.

			all = append(all, flatSymbolsFromAST(path, file)...)
		}
	}

	return filterSymbolsByQuery(all, query)
}

// filterSymbolsByQuery keeps only the symbols whose name contains query,
// case-insensitively. An empty query matches everything.
func filterSymbolsByQuery(symbols protocol.SymbolInformationSlice, query string) protocol.SymbolInformationSlice {
	if query == "" {
		return symbols
	}

	needle := strings.ToLower(query)
	out := make(protocol.SymbolInformationSlice, 0, len(symbols))

	for _, sym := range symbols {
		if strings.Contains(strings.ToLower(sym.Name), needle) {
			out = append(out, sym)
		}
	}

	return out
}

// flatSymbolsFromSchema builds a flat, workspace-wide symbol list out of the
// fully resolved schema, which is the richest source available.
func flatSymbolsFromSchema(schema *ir.Schema) protocol.SymbolInformationSlice {
	if schema == nil {
		return nil
	}

	symbols := make(protocol.SymbolInformationSlice, 0)

	for _, module := range schema.Modules {
		for _, entity := range module.Entities {
			symbols = append(symbols, flatSymbol(entity.Name, protocol.SymbolKindClass, "", entity.Pos))

			for _, field := range entity.Fields {
				symbols = append(symbols, flatSymbol(field.Name, protocol.SymbolKindField, entity.Name, field.Pos))
			}

			for _, rel := range entity.Relations {
				symbols = append(symbols, flatSymbol(rel.FieldName, protocol.SymbolKindField, entity.Name, rel.Pos))
			}
		}

		for _, service := range module.Services {
			symbols = append(symbols, flatSymbol(service.Name, protocol.SymbolKindInterface, "", service.Pos))

			for _, rpc := range service.Operations {
				symbols = append(symbols, flatSymbol(rpc.Name, protocol.SymbolKindMethod, service.Name, rpc.Pos))
			}
		}

		for _, job := range module.Jobs {
			symbols = append(symbols, flatSymbol(job.Name, protocol.SymbolKindFunction, "", job.Pos))
		}

		for _, schedule := range module.Schedules {
			symbols = append(symbols, flatSymbol(schedule.Name, protocol.SymbolKindEvent, "", schedule.Pos))
		}
	}

	return symbols
}

// flatSymbolsFromAST is the best-effort fallback used while the workspace
// does not resolve: it reports whatever the parser managed to recover from
// one file, flattened with a URI per entry rather than nested.
func flatSymbolsFromAST(path string, file *ast.File) protocol.SymbolInformationSlice {
	symbols := make(protocol.SymbolInformationSlice, 0)

	for _, decl := range flattenDecls(file) {
		switch d := decl.(type) {
		case *ast.EntityDecl:
			symbols = append(symbols, flatSymbolAt(d.Name, protocol.SymbolKindClass, "", d.NamePos, path))

			for _, field := range d.Fields {
				symbols = append(symbols,
					flatSymbolAt(field.Name, protocol.SymbolKindField, d.Name, field.NamePos, path))
			}

			for _, rel := range d.Relations {
				symbols = append(symbols,
					flatSymbolAt(rel.FieldName, protocol.SymbolKindField, d.Name, rel.FieldNamePos, path))
			}
		case *ast.ServiceDecl:
			symbols = append(symbols, flatSymbolAt(d.Name, protocol.SymbolKindInterface, "", d.NamePos, path))

			for _, rpc := range d.RPCs {
				symbols = append(symbols, flatSymbolAt(rpc.Name, protocol.SymbolKindMethod, d.Name, rpc.NamePos, path))
			}
		case *ast.JobDecl:
			symbols = append(symbols, flatSymbolAt(d.Name, protocol.SymbolKindFunction, "", d.NamePos, path))
		case *ast.ScheduleDecl:
			symbols = append(symbols, flatSymbolAt(d.Name, protocol.SymbolKindEvent, "", d.NamePos, path))
		}
	}

	return symbols
}

// flatSymbol builds one workspace symbol from a resolved position, which already carries its own file.
func flatSymbol(name string, kind protocol.SymbolKind, container string, pos diag.Position) protocol.SymbolInformation {
	return flatSymbolAt(name, kind, container, pos, pos.File)
}

// flatSymbolAt builds one workspace symbol anchored at pos in path.
func flatSymbolAt(
	name string,
	kind protocol.SymbolKind,
	container string,
	pos diag.Position,
	path string,
) protocol.SymbolInformation {
	symbol := protocol.SymbolInformation{
		BaseSymbolInformation: protocol.BaseSymbolInformation{
			Name: name,
			Kind: kind,
		},
		Location: protocol.Location{
			URI:   pathToURI(path),
			Range: identRange(pos, name),
		},
	}

	if container != "" {
		symbol.ContainerName = &container
	}

	return symbol
}
