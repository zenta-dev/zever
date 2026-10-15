package gogen

import (
	"fmt"
	"strings"
)

// renderService renders "<module>/service.go": one Go interface per
// ir.Service, one method per operation. THIS INTERFACE IS THE BUSINESS-LOGIC
// EXTENSION POINT (Phase 7): there is no separate mechanism -- a developer
// implements it in their own package, and the generated router.go/grpc.go
// call it. This file is always regenerated and must never contain business
// logic itself.
//
// Every method's request parameter and return value is a pointer to the
// real protobuf message type types.go aliases in for that operation (e.g.
// "*CreateTaskRequest"/"*Task") -- a breaking change from this backend's
// earlier plain-struct-by-value signature, made so the identical value can
// flow unchanged through both the HTTP router (protojson) and the real
// gRPC service adapter (grpc.go) with no conversion step anywhere.
func renderService(pkg string, data moduleModel) string {
	var body strings.Builder

	if needsPagination(data) {
		body.WriteString(paginationPrelude)
	}

	for i, svc := range data.Services {
		if i > 0 {
			body.WriteString("\n")
		}

		fmt.Fprintf(&body, "// %s is the business-logic extension point generated for the %s service\n", svc.Name, svc.Name)
		body.WriteString("// declared in the zen schema.\n//\n")
		fmt.Fprintf(&body, "// Implement this interface in your own package (never in this file -- it is\n")
		body.WriteString("// always regenerated and must never contain business logic) and pass your\n")
		fmt.Fprintf(&body, "// implementation to Register%sRoutes (router.go) for HTTP and\n", svc.Name)
		fmt.Fprintf(&body, "// New%sGRPCServer (grpc.go) for gRPC -- both wire the identical\n", svc.Name)
		body.WriteString("// implementation to both transports.\n")
		fmt.Fprintf(&body, "type %s interface {\n", svc.Name)

		for _, op := range svc.Operations {
			if op.Paginated {
				fmt.Fprintf(&body, "\t%s(ctx context.Context, req *%s, cursor string, limit int32) (*%s, error)\n",
					op.Name, op.RequestType, op.ReturnType)

				continue
			}

			fmt.Fprintf(&body, "\t%s(ctx context.Context, req *%s) (*%s, error)\n", op.Name, op.RequestType, op.ReturnType)
		}

		body.WriteString("}\n")
	}

	var b strings.Builder

	b.WriteString(generatedHeader)
	fmt.Fprintf(&b, "// Package %s holds the generated service interfaces for the %s module.\n", pkg, pkg)
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	imports := map[string]bool{importContext: true}
	if needsPagination(data) {
		imports[pkgApperror] = true
	}

	writeGroupedImportBlock(&b, imports, nil)
	b.WriteString("\n")
	b.WriteString(body.String())

	return b.String()
}

// needsPagination reports whether any operation in data is paginated, i.e.
// whether the limit helper below must be emitted.
func needsPagination(data moduleModel) bool {
	for _, svc := range data.Services {
		for _, op := range svc.Operations {
			if op.Paginated {
				return true
			}
		}
	}

	return false
}

// paginationPrelude declares the shared pagination bounds helper every
// paginated Service method relies on, emitted once per service.go when at
// least one operation is paginated. The cursor is a plain string: handlers
// pass it through untouched and implementations must treat it as an opaque
// token, never parse it. Everything emitted here is unexported so it can
// never collide with a user-declared message or entity name (those are
// PascalCase exported aliases in types.go); a former exported Cursor type
// and a generic Page[T] were removed for that reason (Page[T] was unused by
// any signature: responses are the protobuf <Op>Response messages).
//
// normalizePageLimit is the single bounds policy, called by BOTH the HTTP
// router and the gRPC adapter so the transports cannot drift: zero (client
// said nothing) becomes defaultPageLimit, a value above maxPageLimit is
// clamped to it, and a negative value is rejected as InvalidArgument before
// the service is ever called.
const paginationPrelude = `// Pagination limit bounds for every paginated operation in this module.
const (
	// defaultPageLimit applies when the client sends no limit.
	defaultPageLimit int32 = 20
	// maxPageLimit caps the client-supplied limit; larger values are
	// clamped to it.
	maxPageLimit int32 = 100
)

// normalizePageLimit applies the module's pagination bounds to a
// client-supplied limit, shared by the HTTP and gRPC transports: zero
// becomes defaultPageLimit, values above maxPageLimit are clamped, and a
// negative value is an InvalidArgument error.
func normalizePageLimit(limit int32) (int32, error) {
	switch {
	case limit < 0:
		return 0, apperror.New(apperror.InvalidArgument, "invalid parameter \"limit\"")
	case limit == 0:
		return defaultPageLimit, nil
	case limit > maxPageLimit:
		return maxPageLimit, nil
	default:
		return limit, nil
	}
}

`
