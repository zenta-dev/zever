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
	b.WriteString("import (\n")
	fmt.Fprintf(&b, "\t%q\n", importContext)
	b.WriteString(")\n\n")
	b.WriteString(body.String())

	return b.String()
}
