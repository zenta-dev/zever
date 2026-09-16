package gogen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// opNeedsValidate reports whether op has at least one param carrying a
// @validate rule (directly, for a scalar param, or on the referenced
// type's fields, for a ref param) i.e. whether a validate<Op>Request
// function should be emitted for it at all, and whether router.go/grpc.go
// should call it.
func opNeedsValidate(op opModel) bool {
	for _, p := range op.Params {
		if len(p.Validate) > 0 {
			return true
		}

		if p.IsRef && len(p.RefFields) > 0 {
			return true
		}
	}

	return false
}

// renderValidateFunc emits "func validate<Op>Request(req *<Op>Request)
// error" for op, checking every @validate rule declared on op's params as
// literal Go conditionals built from compile-time-known ir.Validation
// values -- never reflection or a generic rule-engine loop, matching this
// backend's existing "boring, explicit, compile-time-known" convention
// (e.g. the authz.Policy literals and <op>Errors mappings elsewhere in this
// package).
//
// It is called identically from router.go (right after decode) and
// grpc.go's generated adapter (as the literal first statement, before
// svc.<Op>), so an invalid request is rejected before business logic runs
// on either transport -- the one shared implementation, two callers shape
// this codebase already uses for authz.Authorize.
//
// The caller must have already confirmed opNeedsValidate(op); this function
// does not check it again.
func renderValidateFunc(b *strings.Builder, op opModel, imports map[string]bool) {
	fmt.Fprintf(b, "// validate%sRequest checks every @validate rule declared on %s's params,\n", op.Name, op.Name)
	b.WriteString("// returning the first failing rule (checked in schema declaration order) as\n")
	b.WriteString("// an apperror.InvalidArgument, or nil once every rule passes.\n")
	fmt.Fprintf(b, "func validate%sRequest(req *%s) error {\n", op.Name, op.RequestType)

	for _, p := range op.Params {
		if p.IsRef {
			writeRefParamChecks(b, p, op.SingleRefParam, imports)
			continue
		}

		for _, v := range p.Validate {
			writeValidationCheck(b, "req."+p.GoName, p.Name, v, imports)
		}
	}

	b.WriteString("\treturn nil\n}\n\n")
}

// writeRefParamChecks emits, for one entity/message-typed param p: a nil
// guard followed by one check per @validate rule declared on each of the
// referenced type's fields (guaranteed present by the resolver's
// validate-everything-in opinion for every request-position field).
//
// singleRefParam mirrors opModel.SingleRefParam: when true, p IS req's own
// type directly (proto skipped synthesizing a wrapper around the sole ref
// param -- see opModel.SingleRefParam's doc comment), so fields are
// addressed directly on req ("req.<Field>") and the guard checks req
// itself, never req.<Param> (there is no such field). When false (p is one
// of several params on this operation), the real protobuf field is a
// pointer to the referenced type's own generated message nested under the
// param's own name, addressed as "req.<Param>.<Field>" as before.
func writeRefParamChecks(b *strings.Builder, p paramModel, singleRefParam bool, imports map[string]bool) {
	if len(p.RefFields) == 0 {
		return
	}

	prefix := "req." + p.GoName
	guardTarget := prefix

	if singleRefParam {
		prefix = "req"
		guardTarget = "req"
	}

	fmt.Fprintf(b, "\tif %s == nil {\n", guardTarget)
	fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", p.Name+": is required")
	b.WriteString("\t}\n\n")

	for _, rf := range p.RefFields {
		if len(rf.Validate) == 0 {
			continue
		}

		fieldExpr := prefix + "." + rf.GoName
		errName := p.Name + "." + rf.Name

		if !rf.Optional {
			for _, v := range rf.Validate {
				writeValidationCheck(b, fieldExpr, errName, v, imports)
			}

			continue
		}

		// An optional field's real protobuf-message Go field is a pointer
		// (see refFieldModel.Optional's doc comment): only validate it when
		// the caller actually provided a value, dereferencing once into a
		// named value so every rule below reads as a plain string/numeric
		// check rather than repeating the dereference per rule.
		fmt.Fprintf(b, "\tif %s != nil {\n", fieldExpr)

		valueExpr := "*" + fieldExpr

		var inner strings.Builder

		for _, v := range rf.Validate {
			writeValidationCheck(&inner, valueExpr, errName, v, imports)
		}

		for _, line := range strings.Split(strings.TrimSuffix(inner.String(), "\n"), "\n") {
			if line == "" {
				b.WriteString("\n")
				continue
			}

			b.WriteString("\t" + line + "\n")
		}

		b.WriteString("\t}\n\n")
	}
}

