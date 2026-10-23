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

// symbolKindParameter is the SymbolKind used for rpc/job parameter
// children. LSP 3.18 defines no Parameter symbol kind (the closest,
// TypeParameter, names generic type parameters rather than value bindings),
// so parameters surface as Variable children -- the kind for a named value
// binding -- with the parameter's type in the detail.
const symbolKindParameter = protocol.SymbolKindVariable

// paramTypeName renders one resolved rpc/job parameter's type as it appears
// in the outline: the referenced entity/message name for a Ref param, else
// the scalar (or inline enum) spelling, the same rendering fields use.
func paramTypeName(param *ir.Param) string {
	if param == nil {
		return ""
	}

	if param.Ref != nil {
		return param.Ref.Name()
	}

	return fieldTypeName(param.Type)
}

// paramChildren builds one parameter child per resolved rpc/job parameter.
func paramChildren(params []*ir.Param) []protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(params))

	for _, param := range params {
		if param == nil {
			continue
		}

		children = append(children, leafSymbol(param.Name, symbolKindParameter,
			paramTypeName(param), identRange(param.Pos, param.Name)))
	}

	return children
}

// astParamTypeName renders one parsed parameter's declared type name, or ""
// when the parser dropped it mid-edit.
func astParamTypeName(param *ast.ParamDecl) string {
	if param == nil || param.Type == nil {
		return ""
	}

	return param.Type.Name
}

// astParamChildren builds one parameter child per parsed rpc/job parameter,
// the AST fallback's counterpart of paramChildren.
func astParamChildren(params []*ast.ParamDecl) []protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(params))

	for _, param := range params {
		if param == nil {
			continue
		}

		children = append(children, leafSymbol(param.Name, symbolKindParameter,
			astParamTypeName(param), identRange(param.NamePos, param.Name)))
	}

	return children
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

			symbols = append(symbols, jobSymbol(job))
		}

		for _, schedule := range module.Schedules {
			if schedule.Pos.File != path {
				continue
			}

			symbols = append(symbols, leafSymbol(schedule.Name, protocol.SymbolKindEvent,
				schedule.Cron, identRange(schedule.Pos, schedule.Name)))
		}

		for _, message := range module.Messages {
			if message.Pos.File != path {
				continue
			}

			symbols = append(symbols, messageSymbol(message))
		}

		for _, saga := range module.Sagas {
			if saga.Pos.File != path {
				continue
			}

			symbols = append(symbols, sagaSymbol(saga))
		}

		for _, enum := range module.Enums {
			if enum.Pos.File != path {
				continue
			}

			symbols = append(symbols, enumSymbol(enum))
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
		children = append(children, rpcSymbol(rpc))
	}

	return containerSymbol(service.Name, protocol.SymbolKindInterface, "service",
		identRange(service.Pos, service.Name), children)
}

// rpcSymbol builds the document symbol for one resolved rpc, with its
// parameters as children. A parameterless rpc stays a leaf, the same shape
// it had before parameters surfaced.
func rpcSymbol(rpc *ir.Operation) protocol.DocumentSymbol {
	detail := ""
	if rpc.Returns != nil {
		detail = "-> " + rpc.Returns.Name()
	}

	params := paramChildren(rpc.Params)
	if len(params) == 0 {
		return leafSymbol(rpc.Name, protocol.SymbolKindMethod,
			detail, identRange(rpc.Pos, rpc.Name))
	}

	return containerSymbol(rpc.Name, protocol.SymbolKindMethod,
		detail, identRange(rpc.Pos, rpc.Name), params)
}

// jobSymbol builds the document symbol for one resolved job, with its
// parameters as children. A parameterless job stays a leaf, the same shape
// it had before parameters surfaced.
func jobSymbol(job *ir.Job) protocol.DocumentSymbol {
	params := paramChildren(job.Params)
	if len(params) == 0 {
		return leafSymbol(job.Name, protocol.SymbolKindFunction,
			"queue "+job.Queue, identRange(job.Pos, job.Name))
	}

	return containerSymbol(job.Name, protocol.SymbolKindFunction,
		"queue "+job.Queue, identRange(job.Pos, job.Name), params)
}

// messageSymbol builds the document symbol for one resolved message, with its fields as children.
func messageSymbol(message *ir.Message) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(message.Fields))

	for _, field := range message.Fields {
		children = append(children, leafSymbol(field.Name, protocol.SymbolKindField,
			fieldTypeName(field.Type), identRange(field.Pos, field.Name)))
	}

	return containerSymbol(message.Name, protocol.SymbolKindStruct, "message",
		identRange(message.Pos, message.Name), children)
}

