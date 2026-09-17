package gogen

import (
	"fmt"
	"sort"
	"strings"
)

// renderGRPC renders "<module>/grpc.go": a thin adapter type per service,
// connecting a hand-written Service implementation to the REAL
// "<Service>Server" gRPC interface protogogen generates for the identical
// schema. Unlike the earlier phase (before protogogen existed), this file
// fakes nothing: <Service>GRPCServer embeds
// pb.Unimplemented<Service>Server (protoc-gen-go-grpc's own
// forward-compatibility embed) and its methods have exactly the signature
// pb.<Service>Server declares, proven by a compile-time
// "var _ pb.<Service>Server = (*<Service>GRPCServer)(nil)" assertion --
// there is no separate, gogen-only request/response shape left to keep in
// sync with the real one. Every returned error is mapped through
// gogenGRPCError. This file contains no business logic and is always
// regenerated.
func renderGRPC(pkg string, data moduleModel) string {
	var body strings.Builder

	anyPolicy := needsAuthzImports(data)

	// Every validate<Op>Request function is emitted here, not in router.go:
	// every ir.Operation always carries a gRPC transport (HTTP is optional),
	// so this is the one file guaranteed to see every operation in the
	// module. router.go, in the same generated package, simply calls the
	// function this file defines for any operation that also has an http:
	// transport.
	validateImports := map[string]bool{}

	for i, svc := range data.Services {
		if i > 0 {
			body.WriteString("\n")
		}

		renderGRPCServer(&body, svc, data.PBAlias)
	}

	for _, svc := range data.Services {
		for _, op := range svc.Operations {
			if !opNeedsValidate(op) {
				continue
			}

			renderValidateFunc(&body, op, validateImports)
		}
	}

	renderPolicies(&body, data)

	var b strings.Builder

	b.WriteString(generatedHeader)
	fmt.Fprintf(&b, "// Package %s holds the generated gRPC server wiring for the %s module.\n", pkg, pkg)
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	b.WriteString("import (\n")
	fmt.Fprintf(&b, "\t%q\n", importContext)
	fmt.Fprintf(&b, "\t%q\n\n", importErrors)

	if len(validateImports) > 0 {
		paths := make([]string, 0, len(validateImports))
		for imp := range validateImports {
			paths = append(paths, imp)
		}

		sort.Strings(paths)

		for _, imp := range paths {
			fmt.Fprintf(&b, "\t%q\n", imp)
		}

		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "\t%q\n", pkgGRPCCodes)
	fmt.Fprintf(&b, "\t%q\n\n", pkgGRPCStat)
	fmt.Fprintf(&b, "\t%q\n", pkgApperror)

	if anyPolicy {
		fmt.Fprintf(&b, "\t%q\n", pkgAuthz)
	}

	b.WriteString("\n")
	fmt.Fprintf(&b, "\t%s %q\n", data.PBAlias, data.PBImportPath)
	b.WriteString(")\n\n")
	b.WriteString(body.String())
	b.WriteString(grpcHelpers)

	return b.String()
}

