package gogen

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// renderRouter renders "<module>/router.go": HTTP routing via the router
// battery's Router/Group interfaces, wired only for operations that declare
// an http: transport. Each handler decodes its request, validates required
// (string-typed) params are present, calls the Service interface method,
// and maps a returned error to an HTTP status via apperror. This file
// contains no business logic and is always regenerated.
//
// Request/response bodies are encoded with protojson (protobuf's canonical
// JSON mapping), not encoding/json: every request/response type here is a
// real protobuf message (aliased by types.go), and protojson is the only
// encoder that respects a protobuf message's field options and oneofs
// correctly. This is a deliberate, accepted breaking change to gogen's wire
// shape: protojson's default field-name casing is lowerCamelCase (e.g.
// "userId"), not this backend's earlier plain-struct casing (e.g.
// "UserID") -- there is no attempt here to preserve the old casing via
// custom protojson.MarshalOptions field-name overrides.
func renderRouter(pkg string, data moduleModel) string {
	imports := map[string]bool{importHTTP: true, importJSON: true, importErrors: true, importIO: true, pkgProtoJSON: true, pkgProto: true}

	withAuthz := needsAuthzImports(data)
	if withAuthz {
		imports[pkgAuthz] = true
		imports[pkgAuth] = true
		imports[pkgPermission] = true
	}

	var body strings.Builder

	for i, svc := range data.Services {
		if i > 0 {
			body.WriteString("\n")
		}

		var httpOps []opModel

		for _, op := range svc.Operations {
			if op.HTTP == nil {
				continue
			}

			httpOps = append(httpOps, op)

			renderHandler(&body, svc.Name, op, imports)
		}

		renderRegisterFunc(&body, svc.Name, httpOps, withAuthz)
	}

	var b strings.Builder

	b.WriteString(generatedHeader)
	fmt.Fprintf(&b, "// Package %s holds the generated HTTP routing for the %s module.\n", pkg, pkg)
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	writeSortedImportBlock(&b, imports)
	fmt.Fprintf(&b, "\t%q\n\n", pkgApperror)
	fmt.Fprintf(&b, "\t%q\n", pkgRouter)

	if needsPBImport(data) {
		fmt.Fprintf(&b, "\t%s %q\n", data.PBAlias, data.PBImportPath)
	}

	b.WriteString(")\n\n")
	b.WriteString(maxRequestBodyBytesConst)
	b.WriteString(body.String())
	b.WriteString(routerHelpers)

	return b.String()
}

// needsPBImport reports whether any HTTP-bound operation in data has a
// named-enum param -- emitParamBind's TEnum branch references the real
// protobuf enum constants (pb.<Type>_<VALUE>) directly, which anonymous
// inline enum(...) params (bound as plain strings) never need.
func needsPBImport(data moduleModel) bool {
	for _, svc := range data.Services {
		for _, op := range svc.Operations {
			if op.HTTP == nil {
				continue
			}

			for _, p := range op.Params {
				if p.EnumName != "" {
					return true
				}
			}
		}
	}

	return false
}

func writeSortedImportBlock(b *strings.Builder, imports map[string]bool) {
	paths := make([]string, 0, len(imports))
	for p := range imports {
		paths = append(paths, p)
	}

	sort.Strings(paths)

	b.WriteString("import (\n")

	for _, p := range paths {
		fmt.Fprintf(b, "\t%q\n", p)
	}
}