// sagaSymbol builds the document symbol for one resolved saga, with its
// steps as children. A step's range anchors on the `step` keyword: the
// resolved step records no position for its own name, so the keyword is the
// honest anchor the same way a leaf symbol's range is its safest span.
func sagaSymbol(saga *ir.Saga) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(saga.Steps))

	for _, step := range saga.Steps {
		children = append(children, leafSymbol(step.Name, protocol.SymbolKindFunction,
			sagaStepDetail(sagaStepExecuteNames(step.Execute),
				sagaStepRefNames(step.Compensate), step.Pivot),
			identRange(step.Pos, "step")))
	}

	return containerSymbol(saga.Name, protocol.SymbolKindEvent, "saga",
		identRange(saga.Pos, saga.Name), children)
}

// sagaStepExecuteNames renders a step's forward reference as the
// "Service.RPC" text its execute option carries, or "" when the resolved
// pointers are missing (the resolver guarantees them, and a "" falls through
// to the "?" detail rendering).
func sagaStepExecuteNames(ref ir.ServiceRPC) string {
	if ref.Service == nil || ref.RPC == nil {
		return ""
	}

	return ref.Service.Name + "." + ref.RPC.Name
}

// sagaStepRefNames renders a resolved step reference as the "Service.RPC"
// text a step's option carries, or "" when the step declares none.
func sagaStepRefNames(ref *ir.ServiceRPC) string {
	if ref == nil || ref.Service == nil || ref.RPC == nil {
		return ""
	}

	return ref.Service.Name + "." + ref.RPC.Name
}

// sagaStepDetail renders one step's outline detail line: its forward and
// optional reverse references, plus the pivot marker when set.
func sagaStepDetail(execute, compensate string, pivot bool) string {
	detail := "execute " + execute
	if execute == "" {
		detail += "?"
	}

	if compensate != "" {
		detail += ", compensate " + compensate
	}

	if pivot {
		detail += " (pivot)"
	}

	return detail
}

// enumSymbol builds the document symbol for one named enum. The resolved IR
// records no per-value positions, so the enum has no children here -- the
// AST fallback (astEnumSymbol) is where values surface with their spans.
func enumSymbol(enum *ir.Enum) protocol.DocumentSymbol {
	return leafSymbol(enum.Name, protocol.SymbolKindEnum, "enum",
		identRange(enum.Pos, enum.Name))
}

// symbolsFromAST is the best-effort fallback used while the workspace does
// not resolve: it reports whatever the parser managed to recover.
func symbolsFromAST(file *ast.File) protocol.DocumentSymbolSlice {
	symbols := make(protocol.DocumentSymbolSlice, 0)

	for _, decl := range flattenDecls(file) {
		switch d := decl.(type) {
		case *ast.EntityDecl:
			symbols = append(symbols, astEntitySymbol(d))
		case *ast.MessageDecl:
			symbols = append(symbols, astMessageSymbol(d))
		case *ast.EnumDecl:
			symbols = append(symbols, astEnumSymbol(d))
		case *ast.SagaDecl:
			symbols = append(symbols, astSagaSymbol(d))
		case *ast.ServiceDecl:
			symbols = append(symbols, astServiceSymbol(d))
		case *ast.JobDecl:
			symbols = append(symbols, astJobSymbol(d))
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

		params := astParamChildren(rpc.Params)
		if len(params) == 0 {
			children = append(children, leafSymbol(rpc.Name, protocol.SymbolKindMethod,
				detail, identRange(rpc.NamePos, rpc.Name)))

			continue
		}

		children = append(children, containerSymbol(rpc.Name, protocol.SymbolKindMethod,
			detail, identRange(rpc.NamePos, rpc.Name), params))
	}

	return containerSymbol(service.Name, protocol.SymbolKindInterface, "service",
		identRange(service.NamePos, service.Name), children)
}

// astJobSymbol builds the document symbol for one parsed job declaration,
// with its parameters as children. A parameterless job stays a leaf, the
// same shape it had before parameters surfaced.
func astJobSymbol(job *ast.JobDecl) protocol.DocumentSymbol {
	params := astParamChildren(job.Params)
	if len(params) == 0 {
		return leafSymbol(job.Name, protocol.SymbolKindFunction,
			job.Queue, identRange(job.NamePos, job.Name))
	}

	return containerSymbol(job.Name, protocol.SymbolKindFunction,
		job.Queue, identRange(job.NamePos, job.Name), params)
}

// astMessageSymbol builds the document symbol for one parsed message declaration, with its fields as children.
func astMessageSymbol(message *ast.MessageDecl) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(message.Fields))

	for _, field := range message.Fields {
		detail := ""
		if field.Type != nil {
			detail = field.Type.Name
		}

		children = append(children, leafSymbol(field.Name, protocol.SymbolKindField,
			detail, identRange(field.NamePos, field.Name)))
	}

	return containerSymbol(message.Name, protocol.SymbolKindStruct, "message",
		identRange(message.NamePos, message.Name), children)
}