// renderPolicies emits one package-level "var <Svc><Op>Policy = authz.Policy{...}"
// literal for every operation across every service in data that declares an
// auth: or permission: option, plus GRPCPolicies(), the per-module map of
// every such policy keyed by the real generated FullMethodName constant
// protogogen's *_grpc.pb.go emits for that method.
//
// This lives in grpc.go (not router.go) because it is grpc.go that needs
// the resulting map (GRPCPolicies feeds a single grpc.Server-wide
// interceptor covering every operation, HTTP-exposed or not -- gRPC
// exposure is implied for every operation per ir.Operation's own doc
// comment); router.go, in the same generated package, simply references
// the same <Svc><Op>Policy vars for the (usually smaller) subset of
// operations that are also HTTP-exposed.
//
// Per-module (not a single project-wide combined map): gogen renders one
// independent Go package per module, and GRPCPolicies() can only aggregate
// what is visible inside its own package -- combining every module's
// policies into one grpc.Server-wide map is therefore necessarily a job
// for whoever constructs that single grpc.Server (the project's scaffolded
// main.go), by calling each imported module package's own GRPCPolicies()
// and merging the results. That mirrors how Register<Service>Routes is
// already per-module/per-service and left to the caller to invoke once per
// module against one shared router.Router.
func renderPolicies(b *strings.Builder, data moduleModel) {
	var mapEntries strings.Builder

	hasAny := false

	for _, svc := range data.Services {
		for _, op := range svc.Operations {
			if !op.needsPolicy() {
				continue
			}

			hasAny = true

			varName := policyVarName(svc.Name, op.Name)

			fmt.Fprintf(b, "// %s is the compile-time authz.Policy for %s.%s, generated\n", varName, svc.Name, op.Name)
			fmt.Fprintf(b, "// from its auth:/permission: schema declaration.\n")
			fmt.Fprintf(b, "var %s = authz.Policy{\n", varName)
			writePolicyFields(b, op)
			b.WriteString("}\n\n")

			fmt.Fprintf(&mapEntries, "\t\t%s.%s_%s_FullMethodName: %s,\n", data.PBAlias, svc.Name, op.Name, varName)
		}
	}

	if !hasAny {
		fmt.Fprintf(b, "// GRPCPolicies returns the authz.Policy for every gRPC method this module's\n")
		fmt.Fprintf(b, "// services declare an auth:/permission: requirement for, keyed by the real\n")
		fmt.Fprintf(b, "// generated FullMethodName. No operation in this module declares one, so\n")
		fmt.Fprintf(b, "// this always returns an empty map -- authz.UnaryServerInterceptor treats a\n")
		fmt.Fprintf(b, "// method with no map entry as unrestricted, matching this module's schema.\n")
		b.WriteString("func GRPCPolicies() map[string]authz.Policy {\n\treturn map[string]authz.Policy{}\n}\n\n")

		return
	}

	b.WriteString("// GRPCPolicies returns the authz.Policy for every gRPC method this module's\n")
	b.WriteString("// services declare an auth:/permission: requirement for, keyed by the real\n")
	b.WriteString("// generated FullMethodName constant (the exact string\n")
	b.WriteString("// grpc.UnaryServerInfo.FullMethod carries for that method at runtime).\n")
	b.WriteString("// Merge every generated module package's GRPCPolicies() into one map before\n")
	b.WriteString("// passing it to authz.UnaryServerInterceptor: one interceptor, attached once\n")
	b.WriteString("// per grpc.Server, must cover every service registered on it.\n")
	b.WriteString("func GRPCPolicies() map[string]authz.Policy {\n")
	b.WriteString("\treturn map[string]authz.Policy{\n")
	b.WriteString(mapEntries.String())
	b.WriteString("\t}\n}\n\n")
}

// writePolicyFields emits the authz.Policy struct literal field
// assignments for op's declared auth:/permission: options -- the exact
// field mapping documented on authz.Policy itself.
func writePolicyFields(b *strings.Builder, op opModel) {
	if op.Auth != nil {
		if op.Auth.Required {
			b.WriteString("\tAuthRequired: true,\n")
		}

		if len(op.Auth.Roles) > 0 {
			b.WriteString("\tRoles: []string{")

			for i, role := range op.Auth.Roles {
				if i > 0 {
					b.WriteString(", ")
				}

				fmt.Fprintf(b, "%q", role)
			}

			b.WriteString("},\n")
		}
	}

	if op.Permission != nil {
		fmt.Fprintf(b, "\tPermissionCheck: %q,\n", op.Permission.Check)

		if op.Permission.Resource != nil {
			fmt.Fprintf(b, "\tResourceType: %q,\n", op.Permission.Resource.Name)
		}
	}
}

// needsAuthzImports reports whether any operation across data's services
// declares an auth:/permission: requirement, i.e. whether router.go needs
// to import authz to wrap the corresponding HTTP handlers.
func needsAuthzImports(data moduleModel) bool {
	for _, svc := range data.Services {
		for _, op := range svc.Operations {
			if op.needsPolicy() {
				return true
			}
		}
	}

	return false
}