// renderHandler renders one operation's http.HandlerFunc factory.
func renderHandler(b *strings.Builder, svcName string, op opModel, imports map[string]bool) {
	pathParams := pathParamNames(op.HTTP.Path)

	var pathP, otherP []paramModel

	for _, p := range op.Params {
		if pathParams[p.Name] {
			pathP = append(pathP, p)
		} else {
			otherP = append(otherP, p)
		}
	}

	fmt.Fprintf(b, "// handle%s%s handles %s %s, calling %s.%s.\n",
		svcName, op.Name, op.HTTP.Method, op.HTTP.Path, svcName, op.Name)
	fmt.Fprintf(b, "func handle%s%s(svc %s) http.HandlerFunc {\n", svcName, op.Name, svcName)
	b.WriteString("\treturn func(w http.ResponseWriter, r *http.Request) {\n")
	b.WriteString("\t\tctx := r.Context()\n\n")
	fmt.Fprintf(b, "\t\treq := &%s{}\n\n", op.RequestType)

	if op.Paginated {
		imports[importStrconv] = true

		b.WriteString("\t\tvar cursor string\n")
		b.WriteString("\t\tvar limit int32\n\n")
		b.WriteString("\t\tcursor = r.URL.Query().Get(\"cursor\")\n")
		b.WriteString("\t\tif raw := r.URL.Query().Get(\"limit\"); raw != \"\" {\n")
		b.WriteString("\t\t\tv, err := strconv.ParseInt(raw, 10, 32)\n")
		b.WriteString("\t\t\tif err != nil {\n")
		b.WriteString("\t\t\t\twriteGogenError(w, apperror.New(apperror.InvalidArgument, \"invalid parameter \\\"limit\\\"\"))\n")
		b.WriteString("\t\t\t\treturn\n\t\t\t}\n\n")
		b.WriteString("\t\t\tlimit = int32(v)\n\t\t}\n\n")
	}

	switch op.HTTP.Method {
	case http.MethodGet, http.MethodDelete:
		for _, p := range pathP {
			emitParamBind(b, "req."+p.GoName, p, fmt.Sprintf("router.Param(r, %q)", p.Name), imports)
		}

		for _, p := range otherP {
			emitParamBind(b, "req."+p.GoName, p, fmt.Sprintf("r.URL.Query().Get(%q)", p.Name), imports)
		}
	default: // POST, PUT, PATCH
		if len(otherP) > 0 {
			b.WriteString("\t\tbody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))\n")
			b.WriteString("\t\tif err != nil {\n")
			b.WriteString("\t\t\tmsg := \"invalid JSON body\"\n\n")
			b.WriteString("\t\t\tvar maxErr *http.MaxBytesError\n")
			b.WriteString("\t\t\tif errors.As(err, &maxErr) {\n")
			b.WriteString("\t\t\t\tmsg = \"request body too large\"\n\t\t\t}\n\n")
			b.WriteString("\t\t\twriteGogenError(w, apperror.New(apperror.InvalidArgument, msg))\n")
			b.WriteString("\t\t\treturn\n\t\t}\n\n")
			b.WriteString("\t\tif err := protojson.Unmarshal(body, req); err != nil {\n")
			b.WriteString("\t\t\twriteGogenError(w, apperror.New(apperror.InvalidArgument, \"invalid JSON body\"))\n")
			b.WriteString("\t\t\treturn\n\t\t}\n\n")

			for _, p := range otherP {
				if !p.IsRef && isStringScalar(p.Scalar) {
					fmt.Fprintf(b, "\t\tif req.%s == \"\" {\n", p.GoName)
					fmt.Fprintf(b, "\t\t\twriteGogenError(w, apperror.New(apperror.InvalidArgument, %q))\n",
						fmt.Sprintf("missing required field %q", p.Name))
					b.WriteString("\t\t\treturn\n\t\t}\n\n")
				}
			}
		}

		for _, p := range pathP {
			emitParamBind(b, "req."+p.GoName, p, fmt.Sprintf("router.Param(r, %q)", p.Name), imports)
		}
	}

	if opNeedsValidate(op) {
		fmt.Fprintf(b, "\t\tif err := validate%sRequest(req); err != nil {\n", op.Name)
		b.WriteString("\t\t\twriteGogenError(w, err)\n\t\t\treturn\n\t\t}\n\n")
	}

	if op.Paginated {
		fmt.Fprintf(b, "\t\tresp, err := svc.%s(ctx, req, cursor, limit)\n", op.Name)
	} else {
		fmt.Fprintf(b, "\t\tresp, err := svc.%s(ctx, req)\n", op.Name)
	}

	b.WriteString("\t\tif err != nil {\n\t\t\twriteGogenError(w, err)\n\t\t\treturn\n\t\t}\n\n")
	b.WriteString("\t\twriteGogenProtoJSON(w, http.StatusOK, resp)\n")
	b.WriteString("\t}\n}\n\n")
}

