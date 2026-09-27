package resolver

import (
	"strings"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// validHTTPMethods is the fixed v1 set of recognized HTTP verbs.
var validHTTPMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

// ValidErrorCodes is the fixed, gRPC-canonical vocabulary of .zen error
// code keywords accepted by an rpc's errors: option. Exported so tooling
// (zen-lsp's completion/hover/inlay-hint support for errors: { }) can reuse
// this exact vocabulary instead of duplicating it.
var ValidErrorCodes = map[string]ir.ErrorCode{
	"cancelled":           ir.ECancelled,
	"unknown":             ir.EUnknown,
	"invalid_argument":    ir.EInvalidArgument,
	"deadline_exceeded":   ir.EDeadlineExceeded,
	"not_found":           ir.ENotFound,
	"already_exists":      ir.EAlreadyExists,
	"permission_denied":   ir.EPermissionDenied,
	"unauthenticated":     ir.EUnauthenticated,
	"resource_exhausted":  ir.EResourceExhausted,
	"failed_precondition": ir.EFailedPrecondition,
	"aborted":             ir.EAborted,
	"out_of_range":        ir.EOutOfRange,
	"unimplemented":       ir.EUnimplemented,
	"internal":            ir.EInternal,
	"unavailable":         ir.EUnavailable,
	"data_loss":           ir.EDataLoss,
}

// resolveServices is Pass 4's entry point: it resolves every service decl
// (in Pass 0's source-declaration order) and appends each resolved
// *ir.Service to its owning Module's Services slice, same reasoning as Pass
// 1's Module.Entities append.
func resolveServices(
	decls []*ast.ServiceDecl, declModule map[ast.Decl]*ir.Module, entityByName map[string]*ir.Entity,
	messageByName map[string]*ir.Message, enumByName map[string]*ir.Enum,
) diag.List {
	var diags diag.List

	for _, decl := range decls {
		module := declModule[decl]
		if module == nil {
			// Unreachable given Pass -1/0's invariants; guarded defensively.
			continue
		}

		service, d := resolveService(decl, module, entityByName, messageByName, enumByName)
		diags = append(diags, d...)

		module.Services = append(module.Services, service)
	}

	return diags
}

// resolveService resolves one service's RPCs.
func resolveService(
	decl *ast.ServiceDecl, module *ir.Module, entityByName map[string]*ir.Entity,
	messageByName map[string]*ir.Message, enumByName map[string]*ir.Enum,
) (*ir.Service, diag.List) {
	var diags diag.List

	service := &ir.Service{Name: decl.Name, Module: module, Pos: decl.Pos, DocComment: decl.DocComment}

	for _, rd := range decl.RPCs {
		op, d := resolveOperation(decl.Name, rd, module, entityByName, messageByName, enumByName)
		diags = append(diags, d...)

		service.Operations = append(service.Operations, op)
	}

	return service, diags
}

// resolveOperation resolves one rpc declaration: params, Returns (against
// entities or messages, with the same cross-module check as relations), transports
// (gRPC always, HTTP when declared), auth, and permission.
func resolveOperation(
	serviceName string, rd *ast.RPCDecl, module *ir.Module, entityByName map[string]*ir.Entity,
	messageByName map[string]*ir.Message, enumByName map[string]*ir.Enum,
) (*ir.Operation, diag.List) {
	var diags diag.List

	params, d := resolveParams(rd.Params, entityByName, messageByName, enumByName)
	diags = append(diags, d...)

	rpc := &ir.Operation{Name: rd.Name, Params: params, Pos: rd.Pos, DocComment: rd.DocComment}
	rpc.Transports = append(rpc.Transports, ir.GRPCTransport{})

	if ent, ok := entityByName[rd.Returns]; ok {
		rpc.Returns = &ir.TypeRef{Entity: ent}
	} else if msg, ok := messageByName[rd.Returns]; ok {
		rpc.Returns = &ir.TypeRef{Message: msg}
	} else {
		diags = append(diags, diag.Wrap("resolve", rd.ReturnsPos, ErrUnresolvedReference,
			"rpc %s.%s returns unknown type %q", serviceName, rd.Name, rd.Returns))
	}
	// Cross-module return check is performed centrally in CheckCrossModule.

	if rd.HTTP != nil {
		http, d2 := resolveHTTP(rd)
		diags = append(diags, d2...)
		rpc.Transports = append(rpc.Transports, *http)
	}

	if rd.Auth != nil {
		auth, d2 := resolveAuth(rd.Auth)
		diags = append(diags, d2...)
		rpc.Auth = auth
		rpc.AuthDeclared = true
	}

	if rd.Permission != nil {
		perm, d2 := resolvePermission(rd.Permission, entityByName, module)
		diags = append(diags, d2...)
		rpc.Permission = perm
	}

	if len(rd.Errors) > 0 {
		errs, d2 := resolveErrors(rd.Errors)
		diags = append(diags, d2...)
		rpc.Errors = errs
	}

	rpc.Paginated = rd.Paginated

	return rpc, diags
}

// resolveErrors decodes an rpc's errors: {...} set. Each item is either a
// bare *ast.IdentValue (an error code with no message) or a *ast.CallValue
// (a code with exactly one positional string message argument and no named
// args, mirroring resolvePermission's positional-arg handling). Every code
// is validated against the fixed ValidErrorCodes vocabulary, and duplicate
// codes within one set are rejected — a deliberate stricter precedent than
// http/auth/permission's existing "silently keep last" gap, since a set is
// naturally many items rather than one option value.
func resolveErrors(items []ast.Value) ([]*ir.ErrorCase, diag.List) {
	var diags diag.List

	seen := make(map[ir.ErrorCode]bool, len(items))

	cases := make([]*ir.ErrorCase, 0, len(items))

	for _, v := range items {
		switch val := v.(type) {
		case *ast.IdentValue:
			code, ok := ValidErrorCodes[val.Name]
			if !ok {
				diags = append(diags, diag.Wrap("resolve", val.Pos, ErrInvalidOption,
					"unknown error code %q", val.Name))

				continue
			}

			if seen[code] {
				diags = append(diags, diag.Wrap("resolve", val.Pos, ErrInvalidOption,
					"duplicate error code %q in errors: set", val.Name))

				continue
			}

			seen[code] = true
			cases = append(cases, &ir.ErrorCase{Code: code, Pos: val.Pos})
		case *ast.CallValue:
			code, ok := ValidErrorCodes[val.Name]
			if !ok {
				diags = append(diags, diag.Wrap("resolve", val.Pos, ErrInvalidOption,
					"unknown error code %q", val.Name))

				continue
			}

			message, d2, valid := resolveErrorMessageArgs(val)
			diags = append(diags, d2...)

			if !valid {
				continue
			}

			if seen[code] {
				diags = append(diags, diag.Wrap("resolve", val.Pos, ErrInvalidOption,
					"duplicate error code %q in errors: set", val.Name))

				continue
			}

			seen[code] = true
			cases = append(cases, &ir.ErrorCase{Code: code, Message: message, Pos: val.Pos})
		default:
			diags = append(diags, diag.Wrap("resolve", valuePos(v), ErrInvalidOption,
				"errors: item must be a bare error code or code(\"message\")"))
		}
	}

	return cases, diags
}

// resolveErrorMessageArgs validates one errors: call item's args: at most
// one positional string argument (the message) and no named args. valid is
// false when any arg was rejected, so the caller drops the whole item
// rather than recording a partially-resolved case.
func resolveErrorMessageArgs(call *ast.CallValue) (message string, diags diag.List, valid bool) {
	valid = true

	havePositional := false

	for _, arg := range call.Args {
		if arg.Name != "" {
			diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption,
				"error code %q takes no named arguments", call.Name))

			valid = false

			continue
		}

		if havePositional {
			diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption,
				"error code %q takes at most one positional message argument", call.Name))

			valid = false

			continue
		}

		s, ok := arg.Value.(*ast.StringLit)
		if !ok {
			diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption,
				"error code %q message argument must be a string", call.Name))

			valid = false

			continue
		}

		message = s.Value
		havePositional = true
	}

	return message, diags, valid
}

