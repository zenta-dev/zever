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
				fmt.Fprintf(&body, "\t%s(ctx context.Context, req *%s, cursor Cursor, limit int32) (*%s, error)\n",
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
	writeGroupedImportBlock(&b, map[string]bool{importContext: true}, nil)
	b.WriteString("\n")
	b.WriteString(body.String())

	return b.String()
}

// needsPagination reports whether any operation in data is paginated, i.e.
// whether the Cursor/Page/limit helpers below must be emitted.
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

// paginationPrelude declares the shared pagination vocabulary every
// paginated Service method uses, emitted once per service.go when at least
// one operation is paginated. Cursor is an opaque string: handlers pass it
// through untouched and implementations must treat it as an opaque token,
// never parse it. Page is the typed in-memory shape of one page for
// consumers that prefer it over the protobuf response message. Limits are
// validated by the generated router handler (see render_router.go): zero
// means "client said nothing, service default applies"; a negative limit or
// one above MaxPageLimit is rejected as InvalidArgument before the service
// is ever called.
const paginationPrelude = `// Cursor is an opaque pagination cursor. Handlers pass the client's
// cursor query parameter through as-is; never construct one by hand and
// never parse one -- only the service implementation that minted it may
// interpret it.
type Cursor string

// Pagination limit bounds for every paginated operation in this module.
const (
	// DefaultPageLimit applies when the client sends no limit.
	DefaultPageLimit int32 = 20
	// MaxPageLimit caps the client-supplied limit; larger values are
	// rejected by the generated handler before reaching the service.
	MaxPageLimit int32 = 100
)

// Page is one page of paginated results: the items of this page plus the
// opaque cursor fetching the next one (empty when the page is last).
type Page[T any] struct {
	Items []T
	NextCursor Cursor
}

`