// astEnumSymbol builds the document symbol for one parsed named enum, with
// its values as children -- the AST records each value's position, which the
// resolved IR carries only fruitlessly.
func astEnumSymbol(enum *ast.EnumDecl) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(enum.Values))

	for i, val := range enum.Values {
		var pos diag.Position
		if i < len(enum.ValuePos) {
			pos = enum.ValuePos[i]
		}

		children = append(children, leafSymbol(val, protocol.SymbolKindEnumMember,
			"", identRange(pos, val)))
	}

	return containerSymbol(enum.Name, protocol.SymbolKindEnum, "enum",
		identRange(enum.NamePos, enum.Name), children)
}

// astSagaSymbol builds the document symbol for one parsed saga, with its
// steps as children. Like the resolved path, a step's range anchors on its
// `step` keyword because the parser records no position for the step's own
// name.
func astSagaSymbol(saga *ast.SagaDecl) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, 0, len(saga.Steps))

	for _, step := range saga.Steps {
		var execute, compensate string
		if step.Execute != nil {
			execute = step.Execute.Service + "." + step.Execute.RPC
		}

		if step.Compensate != nil {
			compensate = step.Compensate.Service + "." + step.Compensate.RPC
		}

		children = append(children, leafSymbol(step.Name, protocol.SymbolKindFunction,
			sagaStepDetail(execute, compensate, step.Pivot),
			identRange(step.Pos, "step")))
	}

	return containerSymbol(saga.Name, protocol.SymbolKindEvent, "saga",
		identRange(saga.NamePos, saga.Name), children)
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

// workspaceSymbols returns every entity/field/job/schedule/service/RPC plus
// every message (and its fields), saga (and its steps) and named enum (and,
// in the AST fallback, its values) across every file the workspace knows
// about, filtered by a case-insensitive substring match against query
// (empty query returns everything, per the workspace/symbol spec's
// convention).
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

		for _, message := range module.Messages {
			symbols = append(symbols, flatSymbol(message.Name, protocol.SymbolKindStruct, "", message.Pos))

			for _, field := range message.Fields {
				symbols = append(symbols, flatSymbol(field.Name, protocol.SymbolKindField, message.Name, field.Pos))
			}
		}

		for _, saga := range module.Sagas {
			symbols = append(symbols, flatSymbol(saga.Name, protocol.SymbolKindEvent, "", saga.Pos))

			for _, step := range saga.Steps {
				symbols = append(symbols, flatSymbol(step.Name, protocol.SymbolKindFunction, saga.Name, step.Pos))
			}
		}

		for _, enum := range module.Enums {
			symbols = append(symbols, flatSymbol(enum.Name, protocol.SymbolKindEnum, "", enum.Pos))
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
		case *ast.MessageDecl:
			symbols = append(symbols, flatSymbolAt(d.Name, protocol.SymbolKindStruct, "", d.NamePos, path))

			for _, field := range d.Fields {
				symbols = append(symbols,
					flatSymbolAt(field.Name, protocol.SymbolKindField, d.Name, field.NamePos, path))
			}
		case *ast.EnumDecl:
			symbols = append(symbols, flatSymbolAt(d.Name, protocol.SymbolKindEnum, "", d.NamePos, path))

			for i, val := range d.Values {
				var pos diag.Position
				if i < len(d.ValuePos) {
					pos = d.ValuePos[i]
				}

				symbols = append(symbols, flatSymbolAt(val, protocol.SymbolKindEnumMember, d.Name, pos, path))
			}
		case *ast.SagaDecl:
			symbols = append(symbols, flatSymbolAt(d.Name, protocol.SymbolKindEvent, "", d.NamePos, path))

			for _, step := range d.Steps {
				symbols = append(symbols,
					flatSymbolAt(step.Name, protocol.SymbolKindFunction, d.Name, step.Pos, path))
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
