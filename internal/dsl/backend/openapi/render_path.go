package openapi

import (
	nethttp "net/http"
	"strconv"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// operation is a minimal OpenAPI 3.0.3 Operation Object, restricted to the
// fields this backend populates. x-roles and x-permission are vendor
// extensions documenting rpc.Auth.Roles and rpc.Permission — concepts
// OpenAPI has no native representation for — rather than silently dropping
// them, matching the proto backend's own convention of explicitly naming
// unmapped concepts.
type operation struct {
	OperationID string                `json:"operationId,omitempty"`
	Parameters  []*parameter          `json:"parameters,omitempty"`
	RequestBody *requestBody          `json:"requestBody,omitempty"`
	Responses   map[string]*response  `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
	XRoles      []string              `json:"x-roles,omitempty"`
	XPermission *xPermission          `json:"x-permission,omitempty"`
}

type parameter struct {
	In       string        `json:"in"`
	Name     string        `json:"name"`
	Required bool          `json:"required"`
	Schema   *schemaObject `json:"schema"`
}

type requestBody struct {
	Content map[string]*mediaType `json:"content"`
}

type mediaType struct {
	Schema *schemaObject `json:"schema"`
}

type response struct {
	Description string                `json:"description"`
	Content     map[string]*mediaType `json:"content,omitempty"`
}

type xPermission struct {
	Check      string `json:"check"`
	Resource   string `json:"resource,omitempty"`
	OwnerField string `json:"owner_field,omitempty"`
}

// httpPathParams scans an HTTP path for "{name}" placeholders, returning
// the set of placeholder names found. This is a small, deliberately
// duplicated equivalent of resolver.pathParams (resolver_service.go:160)
// — that function is unexported to package resolver and this is read-only
// re-derivation of already-resolver-validated data, not a shared
// invariant. Malformed input (unbalanced/empty braces) can't reach this
// backend since the resolver rejects it before a schema is ever resolved,
// so this variant just skips anything that doesn't parse cleanly rather
// than reporting malformed.
func httpPathParams(path string) map[string]bool {
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

// findHTTPTransport returns the HTTPTransport among transports, if any.
func findHTTPTransport(transports []ir.Transport) (ir.HTTPTransport, bool) {
	for _, t := range transports {
		if h, ok := t.(ir.HTTPTransport); ok {
			return h, true
		}
	}

	return ir.HTTPTransport{}, false
}

// buildOperation renders one rpc (which must have an HTTP transport) into
// its OpenAPI operation, registering any component schemas it needs (the
// response entity, and a synthesized request schema for POST/PUT/PATCH RPCs
// with non-path params) on d.
func buildOperation(svc *ir.Service, rpc *ir.Operation, http ir.HTTPTransport, d *docBuilder) *operation {
	op := &operation{
		OperationID: svc.Name + "_" + rpc.Name,
		Responses:   map[string]*response{},
	}

	placeholders := httpPathParams(http.Path)

	var pathParams, rest []*ir.Param

	for _, p := range rpc.Params {
		if placeholders[p.Name] {
			pathParams = append(pathParams, p)
		} else {
			rest = append(rest, p)
		}
	}

	for _, p := range pathParams {
		op.Parameters = append(op.Parameters, &parameter{
			In:       "path",
			Name:     p.Name,
			Required: true,
			Schema:   d.paramSchema(p),
		})
	}

	switch http.Method {
	case nethttp.MethodGet, nethttp.MethodDelete:
		for _, p := range rest {
			op.Parameters = append(op.Parameters, &parameter{
				In:       "query",
				Name:     p.Name,
				Required: true,
				Schema:   d.paramSchema(p),
			})
		}
	default: // POST, PUT, PATCH
		if len(rest) > 0 {
			reqName := d.addRequestSchema(rpc.Name+"Request", svc.Module, rest)
			op.RequestBody = &requestBody{
				Content: map[string]*mediaType{
					"application/json": {Schema: &schemaObject{Ref: "#/components/schemas/" + reqName}},
				},
			}
		}
	}

	if rpc.Paginated {
		op.Parameters = append(op.Parameters,
			&parameter{In: "query", Name: "cursor", Required: false, Schema: &schemaObject{Type: "string"}},
			&parameter{In: "query", Name: "limit", Required: false, Schema: &schemaObject{Type: "integer", Format: "int32"}},
		)
	}

	respName := d.addTypeRefSchema(rpc.Returns)

	var respSchema *schemaObject
	if rpc.Paginated {
		respSchema = renderPaginatedResponseSchema(respName)
	} else {
		respSchema = &schemaObject{Ref: "#/components/schemas/" + respName}
	}

	op.Responses["200"] = &response{
		Description: "OK",
		Content: map[string]*mediaType{
			"application/json": {Schema: respSchema},
		},
	}

	addErrorResponses(op, rpc.Errors)

	if rpc.Auth != nil && rpc.Auth.Required {
		op.Security = []map[string][]string{{"bearerAuth": {}}}

		if len(rpc.Auth.Roles) > 0 {
			op.XRoles = rpc.Auth.Roles
		}
	}

	if rpc.Permission != nil {
		xp := &xPermission{Check: rpc.Permission.Check}

		if rpc.Permission.Resource != nil {
			xp.Resource = rpc.Permission.Resource.Name
		}

		if rpc.Permission.OwnerField != nil {
			xp.OwnerField = rpc.Permission.OwnerField.Name
		}

		op.XPermission = xp
	}

	return op
}

// addErrorResponses populates op.Responses with one entry per distinct HTTP
// status among errs. The 16-code error vocabulary has real, deliberate
// status collisions per Google's own canonical mapping (400 shared by
// invalid_argument/failed_precondition/out_of_range, 409 by
// already_exists/aborted, 500 by unknown/internal/data_loss), so multiple
// declared codes mapping to the same status are merged into ONE response
// whose Description joins every matching code's "CODE_NAME" (or
// "CODE_NAME: message" when a message was given) with "; ", in declaration
// order — never a last-write-wins overwrite.
func addErrorResponses(op *operation, errs []*ir.ErrorCase) {
	if len(errs) == 0 {
		return
	}

	var order []int

	parts := map[int][]string{}

	for _, e := range errs {
		status := e.Code.HTTPStatus()

		if _, ok := parts[status]; !ok {
			order = append(order, status)
		}

		part := e.Code.GRPCName()
		if e.Message != "" {
			part += ": " + e.Message
		}

		parts[status] = append(parts[status], part)
	}

	for _, status := range order {
		op.Responses[strconv.Itoa(status)] = &response{Description: strings.Join(parts[status], "; ")}
	}
}