// isStringScalar reports whether a scalar's real protobuf-message Go field
// type is string (see pbGoParamType), so "" is a meaningful "absent" signal
// minimal presence validation can check.
func isStringScalar(s ir.ScalarType) bool {
	return s == ir.TUUID || s == ir.TString || s == ir.TDate || s == ir.TEnum
}

// emitEnumParamBind emits a switch converting rawExpr's already-bound "raw"
// string variable into one of p's real protobuf enum constants
// (pb.<Type>_<PREFIX>_<VALUE>, matching proto.renderTopLevelEnum's naming
// exactly), returning a 400 for any value not in the enum's declared set.
func emitEnumParamBind(b *strings.Builder, dst string, p paramModel, invalid string) {
	prefix := naming.ScreamingSnake(p.EnumName)

	b.WriteString("\t\tswitch raw {\n")

	for _, v := range p.EnumValues {
		fmt.Fprintf(b, "\t\tcase %q:\n", v)
		fmt.Fprintf(b, "\t\t\t%s = pb.%s_%s_%s\n", dst, p.GoType, prefix, naming.ScreamingSnake(v))
	}

	b.WriteString("\t\tdefault:\n")
	fmt.Fprintf(b, "\t\t\twriteGogenError(w, apperror.New(apperror.InvalidArgument, %q))\n", invalid)
	b.WriteString("\t\t\treturn\n\t\t}\n\n")
}

// emitParamBind emits code assigning dst from the string-producing
// expression rawExpr, converting to p's real protobuf-message field type
// and returning a 400 on a missing or unparsable value. imports is updated
// with any import the conversion needs. A ref (entity/message-typed) param
// has no string-representable path/query binding -- it is only ever
// populated by the JSON body decode in the POST/PUT/PATCH branch above --
// so this is a deliberate no-op for one.
func emitParamBind(b *strings.Builder, dst string, p paramModel, rawExpr string, imports map[string]bool) {
	if p.IsRef {
		return
	}

	missing := fmt.Sprintf("missing required parameter %q", p.Name)
	invalid := fmt.Sprintf("invalid parameter %q", p.Name)

	// Each param binds inside its own block: several params share this
	// handler's scope, and each needs its own "raw"/"v"/"err" without
	// colliding with a sibling param's identically-named locals.
	b.WriteString("\t\t{\n")
	fmt.Fprintf(b, "\t\traw := %s\n", rawExpr)
	b.WriteString("\t\tif raw == \"\" {\n")
	fmt.Fprintf(b, "\t\t\twriteGogenError(w, apperror.New(apperror.InvalidArgument, %q))\n", missing)
	b.WriteString("\t\t\treturn\n\t\t}\n\n")

	switch p.Scalar {
	case ir.TEnum:
		if p.EnumName != "" {
			emitEnumParamBind(b, dst, p, invalid)
		} else {
			fmt.Fprintf(b, "\t\t%s = raw\n\n", dst)
		}
	case ir.TUUID, ir.TString, ir.TDate:
		fmt.Fprintf(b, "\t\t%s = raw\n\n", dst)
	case ir.TBytes:
		fmt.Fprintf(b, "\t\t%s = []byte(raw)\n\n", dst)
	case ir.TJSON:
		imports[importStructpb] = true

		b.WriteString("\t\tv := &structpb.Struct{}\n")
		b.WriteString("\t\tif err := protojson.Unmarshal([]byte(raw), v); err != nil {\n")
		writeParseErrCheckBody(b, invalid)
		fmt.Fprintf(b, "\t\t%s = v\n\n", dst)
	case ir.TInt32:
		imports[importStrconv] = true

		b.WriteString("\t\tv, err := strconv.ParseInt(raw, 10, 32)\n")
		writeParseErrCheck(b, invalid)
		fmt.Fprintf(b, "\t\t%s = int32(v)\n\n", dst)
	case ir.TInt64:
		imports[importStrconv] = true

		b.WriteString("\t\tv, err := strconv.ParseInt(raw, 10, 64)\n")
		writeParseErrCheck(b, invalid)
		fmt.Fprintf(b, "\t\t%s = v\n\n", dst)
	case ir.TFloat32:
		imports[importStrconv] = true

		b.WriteString("\t\tv, err := strconv.ParseFloat(raw, 32)\n")
		writeParseErrCheck(b, invalid)
		fmt.Fprintf(b, "\t\t%s = float32(v)\n\n", dst)
	case ir.TFloat64:
		imports[importStrconv] = true

		b.WriteString("\t\tv, err := strconv.ParseFloat(raw, 64)\n")
		writeParseErrCheck(b, invalid)
		fmt.Fprintf(b, "\t\t%s = v\n\n", dst)
	case ir.TBool:
		imports[importStrconv] = true

		b.WriteString("\t\tv, err := strconv.ParseBool(raw)\n")
		writeParseErrCheck(b, invalid)
		fmt.Fprintf(b, "\t\t%s = v\n\n", dst)
	case ir.TTimestamp:
		imports[importTime] = true
		imports[importTimestamppb] = true

		b.WriteString("\t\tv, err := time.Parse(time.RFC3339, raw)\n")
		writeParseErrCheck(b, invalid)
		fmt.Fprintf(b, "\t\t%s = timestamppb.New(v)\n\n", dst)
	}

	b.WriteString("\t\t}\n\n")
}

