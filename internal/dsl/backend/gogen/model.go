package gogen

import (
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// pbFieldName converts a schema/proto field identifier (snake_case, e.g.
// "user_id") to the exact Go field name protoc-gen-go's real generator
// would produce for it, so generated code can reference fields on the real
// protobuf message types protogogen emits for the SAME schema. This is a
// deliberate, minimal reimplementation of
// google.golang.org/protobuf/internal/strs.GoCamelCase (an unexported
// package, not importable) -- the exact algorithm protoc-gen-go uses:
// capitalize the first letter of each "_"-or-digit-delimited word and drop
// the underscores, with NO Go-initialism awareness ("user_id" -> "UserId",
// not "UserID", deliberately unlike zenorm's own goName-style
// initialism table). Every gogen-generated reference to a request/response
// field must go through this, never the old initialism-aware convention,
// since the field literally lives on protogogen's generated struct now.
func pbFieldName(s string) string {
	var b strings.Builder

	upperNext := true

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch {
		case c == '_':
			upperNext = true
		case upperNext:
			b.WriteByte(toUpperASCII(c))

			upperNext = false
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}

func toUpperASCII(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}

	return c
}

// pbGoParamType maps a DSL scalar type to the Go type protoc-gen-go
// generates for the equivalent proto field (see proto.protoScalar, which
// this mirrors field-for-field so gogen's request-binding code always
// assigns a value of the exact type the real protobuf message field
// expects), and any import that type needs. Operation params (ir.Param)
// carry no Optional flag today -- only entity fields do (Phase 2's `field:
// type?` syntax applies to entity declarations, not rpc parameter lists) --
// so every param renders as a plain value type.
func pbGoParamType(t ir.FieldType) (goType, importPath string) {
	if t.Scalar == ir.TEnum && t.EnumName != "" {
		// The real protobuf enum type, aliased locally by renderTypes
		// (writeAlias) onto protogogen's generated type of the same name --
		// see paramModel.EnumName.
		return naming.PascalCase(t.EnumName), ""
	}

	switch t.Scalar {
	case ir.TUUID, ir.TString, ir.TDate, ir.TEnum:
		// TDate has no dedicated well-known proto type (proto.protoScalar
		// maps it to plain "string"). An anonymous inline enum(...) param
		// has no shared type to alias, so it is bound as a plain string.
		return "string", ""
	case ir.TInt32:
		return "int32", ""
	case ir.TInt64:
		return "int64", ""
	case ir.TFloat32:
		return "float32", ""
	case ir.TFloat64:
		return "float64", ""
	case ir.TBool:
		return "bool", ""
	case ir.TTimestamp:
		return "*timestamppb.Timestamp", importTimestamppb
	case ir.TBytes:
		return "[]byte", ""
	case ir.TJSON:
		return "*structpb.Struct", importStructpb
	}

	return "any", ""
}

const (
	importTime        = "time"
	importJSON        = "encoding/json"
	importIO          = "io"
	importContext     = "context"
	importErrors      = "errors"
	importHTTP        = "net/http"
	importStrconv     = "strconv"
	importTimestamppb = "google.golang.org/protobuf/types/known/timestamppb"
	importStructpb    = "google.golang.org/protobuf/types/known/structpb"
	importNetMail     = "net/mail"
	importNetURL      = "net/url"
	pkgProto          = "google.golang.org/protobuf/proto"
	pkgProtoJSON      = "google.golang.org/protobuf/encoding/protojson"
	pkgUUID           = "github.com/google/uuid"

	pkgRouter     = "github.com/zenta-dev/zever/router"
	pkgApperror   = "github.com/zenta-dev/zever/apperror"
	pkgGRPCCodes  = "google.golang.org/grpc/codes"
	pkgGRPCStat   = "google.golang.org/grpc/status"
	pkgGRPC       = "google.golang.org/grpc"
	pkgAuthz      = "github.com/zenta-dev/zever/authz"
	pkgAuth       = "github.com/zenta-dev/zever/auth"
	pkgPermission = "github.com/zenta-dev/zever/permission"
)

// paramModel is one operation parameter, resolved once for every renderer.
type paramModel struct {
	Name   string // schema declared name, e.g. "user_id"
	GoName string // the real protobuf message's Go field name, e.g. "UserId" (see pbFieldName)
	GoType string // the real protobuf message field's Go type
	Import string // import GoType needs, if any
	Scalar ir.ScalarType
	// EnumName is set only for a named (non-anonymous) enum param: the
	// PascalCase Go name renderTypes aliases onto protogogen's real
	// generated enum type (see pbGoParamType). Empty for every other
	// scalar, including an anonymous inline enum(...) param.
	EnumName string
	// EnumValues is the enum's declared value list, valid only when
	// EnumName != "" -- emitParamBind uses it to generate a raw-string ->
	// real protobuf enum constant switch.
	EnumValues []string
	IsPathOnly bool            // set per-HTTP-transport at render time (path placeholders differ per transport)
	Validate   []ir.Validation // @validate(...) rules declared on this param, in declaration order

	// IsRef is true when this param references an entity/message (ir.Param
	// Ref set) rather than a scalar -- Scalar/Validate above are meaningless
	// in that case (Scalar's zero value TUUID must never be treated as "this
	// param is a UUID"; renderers must check IsRef first). The real
	// protobuf-message field's Go type is a pointer to the referenced
	// type's own generated message (protojson decodes it as a nested
	// object automatically; see render_router.go's body-decode path), so
	// no HTTP path/query binding is generated for a ref param -- only JSON
	// body decode populates it.
	IsRef bool
	// RefTypeName is the referenced entity/message's bare PascalCase name
	// (e.g. "User"), valid only when IsRef.
	RefTypeName string
	// RefFields lists the referenced type's own fields that carry at least
	// one @validate rule (the v-next "validate-everything-in" opinion
	// guarantees every field of a request-position entity/message has one,
	// so in practice this is every field), valid only when IsRef.
	RefFields []refFieldModel
}

// refFieldModel is one field of an entity/message a ref param points at,
// resolved once so render_validate.go can emit a nested-field check
// (req.<Param>.<Field>) without re-deriving Go naming.
type refFieldModel struct {
	Name     string // schema declared field name
	GoName   string // the real protobuf message's Go field name (see pbFieldName)
	Validate []ir.Validation
	// Optional mirrors ir.Field.Optional: an optional field's real
	// protobuf-message Go field is a pointer (proto3 "optional" field
	// semantics -- see proto.renderField), so a validate check against it
	// must nil-guard and dereference rather than use it as a plain value.
	// Unlike an RPC param (which carries no Optional concept at all -- see
	// paramModel's doc comment), an entity/message field genuinely can be.
	Optional bool
}

// newParamModel resolves one ir.Param into its render model, dispatching on
// whether it references an entity/message (Ref set) or a plain scalar.
func newParamModel(p *ir.Param) paramModel {
	if p.Ref != nil {
		refName := naming.PascalCase(p.Ref.Name())

		var fields []*ir.Field

		switch {
		case p.Ref.Entity != nil:
			fields = p.Ref.Entity.Fields
		case p.Ref.Message != nil:
			fields = p.Ref.Message.Fields
		}

		var refFields []refFieldModel

		for _, f := range fields {
			if len(f.Validate) == 0 {
				continue
			}

			refFields = append(refFields, refFieldModel{
				Name:     f.Name,
				GoName:   pbFieldName(f.Name),
				Validate: f.Validate,
				Optional: f.Optional,
			})
		}

		return paramModel{
			Name:        p.Name,
			GoName:      pbFieldName(p.Name),
			GoType:      "*" + refName,
			IsRef:       true,
			RefTypeName: refName,
			RefFields:   refFields,
		}
	}

	goType, imp := pbGoParamType(p.Type)

	return paramModel{
		Name:       p.Name,
		GoName:     pbFieldName(p.Name),
		GoType:     goType,
		Import:     imp,
		Scalar:     p.Type.Scalar,
		EnumName:   p.Type.EnumName,
		EnumValues: p.Type.EnumValues,
		Validate:   p.Validate,
	}
}

// opModel is everything the renderers need about one ir.Operation. Every
// named type here (RequestType, ItemType, ReturnType) is a bare local
// identifier -- e.g. "CreateTaskRequest", "Task" -- that types.go declares
// as a Go type ALIAS onto the real protobuf message type protogogen
// generates for the same operation/entity (e.g. "type CreateTaskRequest =
// pb.CreateTaskRequest"), so every other renderer can keep referring to a
// short in-package name while the underlying type is, by construction, the
// exact same type protogogen's generated gRPC service interface expects.
type opModel struct {
	Name        string // Pascal operation name, e.g. "CreateTask"
	RequestType string // "<Name>Request"
	// ItemType is the page-item/entity type's bare alias name, e.g. "Task"
	// -- always the same regardless of Paginated.
	ItemType string
	// ReturnType is what the generated Service interface method, gRPC
	// server method, and router handler actually return: ItemType for a
	// plain operation, or "<Name>Response" (protogogen's own synthesized
	// paginated-response message, aliased by types.go) when Paginated.
	ReturnType string
	Paginated  bool
	Params     []paramModel
	HTTP       *ir.HTTPTransport // nil when the operation declares no http: option
	// Auth/Permission are copied directly from ir.Operation so the
	// renderers can translate them into a compile-time authz.Policy
	// literal (see policyVarName/needsPolicy in render_grpc.go). gogen
	// itself may depend on internal/dsl/ir (it is the generator, not the
	// generated output) -- the generated Go source these fields feed never
	// references ir at all, only the runtime authz.Policy struct.
	Auth       *ir.AuthPolicy
	Permission *ir.PermissionCheck
	// SingleRefParam is true when op has exactly one param and it is a ref
	// (Params[0].IsRef) -- mirrors proto backend's singleRefParam exactly
	// (internal/dsl/backend/proto/render_service.go): when true, RequestType
	// IS that ref'd type's own message directly (proto skips synthesizing a
	// "<Op>Request" wrapper around it), so every renderer that addresses a
	// ref param's fields must do so directly on req ("req.<Field>"), never
	// nested under the param's own name ("req.<Param>.<Field>") -- there is
	// no wrapper struct to nest under. See render_validate.go's
	// writeRefParamChecks, the one renderer this actually affects (router.go
	// and grpc.go already address the whole req directly regardless).
	SingleRefParam bool
}

// needsPolicy reports whether op declares any auth/permission requirement
// at all, i.e. whether a <Svc><Op>Policy literal and an Authorize call
// should be generated for it. An operation with neither declared gets no
// policy var and no Authorize call -- exactly as unrestricted today.
func (op opModel) needsPolicy() bool {
	return op.Auth != nil || op.Permission != nil
}

// policyVarName returns the generated package-level authz.Policy variable
// name for one operation of one service, e.g. "UserServiceCreateUserPolicy".
// It is namespaced by service name (not just operation name) since two
// services in the same generated package could otherwise declare
// same-named operations.
func policyVarName(svcName, opName string) string {
	return svcName + opName + "Policy"
}

// serviceModel is everything the renderers need about one ir.Service.
type serviceModel struct {
	Name       string // Pascal service name, e.g. "TaskService"
	Operations []opModel
}

// moduleModel is everything the renderers need about one ir.Module.
type moduleModel struct {
	PkgName      string
	PBImportPath string
	PBAlias      string
	Services     []serviceModel
}

// pbAlias is a fixed import alias for the protogogen-generated message
// package, used instead of its real package name (e.g. "appv1" or
// "zengov1", see pbGoPackage) so every gogen-generated file that imports it
// reads unambiguously with one predictable identifier regardless of module
// name.
const pbAlias = "pb"

// defaultPBImportRoot is the root under which protogogen's generated
// packages are assumed to live, mirroring the proto backend's own hardcoded
// "github.com/zenta-dev/zever/gen" prefix (see proto.moduleNaming, which
// this package's pbGoPackage deliberately duplicates the naming formula
// of -- the same "small deliberately duplicated equivalent" pattern this
// file already uses for pathParamNames). No zever gen package exists yet,
// so this default is override-required for any real project: use
// NewWithPBImportRoot to point at the project's actual protogogen output.
const defaultPBImportRoot = "github.com/zenta-dev/zever/gen"

// pbGoPackage returns the import path of the protogogen-generated package
// for module m, given root (defaultPBImportRoot unless overridden by
// NewWithPBImportRoot) and flat (Backend.pbImportFlat -- see its doc
// comment for why the two formulas differ). flat == false mirrors
// proto.moduleNaming's goPackage formula exactly: the implicit unnamed
// module maps to "<root>/zengov1", a named module "billing" maps to
// "<root>/zengo/billing". flat == true maps to protogogen's real
// "paths=source_relative" output layout instead: the implicit unnamed
// module maps to bare "<root>", a named module "billing" maps to
// "<root>/billing".
func pbGoPackage(m *ir.Module, root string, flat bool) string {
	named := m != nil && m.Name != ""

	switch {
	case flat && !named:
		return root
	case flat && named:
		return root + "/" + m.Name
	case !named:
		return root + "/zengov1"
	default:
		return root + "/zengo/" + m.Name
	}
}

// newModuleModel builds the render model for module m.
func newModuleModel(m *ir.Module, pkg, pbImportRoot string, pbImportFlat bool) moduleModel {
	data := moduleModel{
		PkgName:      pkg,
		PBImportPath: pbGoPackage(m, pbImportRoot, pbImportFlat),
		PBAlias:      pbAlias,
	}

	for _, s := range m.Services {
		data.Services = append(data.Services, newServiceModel(s))
	}

	return data
}

func newServiceModel(s *ir.Service) serviceModel {
	sm := serviceModel{Name: naming.PascalCase(s.Name)}

	for _, op := range s.Operations {
		sm.Operations = append(sm.Operations, newOpModel(op))
	}

	return sm
}

func newOpModel(op *ir.Operation) opModel {
	name := naming.PascalCase(op.Name)

	itemType := "Empty"
	if op.Returns != nil {
		itemType = naming.PascalCase(op.Returns.Name())
	}

	returnType := itemType
	if op.Paginated {
		returnType = name + "Response"
	}

	requestType := name + "Request"

	singleRefParam := len(op.Params) == 1 && op.Params[0].Ref != nil
	if singleRefParam {
		requestType = naming.PascalCase(op.Params[0].Ref.Name())
	}

	m := opModel{
		Name:           name,
		RequestType:    requestType,
		ItemType:       itemType,
		ReturnType:     returnType,
		Paginated:      op.Paginated,
		Auth:           op.Auth,
		Permission:     op.Permission,
		SingleRefParam: singleRefParam,
	}

	for _, p := range op.Params {
		m.Params = append(m.Params, newParamModel(p))
	}

	for _, t := range op.Transports {
		if h, ok := t.(ir.HTTPTransport); ok {
			http := h
			m.HTTP = &http
		}
	}

	return m
}

// pathParamNames scans an HTTP path for "{name}" placeholders, returning the
// set of placeholder names found. This is a small, deliberately duplicated
// equivalent of the openapi backend's own httpPathParams -- read-only
// re-derivation of already-resolver-validated data, not a shared invariant.
func pathParamNames(path string) map[string]bool {
	names := map[string]bool{}
	inBrace := false

	var cur strings.Builder

	for _, r := range path {
		switch r {
		case '{':
			inBrace = true

			cur.Reset()
		case '}':
			if inBrace && cur.Len() > 0 {
				names[cur.String()] = true
			}

			inBrace = false
		default:
			if inBrace {
				cur.WriteRune(r)
			}
		}
	}

	return names
}
