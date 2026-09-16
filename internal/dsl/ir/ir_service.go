package ir

import "github.com/zenta-dev/zever/internal/dsl/diag"

// Param represents a parameter for an RPC method.
type Param struct {
	Name string    // Parameter name
	Type FieldType // Parameter type; valid only when Ref is nil (scalar param)
	// Ref is non-nil when the param references an Entity or Message by
	// name (request-position usage of a message/entity, mirroring how
	// Operation.Returns references a TypeRef in response position) instead
	// of a scalar FieldType. Exactly one of Type/Ref is meaningful for a
	// given resolved Param: Ref set means Type is the zero value and
	// unused.
	Ref      *TypeRef
	Validate []Validation  // @validate(...) rules declared on this param
	Pos      diag.Position // Source position
}

// IsRef reports whether p references an Entity/Message rather than a
// scalar FieldType.
func (p *Param) IsRef() bool { return p != nil && p.Ref != nil }

// TransportKind identifies which wire transport a Transport value describes.
type TransportKind int

const (
	// TransportHTTP identifies an HTTPTransport.
	TransportHTTP TransportKind = iota
	// TransportGRPC identifies a GRPCTransport.
	TransportGRPC
)

// Transport is one wire transport an Operation is exposed over. Every
// resolved Operation always carries a GRPCTransport, and additionally an
// HTTPTransport when the operation declares an http: option.
type Transport interface {
	Kind() TransportKind
}

// HTTPTransport represents HTTP binding information for an Operation.
type HTTPTransport struct {
	Method string        // HTTP method (GET, POST, PUT, DELETE, etc.)
	Path   string        // HTTP path
	Pos    diag.Position // Source position
}

// Kind implements Transport.
func (HTTPTransport) Kind() TransportKind { return TransportHTTP }

// GRPCTransport marks an Operation as exposed over gRPC. It carries no data:
// gRPC exposure is always implied for every resolved Operation.
type GRPCTransport struct{}

// Kind implements Transport.
func (GRPCTransport) Kind() TransportKind { return TransportGRPC }

// AuthPolicy represents authentication and authorization policy for an RPC.
type AuthPolicy struct {
	Required bool          // Whether authentication is required
	Roles    []string      // Required roles
	Pos      diag.Position // Source position
}

// PermissionCheck represents a permission check for an RPC.
type PermissionCheck struct {
	Check      string        // The permission check expression
	Resource   *Entity       // The resource entity being checked
	OwnerField *Field        // The owner field on the resource
	Pos        diag.Position // Source position
}

// ErrorCase represents one declared error an rpc may return: a fixed-
// vocabulary code with an optional human-readable message ("" = no message
// given).
type ErrorCase struct {
	Code    ErrorCode
	Message string
	Pos     diag.Position // Source position
}

// TypeRef is a union reference for an operation's return type: either an
// Entity or a Message. Exactly one of Entity/Message is non-nil when the
// TypeRef is non-nil.
type TypeRef struct {
	Entity  *Entity
	Message *Message
}

// IsEntity reports whether the return type is an entity.
func (r TypeRef) IsEntity() bool { return r.Entity != nil }

// IsMessage reports whether the return type is a message.
func (r TypeRef) IsMessage() bool { return r.Message != nil }

// Name returns the referenced type's name.
func (r TypeRef) Name() string {
	if r.Entity != nil {
		return r.Entity.Name
	}

	if r.Message != nil {
		return r.Message.Name
	}

	return ""
}

// Module returns the referenced type's owning module.
func (r TypeRef) Module() *Module {
	if r.Entity != nil {
		return r.Entity.Module
	}

	if r.Message != nil {
		return r.Message.Module
	}

	return nil
}

// Operation represents a Remote Procedure Call in a service, exposed over
// one or more Transports (always gRPC, optionally also HTTP).
type Operation struct {
	Name       string
	Params     []*Param
	Returns    *TypeRef
	Transports []Transport
	Auth       *AuthPolicy
	// AuthDeclared is true when the source wrote an auth: clause at all
	// (none, required, or required(roles: {...})), false when the auth:
	// clause was omitted entirely from the RPC declaration. Distinguishes
	// "explicitly public" (auth: none) from "forgot to add auth" for the
	// secure-by-default opinion.
	AuthDeclared bool
	Permission   *PermissionCheck
	Errors       []*ErrorCase
	// Paginated changes the semantic meaning of Returns from "this operation
	// returns exactly one Returns entity" to "this operation returns one
	// page of Returns entities" -- Returns still points at the page's item
	// type; every consumer (backends) checks Paginated to know whether to
	// wrap the response in an {items, next_cursor} shape.
	Paginated bool
	Pos       diag.Position
	// DocComment is the "//" comment written immediately above this rpc's
	// declaration in the source .zen file, or "" if there was none.
	DocComment string
}

// Service represents a service in the schema.
type Service struct {
	Name       string
	Module     *Module // Resolved pointer
	Operations []*Operation
	Pos        diag.Position
	// DocComment is the "//" comment written immediately above this
	// service's declaration in the source .zen file, or "" if there was
	// none.
	DocComment string
}