func writeParseErrCheck(b *strings.Builder, invalidMsg string) {
	b.WriteString("\t\tif err != nil {\n")
	fmt.Fprintf(b, "\t\t\twriteGogenError(w, apperror.New(apperror.InvalidArgument, %q))\n", invalidMsg)
	b.WriteString("\t\t\treturn\n\t\t}\n")
}

// writeParseErrCheckBody is writeParseErrCheck's body-only variant, for a
// conversion whose "if err != nil {" has already been opened by the caller
// (see the TJSON case in emitParamBind, which needs the structpb.Struct
// value populated in place rather than returned from a helper call).
func writeParseErrCheckBody(b *strings.Builder, invalidMsg string) {
	fmt.Fprintf(b, "\t\t\twriteGogenError(w, apperror.New(apperror.InvalidArgument, %q))\n", invalidMsg)
	b.WriteString("\t\t\treturn\n\t\t}\n\n")
}

// renderRegisterFunc renders Register<Service>Routes, wiring every HTTP
// operation of svc onto r in declaration order. When withAuthz is true (at
// least one operation across the whole module declares auth:/permission:),
// Register<Service>Routes always accepts an auth.Auth/permission.Checker
// pair -- even for a service whose own operations need neither -- so every
// service in a module shares one uniform registration signature regardless
// of which specific operations are guarded; a plain operation is registered
// unwrapped and never touches a or p.
func renderRegisterFunc(b *strings.Builder, svcName string, ops []opModel, withAuthz bool) {
	fmt.Fprintf(b, "// Register%sRoutes wires every HTTP-transport operation of %s onto r,\n", svcName, svcName)
	b.WriteString("// calling svc for each request. An operation with an auth:/permission:\n")
	b.WriteString("// declaration is wrapped in authz.Middleware, enforcing the identical\n")
	b.WriteString("// policy the same operation's gRPC method enforces via GRPCPolicies() --\n")
	b.WriteString("// see authz.Authorize, the single implementation both transports call.\n")

	if withAuthz {
		fmt.Fprintf(b, "func Register%sRoutes(r router.Router, svc %s, a auth.Auth, p permission.Checker) {\n", svcName, svcName)
	} else {
		fmt.Fprintf(b, "func Register%sRoutes(r router.Router, svc %s) {\n", svcName, svcName)
	}

	for _, op := range ops {
		handlerExpr := fmt.Sprintf("handle%s%s(svc)", svcName, op.Name)

		if op.needsPolicy() {
			resourceIDExpr := resourceIDFromRequestExpr(op)
			fmt.Fprintf(b, "\tr.Handle(%q, %q, authz.Middleware(a, p, %s, %s)(%s).ServeHTTP)\n",
				op.HTTP.Method, op.HTTP.Path, policyVarName(svcName, op.Name), resourceIDExpr, handlerExpr)

			continue
		}

		fmt.Fprintf(b, "\tr.Handle(%q, %q, %s)\n", op.HTTP.Method, op.HTTP.Path, handlerExpr)
	}

	b.WriteString("}\n\n")
}