// writeValidationCheck emits one ir.Validation's check against fieldExpr (a
// Go expression selecting the field on req, e.g. "req.Email" or
// "req.Profile.Email" for a nested ref-param field), using errName as the
// human-readable name in the returned apperror message.
func writeValidationCheck(b *strings.Builder, fieldExpr, errName string, v ir.Validation, imports map[string]bool) {
	value := v.Args["value"]

	switch v.Kind {
	case "format":
		writeFormatCheck(b, fieldExpr, errName, value, imports)
	case "min_len":
		n := numericLiteral(value)
		fmt.Fprintf(b, "\tif len(%s) < %s {\n", fieldExpr, n)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": must be at least "+n+" characters long")
		b.WriteString("\t}\n\n")
	case "max_len":
		n := numericLiteral(value)
		fmt.Fprintf(b, "\tif len(%s) > %s {\n", fieldExpr, n)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": must be at most "+n+" characters long")
		b.WriteString("\t}\n\n")
	case "gt":
		n := numericLiteral(value)
		fmt.Fprintf(b, "\tif %s <= %s {\n", fieldExpr, n)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": must be greater than "+n)
		b.WriteString("\t}\n\n")
	case "gte":
		n := numericLiteral(value)
		fmt.Fprintf(b, "\tif %s < %s {\n", fieldExpr, n)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": must be greater than or equal to "+n)
		b.WriteString("\t}\n\n")
	case "lt":
		n := numericLiteral(value)
		fmt.Fprintf(b, "\tif %s >= %s {\n", fieldExpr, n)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": must be less than "+n)
		b.WriteString("\t}\n\n")
	case "lte":
		n := numericLiteral(value)
		fmt.Fprintf(b, "\tif %s > %s {\n", fieldExpr, n)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": must be less than or equal to "+n)
		b.WriteString("\t}\n\n")
	}
}

// writeFormatCheck emits the check for one @validate(format: "...") rule,
// dispatching to the right zero-new-dependency stdlib/existing-dependency
// parser for the fixed v1 format set {email, url, uuid}.
func writeFormatCheck(b *strings.Builder, fieldExpr, errName string, value any, imports map[string]bool) {
	format, _ := value.(string)

	switch format {
	case "email":
		imports[importNetMail] = true

		fmt.Fprintf(b, "\tif _, err := mail.ParseAddress(%s); err != nil {\n", fieldExpr)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": invalid email format")
		b.WriteString("\t}\n\n")
	case "url":
		imports[importNetURL] = true

		fmt.Fprintf(b, "\tif _, err := url.ParseRequestURI(%s); err != nil {\n", fieldExpr)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": invalid url format")
		b.WriteString("\t}\n\n")
	case "uuid":
		imports[pkgUUID] = true

		fmt.Fprintf(b, "\tif _, err := uuid.Parse(%s); err != nil {\n", fieldExpr)
		fmt.Fprintf(b, "\t\treturn apperror.New(apperror.InvalidArgument, %q)\n", errName+": invalid uuid format")
		b.WriteString("\t}\n\n")
	}
}

// numericLiteral renders a decoded @validate numeric argument (always int64
// or float64, per resolver.decodeValidateValue) as a Go literal, relying on
// Go's untyped-constant conversion rules to make the emitted comparison
// compile against the param's real numeric Go type (int32/int64/float32/
// float64) without any explicit cast.
func numericLiteral(v any) string {
	switch n := v.(type) {
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.FormatFloat(n, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", n)
	}
}
