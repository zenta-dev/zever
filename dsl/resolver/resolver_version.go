package resolver

import (
	"strings"

	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// versionPathDrift is an opt-in lint: for every operation with an HTTP
// transport whose owning module has a dir-derived Version (e.g. "v1", from
// schema/v1/iam/*.zen), warn if the HTTP path doesn't start with
// "/<version>/". This never blocks compilation (SeverityWarning, not the
// default SeverityError) and never rewrites anything -- HTTP path strings
// stay entirely author-written; this only flags likely drift, e.g. a v2
// module whose rpc still says `http: GET "/v1/..."`.
func versionPathDrift(schema *ir.Schema) diag.List {
	var diags diag.List

	if schema == nil {
		return diags
	}

	for _, m := range schema.Modules {
		if m.Version == "" {
			continue
		}

		prefix := "/" + m.Version + "/"

		for _, svc := range m.Services {
			for _, op := range svc.Operations {
				http, ok := findHTTPTransport(op.Transports)
				if !ok {
					continue
				}

				if strings.HasPrefix(http.Path, prefix) {
					continue
				}

				d := diag.Wrap("resolve", http.Pos, ErrVersionDrift,
					"rpc %s.%s: http path %q does not start with %q (module resolved to version %q)",
					svc.Name, op.Name, http.Path, prefix, m.Version)
				d.Severity = diag.SeverityWarning
				diags = append(diags, d)
			}
		}
	}

	return diags
}

// findHTTPTransport returns the HTTPTransport among transports, if any.
// Mirrors internal/dsl/backend/openapi/render_path.go's helper of the same
// name/shape -- kept as a small, separate copy rather than an import, since
// resolver must not depend on any backend package.
func findHTTPTransport(transports []ir.Transport) (ir.HTTPTransport, bool) {
	for _, t := range transports {
		if h, ok := t.(ir.HTTPTransport); ok {
			return h, true
		}
	}

	return ir.HTTPTransport{}, false
}