// resolveParams resolves an RPC's or job's parameter list. A param whose
// type name matches a known entity or message (with no type args, mirroring
// how those names are declared) resolves as an ir.Param.Ref (the same
// TypeRef union Operation.Returns already uses in response position) rather
// than a scalar ir.FieldType -- resolveTypeRefOrFieldType is the single
// shared lookup both this and resolveOperation's Returns handling could, in
// principle, reuse; kept local here since Returns resolves against a bare
// string (rd.Returns) rather than a *ast.TypeExpr.
func resolveParams(
	decls []*ast.ParamDecl, entityByName map[string]*ir.Entity, messageByName map[string]*ir.Message, enumByName map[string]*ir.Enum,
) ([]*ir.Param, diag.List) {
	var diags diag.List

	params := make([]*ir.Param, 0, len(decls))

	for _, pd := range decls {
		param, d := resolveParam(pd, entityByName, messageByName, enumByName)
		diags = append(diags, d...)

		if param == nil {
			continue
		}

		params = append(params, param)
	}

	return params, diags
}

// resolveParam resolves one param declaration, preferring an entity/message
// TypeRef match (a bare type name with no arg list) over scalar resolution
// so a param can reference a message/entity by name exactly like Returns
// does. Cross-module reference checking is performed centrally in
// CheckCrossModule, same as Returns.
func resolveParam(
	pd *ast.ParamDecl, entityByName map[string]*ir.Entity, messageByName map[string]*ir.Message, enumByName map[string]*ir.Enum,
) (*ir.Param, diag.List) {
	if pd.Type != nil && len(pd.Type.Args) == 0 {
		if ent, ok := entityByName[pd.Type.Name]; ok {
			return resolveRefParam(pd, &ir.TypeRef{Entity: ent})
		}

		if msg, ok := messageByName[pd.Type.Name]; ok {
			return resolveRefParam(pd, &ir.TypeRef{Message: msg})
		}
	}

	ft, d := resolveFieldType(pd.Type, enumByName)
	if d != nil {
		return nil, diag.List{d}
	}

	var diags diag.List

	param := &ir.Param{Name: pd.Name, Type: ft, Pos: pd.Pos}

	for _, attr := range pd.Attributes {
		if attr.Name != "validate" {
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute,
				"unknown param attribute @%s", attr.Name))

			continue
		}

		vals, d := resolveValidateRules(attr, ft.Scalar)
		param.Validate = append(param.Validate, vals...)
		diags = append(diags, d...)
	}

	return param, diags
}