// resourceIDFromRequestExpr renders a "func(*http.Request) string" literal
// resolving a permission-check resource id for op, or the literal "nil"
// when op declares no PermissionCheck (Middleware treats a nil
// extractor as "always empty resourceID", which is exactly right for an
// auth-only operation).
//
// gRPC has no path-param equivalent to derive a resource id from generically
// (see authz.UnaryServerInterceptor's own doc comment, which always passes
// ""), so this HTTP-only heuristic reads the operation's own "{id}" path
// placeholder when present -- the REST convention every operation in this
// codebase's own example schemas already follows for a single-resource
// route (e.g. "/v1/orders/{id}"). An operation whose PermissionCheck needs
// a resource id from anywhere else (a non-"id" path param, a query
// parameter, the request body) is not covered by this heuristic and
// resolves to "": still evaluated by permission.Checker.Can, per
// authz.Authorize's own documented behavior, not silently skipped.
func resourceIDFromRequestExpr(op opModel) string {
	if op.Permission == nil {
		return "nil"
	}

	if pathParamNames(op.HTTP.Path)["id"] {
		return `func(r *http.Request) string { return router.Param(r, "id") }`
	}

	return `func(*http.Request) string { return "" }`
}

// maxRequestBodyBytesConst declares the size cap every POST/PUT/PATCH
// handler in this file enforces via http.MaxBytesReader before buffering a
// request body with io.ReadAll -- without it, an unbounded body is read
// fully into memory before any validation runs, a memory-exhaustion vector
// for any client (or attacker) willing to send a large enough request. 10MB
// matches this module's own precedent for a request-size limit
// (document/local.maxSourceBytes), reused here for consistency.
const maxRequestBodyBytesConst = "const maxRequestBodyBytes = 10 << 20 // 10MB, matches document/local.maxSourceBytes\n\n"

// routerHelpers are the shared JSON/error-writing helpers every generated
// handler in this file calls; emitted once per router.go regardless of how
// many services/operations it holds.
const routerHelpers = `// writeGogenJSON writes v as a plain (non-protobuf) JSON response body with
// the given status -- used only for the generic error shape below, since an
// *apperror.Error is not a protobuf message.
func writeGogenJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeGogenProtoJSON writes msg -- a real protobuf message -- as a
// protojson-encoded response body with the given status. protojson is
// protobuf's canonical JSON mapping: field names render in lowerCamelCase
// (e.g. "userId"), not this backend's earlier plain-struct casing.
func writeGogenProtoJSON(w http.ResponseWriter, status int, msg proto.Message) {
	body, err := protojson.Marshal(msg)
	if err != nil {
		writeGogenJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeGogenError maps err to an HTTP status: an *apperror.Error maps via
// its Code.HTTPStatus(); any other error maps to 500.
func writeGogenError(w http.ResponseWriter, err error) {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		writeGogenJSON(w, appErr.Code().HTTPStatus(), map[string]string{"error": appErr.Message()})
		return
	}

	writeGogenJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}
`
