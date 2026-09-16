package proto

import (
	"fmt"
	nethttp "net/http"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// renderService renders one service's synthesized request messages followed
// by the service block itself (google.api.http and, when present, the
// zengo auth/permission options are rendered per RPC).
func renderService(w *strings.Builder, s *ir.Service, ctx *fileCtx) error {
	for _, r := range s.Operations {
		if singleRefParam(r) == nil {
			if err := renderRequestMessage(w, s, r, ctx); err != nil {
				return err
			}

			w.WriteString("\n")
		}

		if r.Paginated {
			if err := renderResponseMessage(w, s, r, ctx); err != nil {
				return err
			}

			w.WriteString("\n")
		}
	}

	fmt.Fprintf(w, "service %s {\n", naming.PascalCase(s.Name))

	for i, r := range s.Operations {
		if i > 0 {
			w.WriteString("\n")
		}

		renderRPC(w, r, ctx)
	}

	w.WriteString("}\n")

	return nil
}

// singleRefParam returns the operation's sole param's TypeRef when it has
// exactly one param and that param references an entity/message by name
// (p.Ref set), or nil otherwise. gRPC requires exactly one request message
// per RPC; when the DSL author already wrote the RPC as taking one
// message/entity directly (the idiomatic "rpc Foo(req: FooRequest)" shape),
// that referenced type IS the request message -- synthesizing a redundant
// "<RPCName>Request" wrapper around it would, in the common case where the
// author names their message exactly that way (e.g. `rpc SignUp(req:
// SignUpRequest)`), collide by name with the very message being wrapped.
func singleRefParam(r *ir.Operation) *ir.TypeRef {
	if len(r.Params) != 1 {
		return nil
	}

	return r.Params[0].Ref
}

// renderRequestMessage synthesizes and renders an RPC's request message
// (load-bearing: gRPC needs exactly one request message, so every RPC gets
// one regardless of its param count, matching the shape the brief's own
// generated example shows even for a single-param RPC). Fields are numbered
// in declared param order. Not called when singleRefParam(r) != nil -- see
// its doc comment.
func renderRequestMessage(w *strings.Builder, s *ir.Service, r *ir.Operation, ctx *fileCtx) error {
	name := naming.PascalCase(r.Name) + "Request"
	owner := fmt.Sprintf("rpc %s.%s (%s)", s.Name, r.Name, r.Pos)

	if err := ctx.claimMessageName(name, owner); err != nil {
		return err
	}

	fmt.Fprintf(w, "message %s {\n", name)

	var enums []string

	for i, p := range r.Params {
		if p.Ref != nil {
			renderRefField(w, p.Name, p.Ref, i+1)
			continue
		}

		renderField(w, p.Name, p.Type, false, i+1, ctx)

		if p.Type.Scalar == ir.TEnum {
			enumSrc, err := renderNestedEnum(p.Name, p.Type)
			if err != nil {
				return fmt.Errorf("rpc %s.%s: %w", s.Name, r.Name, err)
			}

			enums = append(enums, enumSrc)
		}
	}

	for i, enumSrc := range enums {
		if i == 0 && len(r.Params) > 0 {
			w.WriteString("\n")
		}

		w.WriteString(enumSrc)
	}

	w.WriteString("}\n")

	return nil
}

// renderResponseMessage synthesizes and renders a paginated RPC's response
// message: a `repeated <ItemType> items = 1` field (the page's entity/message type)
// followed by `string next_cursor = 2`, mirroring renderRequestMessage's
// synthesis pattern for the request side. Only called when r.Paginated is
// true -- a non-paginated operation keeps using its Returns type's own
// message directly (see renderRPC), so this never runs for it.
func renderResponseMessage(w *strings.Builder, s *ir.Service, r *ir.Operation, ctx *fileCtx) error {
	name := naming.PascalCase(r.Name) + "Response"
	owner := fmt.Sprintf("rpc %s.%s (%s)", s.Name, r.Name, r.Pos)

	if err := ctx.claimMessageName(name, owner); err != nil {
		return err
	}

	itemType := "google.protobuf.Empty"
	if r.Returns != nil {
		itemType = naming.PascalCase(r.Returns.Name())
	}

	fmt.Fprintf(w, "message %s {\n", name)
	fmt.Fprintf(w, "  repeated %s items = 1;\n", itemType)
	w.WriteString("  string next_cursor = 2;\n")
	w.WriteString("}\n")

	return nil
}

// renderRPC renders one rpc declaration: its request type (the sole ref
// param's own type directly when singleRefParam applies, else the
// synthesized "<RPCName>Request" wrapper), its Returns type (entity or
// message) used directly as the response type, and any http/auth/permission
// options. An RPC with none of the three renders as a bodyless declaration
// ("rpc Foo(...) returns (...);") rather than an empty
// "{}" block.
func renderRPC(w *strings.Builder, r *ir.Operation, ctx *fileCtx) {
	reqName := naming.PascalCase(r.Name) + "Request"
	if ref := singleRefParam(r); ref != nil {
		reqName = naming.PascalCase(ref.Name())
	}

	respName := "google.protobuf.Empty"
	if r.Returns != nil {
		respName = naming.PascalCase(r.Returns.Name())
	}

	if r.Paginated {
		respName = naming.PascalCase(r.Name) + "Response"
	}

	var opts []string

	if http := renderHTTPOption(findHTTPTransport(r.Transports)); http != "" {
		opts = append(opts, http)

		ctx.require(importHTTP)
	}

	if auth := renderAuthOption(r.Auth); auth != "" {
		opts = append(opts, auth)

		ctx.require(importAnnotations)
	}

	if perm := renderPermissionOption(r.Permission); perm != "" {
		opts = append(opts, perm)

		ctx.require(importAnnotations)
	}

	if errs := renderErrorsOption(r.Errors); errs != "" {
		opts = append(opts, errs)

		ctx.require(importAnnotations)
	}

	if len(opts) == 0 {
		fmt.Fprintf(w, "  rpc %s(%s) returns (%s);\n", naming.PascalCase(r.Name), reqName, respName)
		return
	}

	fmt.Fprintf(w, "  rpc %s(%s) returns (%s) {\n", naming.PascalCase(r.Name), reqName, respName)

	for _, opt := range opts {
		fmt.Fprintf(w, "    %s\n", opt)
	}

	w.WriteString("  }\n")
}

// findHTTPTransport returns the HTTPTransport among transports, if any.
func findHTTPTransport(transports []ir.Transport) *ir.HTTPTransport {
	for _, t := range transports {
		if h, ok := t.(ir.HTTPTransport); ok {
			return &h
		}
	}

	return nil
}

// renderHTTPOption renders an rpc's google.api.http option. GET/DELETE take
// only a path; POST/PUT/PATCH always get body: "*" (a more precise
// per-field path-vs-body split is possible but deferred, not silently
// dropped). Returns "" when http is nil.
func renderHTTPOption(http *ir.HTTPTransport) string {
	if http == nil {
		return ""
	}

	verb := strings.ToLower(http.Method)

	switch http.Method {
	case nethttp.MethodGet, nethttp.MethodDelete:
		return fmt.Sprintf("option (google.api.http) = { %s: %q };", verb, http.Path)
	case nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch:
		return fmt.Sprintf("option (google.api.http) = { %s: %q body: \"*\" };", verb, http.Path)
	default:
		// resolveHTTP (Task 8) already rejects any other method before a
		// schema reaches this backend; this default only guards against an
		// impossible value ever reaching Generate.
		return fmt.Sprintf("option (google.api.http) = { %s: %q };", verb, http.Path)
	}
}

// renderAuthOption renders an rpc's zengo.annotations.v1.auth option. auth
// == nil (the resolver's representation of "auth: none" or an absent auth:
// clause) omits the option entirely rather than emitting "required: false"
// — absence itself reads as unauthenticated.
func renderAuthOption(auth *ir.AuthPolicy) string {
	if auth == nil {
		return ""
	}

	if len(auth.Roles) == 0 {
		return fmt.Sprintf("option (zengo.annotations.v1.auth) = { required: %t };", auth.Required)
	}

	quoted := make([]string, len(auth.Roles))
	for i, role := range auth.Roles {
		quoted[i] = fmt.Sprintf("%q", role)
	}

	return fmt.Sprintf("option (zengo.annotations.v1.auth) = { required: %t, roles: [%s] };",
		auth.Required, strings.Join(quoted, ", "))
}

// renderPermissionOption renders an rpc's zengo.annotations.v1.permission
// option. perm == nil (no permission: clause) omits the option entirely.
// resource is rendered as the resolved resource entity's proto message name
// and owner_field as the declared (snake_case) field name, so both values
// line up with identifiers actually present in the generated proto.
func renderPermissionOption(perm *ir.PermissionCheck) string {
	if perm == nil {
		return ""
	}

	parts := []string{fmt.Sprintf("check: %q", perm.Check)}

	if perm.Resource != nil {
		parts = append(parts, fmt.Sprintf("resource: %q", naming.PascalCase(perm.Resource.Name)))
	}

	if perm.OwnerField != nil {
		parts = append(parts, fmt.Sprintf("owner_field: %q", perm.OwnerField.Name))
	}

	return fmt.Sprintf("option (zengo.annotations.v1.permission) = { %s };", strings.Join(parts, ", "))
}

// renderErrorsOption renders an rpc's zengo.annotations.v1.errors option.
// errs == nil (no errors: clause, or an empty errors: {} set) omits the
// option entirely. Each case renders its code via ErrorCode.GRPCName() so
// the wire value is always one of the canonical gRPC names, and its
// message only when one was given.
func renderErrorsOption(errs []*ir.ErrorCase) string {
	if len(errs) == 0 {
		return ""
	}

	cases := make([]string, len(errs))

	for i, e := range errs {
		if e.Message == "" {
			cases[i] = fmt.Sprintf("{ code: %q }", e.Code.GRPCName())
		} else {
			cases[i] = fmt.Sprintf("{ code: %q, message: %q }", e.Code.GRPCName(), e.Message)
		}
	}

	return fmt.Sprintf("option (zengo.annotations.v1.errors) = { cases: [%s] };", strings.Join(cases, ", "))
}