func renderGRPCServer(b *strings.Builder, svc serviceModel, pbAlias string) {
	fmt.Fprintf(b, "// %sGRPCServer adapts a %s implementation to the real %s.%sServer\n", svc.Name, svc.Name, pbAlias, svc.Name)
	b.WriteString("// gRPC interface protogogen generates for this schema, mapping every\n")
	b.WriteString("// returned error through gogenGRPCError.\n")
	fmt.Fprintf(b, "type %sGRPCServer struct {\n", svc.Name)
	fmt.Fprintf(b, "\t%s.Unimplemented%sServer\n\n", pbAlias, svc.Name)
	fmt.Fprintf(b, "\tsvc %s\n}\n\n", svc.Name)

	fmt.Fprintf(b, "// New%sGRPCServer returns a %sGRPCServer delegating to svc.\n", svc.Name, svc.Name)
	fmt.Fprintf(b, "func New%sGRPCServer(svc %s) *%sGRPCServer {\n", svc.Name, svc.Name, svc.Name)
	fmt.Fprintf(b, "\treturn &%sGRPCServer{svc: svc}\n}\n\n", svc.Name)

	fmt.Fprintf(b, "// var _ asserts %sGRPCServer genuinely implements the real generated\n", svc.Name)
	fmt.Fprintf(b, "// %s.%sServer interface, not just a same-shaped lookalike.\n", pbAlias, svc.Name)
	fmt.Fprintf(b, "var _ %s.%sServer = (*%sGRPCServer)(nil)\n\n", pbAlias, svc.Name, svc.Name)

	for _, op := range svc.Operations {
		fmt.Fprintf(b, "func (g *%sGRPCServer) %s(ctx context.Context, req *%s) (*%s, error) {\n",
			svc.Name, op.Name, op.RequestType, op.ReturnType)

		if opNeedsValidate(op) {
			fmt.Fprintf(b, "\tif err := validate%sRequest(req); err != nil {\n", op.Name)
			b.WriteString("\t\treturn nil, gogenGRPCError(err)\n\t}\n\n")
		}

		if op.Paginated {
			b.WriteString("\t// NOTE: protogogen's synthesized request message carries no cursor/limit\n")
			b.WriteString("\t// fields (Phase 2 does not extend proto request-message synthesis for\n")
			b.WriteString("\t// pagination) -- a gRPC caller always gets the first page today; an HTTP\n")
			b.WriteString("\t// caller gets full cursor/limit support via query parameters, see router.go.\n")
			fmt.Fprintf(b, "\tresp, err := g.svc.%s(ctx, req, \"\", 0)\n", op.Name)
		} else {
			fmt.Fprintf(b, "\tresp, err := g.svc.%s(ctx, req)\n", op.Name)
		}

		b.WriteString("\tif err != nil {\n\t\treturn nil, gogenGRPCError(err)\n\t}\n\n")
		b.WriteString("\treturn resp, nil\n}\n\n")
	}
}

// grpcHelpers are the shared error-mapping helpers every generated gRPC
// method in this file calls; emitted once per grpc.go regardless of how
// many services/operations it holds.
const grpcHelpers = `// gogenGRPCError maps err to a gRPC status: an *apperror.Error maps via
// gogenGRPCCode(Code); any other error maps to codes.Internal with a fixed
// "internal error" message, never err.Error() -- a service bug or panic
// value must not leak internals to the client.
func gogenGRPCError(err error) error {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return status.Error(gogenGRPCCode(appErr.Code()), appErr.Message())
	}

	return status.Error(codes.Internal, "internal error")
}

// gogenGRPCCode maps an apperror.ErrorCode to its canonical grpc-go codes.Code.
func gogenGRPCCode(c apperror.ErrorCode) codes.Code {
	switch c {
	case apperror.Cancelled:
		return codes.Canceled
	case apperror.Unknown:
		return codes.Unknown
	case apperror.InvalidArgument:
		return codes.InvalidArgument
	case apperror.DeadlineExceeded:
		return codes.DeadlineExceeded
	case apperror.NotFound:
		return codes.NotFound
	case apperror.AlreadyExists:
		return codes.AlreadyExists
	case apperror.PermissionDenied:
		return codes.PermissionDenied
	case apperror.ResourceExhausted:
		return codes.ResourceExhausted
	case apperror.FailedPrecondition:
		return codes.FailedPrecondition
	case apperror.Aborted:
		return codes.Aborted
	case apperror.OutOfRange:
		return codes.OutOfRange
	case apperror.Unimplemented:
		return codes.Unimplemented
	case apperror.Internal:
		return codes.Internal
	case apperror.Unavailable:
		return codes.Unavailable
	case apperror.DataLoss:
		return codes.DataLoss
	case apperror.Unauthenticated:
		return codes.Unauthenticated
	default:
		return codes.Unknown
	}
}
`