// resolveRefParam finishes resolving a param already known to reference an
// entity/message (ref). @validate on a ref-typed param itself is rejected:
// per the v-next "validate-everything-in" opinion, validation lives on the
// referenced type's own fields (each of which must carry @validate), not on
// the param that merely names the type.
func resolveRefParam(pd *ast.ParamDecl, ref *ir.TypeRef) (*ir.Param, diag.List) {
	var diags diag.List

	param := &ir.Param{Name: pd.Name, Ref: ref, Pos: pd.Pos}

	for _, attr := range pd.Attributes {
		switch attr.Name {
		case "validate":
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute,
				"cannot declare @validate on param %q of entity/message type %q; add @validate to that type's own fields instead",
				pd.Name, ref.Name()))
		default:
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute,
				"unknown param attribute @%s", attr.Name))
		}
	}

	return param, diags
}

// resolveHTTP resolves an rpc's http: option: method membership and path
// placeholder validation against the rpc's declared params.
func resolveHTTP(rd *ast.RPCDecl) (*ir.HTTPTransport, diag.List) {
	var diags diag.List

	opt := rd.HTTP

	if !validHTTPMethods[opt.Method] {
		diags = append(diags, diag.Wrap("resolve", opt.MethodPos, ErrInvalidOption,
			"http method %q must be one of GET, POST, PUT, PATCH, DELETE", opt.Method))
	}

	names, malformed := pathParams(opt.Path)
	if malformed {
		diags = append(diags, diag.Wrap("resolve", opt.PathPos, ErrInvalidOption,
			"http path %q has an unbalanced or empty {} placeholder", opt.Path))
	} else {
		declared := make(map[string]bool, len(rd.Params))
		for _, p := range rd.Params {
			declared[p.Name] = true
		}

		for _, n := range names {
			if !declared[n] {
				diags = append(diags, diag.Wrap("resolve", opt.PathPos, ErrUnresolvedReference,
					"http path placeholder {%s} does not match any declared rpc param", n))
			}
		}
	}

	return &ir.HTTPTransport{Method: opt.Method, Path: opt.Path, Pos: opt.Pos}, diags
}

// pathParams scans an HTTP path for "{name}" placeholders without regexp.
// malformed reports an unbalanced brace (an unmatched '{' or '}'), a nested
// '{' before the previous one closed, or an empty "{}" placeholder. names
// lists every well-formed placeholder's inner text in left-to-right order
// (not deduplicated: a path may legitimately repeat a param).
func pathParams(path string) (names []string, malformed bool) {
	inBrace := false

	var cur strings.Builder

	for _, r := range path {
		switch r {
		case '{':
			if inBrace {
				return names, true
			}

			inBrace = true

			cur.Reset()
		case '}':
			if !inBrace {
				return names, true
			}

			inBrace = false

			if cur.Len() == 0 {
				return names, true
			}

			names = append(names, cur.String())
		default:
			if inBrace {
				cur.WriteRune(r)
			}
		}
	}

	if inBrace {
		return names, true
	}

	return names, false
}

// resolveAuth decodes an rpc's auth: value. none/absent -> nil (absent is
// handled by the caller not invoking this at all); bare required or
// required(roles: {...}) -> &ir.AuthPolicy{...}; anything else ->
// ErrInvalidOption.
func resolveAuth(v ast.Value) (*ir.AuthPolicy, diag.List) {
	switch val := v.(type) {
	case *ast.IdentValue:
		switch val.Name {
		case "none":
			return nil, nil
		case "required":
			return &ir.AuthPolicy{Required: true, Pos: val.Pos}, nil
		default:
			return nil, diag.List{diag.Wrap("resolve", val.Pos, ErrInvalidOption,
				"auth value %q must be none, required, or required(roles: {...})", val.Name)}
		}
	case *ast.CallValue:
		return resolveAuthCall(val)
	default:
		return nil, diag.List{diag.Wrap("resolve", valuePos(v), ErrInvalidOption, "invalid auth value")}
	}
}

// resolveAuthCall decodes the required(roles: {...}) call shape.
func resolveAuthCall(val *ast.CallValue) (*ir.AuthPolicy, diag.List) {
	if val.Name != "required" {
		return nil, diag.List{diag.Wrap("resolve", val.Pos, ErrInvalidOption, "unknown auth call %q", val.Name)}
	}

	var diags diag.List

	var roles []string

	for _, arg := range val.Args {
		if arg.Name != "roles" {
			diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption, "unknown auth argument %q", arg.Name))
			continue
		}

		set, ok := arg.Value.(*ast.SetLit)
		if !ok {
			diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption, "roles must be a set literal"))
			continue
		}

		roles = append(roles, set.Items...)
	}

	return &ir.AuthPolicy{Required: true, Roles: roles, Pos: val.Pos}, diags
}

// resolvePermission decodes an rpc's permission: value, requiring the shape
// check(<string>, resource: <ident>, owner_field: <ident>) with the
// positional string first and named args in any order. resource is then
// resolved against entities (same cross-module check as Returns) and
// owner_field against that entity's fields, independently.
func resolvePermission(v ast.Value, entityByName map[string]*ir.Entity, module *ir.Module) (*ir.PermissionCheck, diag.List) {
	call, ok := v.(*ast.CallValue)
	if !ok || call.Name != "check" {
		return nil, diag.List{diag.Wrap("resolve", valuePos(v), ErrInvalidOption,
			"permission must be check(<string>, resource: <entity>, owner_field: <field>)")}
	}

	perm := &ir.PermissionCheck{Pos: call.Pos}

	var diags diag.List

	var resourceName, ownerFieldName string

	var resourcePos, ownerFieldPos diag.Position

	haveResource, haveOwnerField, havePositional := false, false, false

	for _, arg := range call.Args {
		switch {
		case arg.Name == "" && !havePositional:
			s, ok := arg.Value.(*ast.StringLit)
			if !ok {
				diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption,
					"permission check() positional argument must be a string"))

				continue
			}

			perm.Check = s.Value
			havePositional = true
		case arg.Name == "":
			diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption,
				"permission check() takes exactly one positional argument"))
		case arg.Name == "resource":
			ident, ok := arg.Value.(*ast.IdentValue)
			if !ok {
				diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption, "permission resource must be an identifier"))
				continue
			}

			resourceName, resourcePos, haveResource = ident.Name, arg.Pos, true
		case arg.Name == "owner_field":
			ident, ok := arg.Value.(*ast.IdentValue)
			if !ok {
				diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption, "permission owner_field must be an identifier"))
				continue
			}

			ownerFieldName, ownerFieldPos, haveOwnerField = ident.Name, arg.Pos, true
		default:
			diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidOption, "unknown permission argument %q", arg.Name))
		}
	}

	if !havePositional {
		diags = append(diags, diag.Wrap("resolve", call.Pos, ErrInvalidOption, "permission check() requires a positional string argument"))
	}

	if !haveResource {
		diags = append(diags, diag.Wrap("resolve", call.Pos, ErrInvalidOption, "permission check() requires a resource argument"))
		return perm, diags
	}

	if !haveOwnerField {
		diags = append(diags, diag.Wrap("resolve", call.Pos, ErrInvalidOption, "permission check() requires an owner_field argument"))
	}

	diags = append(diags, resolvePermissionResource(
		perm, resourceName, resourcePos, ownerFieldName, ownerFieldPos, haveOwnerField, entityByName, module)...)

	return perm, diags
}

// resolvePermissionResource resolves permission.resource against entities
// (with the relation-style cross-module check) and, independently,
// owner_field against that resolved resource's fields.
func resolvePermissionResource(
	perm *ir.PermissionCheck,
	resourceName string, resourcePos diag.Position,
	ownerFieldName string, ownerFieldPos diag.Position, haveOwnerField bool,
	entityByName map[string]*ir.Entity, _ *ir.Module,
) diag.List {
	var diags diag.List

	entity, ok := entityByName[resourceName]
	if !ok {
		diags = append(diags, diag.Wrap("resolve", resourcePos, ErrUnresolvedReference,
			"permission resource %q is not a known entity", resourceName))

		return diags
	}

	perm.Resource = entity

	// Cross-module permission check is performed centrally in CheckCrossModule.

	if !haveOwnerField {
		return diags
	}

	field := entity.FieldByName(ownerFieldName)
	if field == nil {
		diags = append(diags, diag.Wrap("resolve", ownerFieldPos, ErrUnresolvedReference,
			"permission owner_field %q not found on resource entity %s", ownerFieldName, entity.Name))

		return diags
	}

	perm.OwnerField = field

	return diags
}

// valuePos returns a Value's source position by type-switching on every
// concrete ast.Value implementation (its interface method is unexported to
// package ast, so callers outside it must do this).
func valuePos(v ast.Value) diag.Position {
	switch val := v.(type) {
	case *ast.IdentValue:
		return val.Pos
	case *ast.CallValue:
		return val.Pos
	case *ast.StringLit:
		return val.Pos
	case *ast.IntLit:
		return val.Pos
	case *ast.FloatLit:
		return val.Pos
	case *ast.DurationLit:
		return val.Pos
	case *ast.SetLit:
		return val.Pos
	default:
		return diag.Position{}
	}
}
