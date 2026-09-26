package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
)

// errExplainUsage is returned when runExplain is invoked without the
// dotted path argument and at least one input file.
var errExplainUsage = errors.New(
	"zever explain: usage: zever explain <Service.Operation|Module.Service.Operation> <file.zen>",
)

// ExplainConfig carries every input runExplainWith needs. Screen agents
// build it from huh forms; the flag shell (runExplain) builds it from argv.
// OpPath is "Service.Operation" or "Module.Service.Operation". A nil Out
// defaults to os.Stdout.
type ExplainConfig struct {
	OpPath string
	Files  []string
	Out    io.Writer
}

// runExplain compiles the given .zen files with zero backends (schema
// resolution only, same read-only pattern as runRoutes) and prints the
// declaration location and a compact summary of one operation identified by
// a dotted path.
//
// The path is either "Service.Operation" (matched against every module) or
// "Module.Service.Operation" (module-qualified, for disambiguating an
// operation name that exists in more than one module's service). Matching
// is exact and case-sensitive on Module.Name/Service.Name/Operation.Name.
func runExplain(args []string) error {
	if len(args) < 1 {
		return errExplainUsage
	}

	path := args[0]

	paths, err := resolveInputFiles(args[1:])
	if err != nil {
		return err
	}

	if len(args) < 2 {
		reportAutoDiscovery(paths)
	}

	return runExplainWith(ExplainConfig{OpPath: path, Files: paths})
}

// runExplainWith resolves cfg.Files with zero backends and prints cfg.OpPath's
// declaration location plus a compact summary (transports, errors, auth,
// permission) to cfg.Out.
func runExplainWith(cfg ExplainConfig) error {
	out := outOrStdout(cfg.Out)

	files, err := loadFiles(cfg.Files)
	if err != nil {
		return err
	}

	result, diags := compile.Compile(files)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		return fmt.Errorf("zever explain: %d file(s) failed to compile", len(files))
	}

	moduleName, serviceName, opName, err := parseExplainPath(cfg.OpPath)
	if err != nil {
		return err
	}

	op, svc, mod, found := findOperation(result.Schema.Modules, moduleName, serviceName, opName)
	if !found {
		return explainNotFoundError(cfg.OpPath, result.Schema.Modules)
	}

	printExplain(out, mod, svc, op)

	return nil
}

// parseExplainPath splits path into (module, service, operation). module is
// "" when path has exactly two dot-separated segments (unqualified, matched
// against every module); a three-segment path module-qualifies the match.
// Any other segment count is an error.
func parseExplainPath(path string) (module, service, operation string, err error) {
	parts := strings.Split(path, ".")

	switch len(parts) {
	case 2:
		return "", parts[0], parts[1], nil
	case 3:
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", fmt.Errorf(
			"zever explain: %q must be Service.Operation or Module.Service.Operation", path)
	}
}

// findOperation walks modules looking for an operation matching
// moduleName (ignored when ""), serviceName, and opName.
func findOperation(
	modules []*ir.Module, moduleName, serviceName, opName string,
) (op *ir.Operation, svc *ir.Service, mod *ir.Module, found bool) {
	for _, m := range modules {
		if moduleName != "" && m.Name != moduleName {
			continue
		}

		for _, s := range m.Services {
			if s.Name != serviceName {
				continue
			}

			for _, o := range s.Operations {
				if o.Name == opName {
					return o, s, m, true
				}
			}
		}
	}

	return nil, nil, nil, false
}

// explainNotFoundError builds a clear "not found" error, adding a
// suggestion via the repo's existing closest-match helper against every
// operation's fully-qualified name in the compiled schema when one is
// close enough.
func explainNotFoundError(path string, modules []*ir.Module) error {
	var known []string

	for _, m := range modules {
		for _, s := range m.Services {
			for _, o := range s.Operations {
				known = append(known, s.Name+"."+o.Name)
				if m.Name != "" {
					known = append(known, m.Name+"."+s.Name+"."+o.Name)
				}
			}
		}
	}

	if s := closest(path, known); s != "" {
		return fmt.Errorf("zever explain: no operation matches %q (did you mean %q?)", path, s)
	}

	return fmt.Errorf("zever explain: no operation matches %q", path)
}

// printExplain prints op's declaration file:line plus a compact summary:
// transports, declared errors, auth requirement, and permission check.
func printExplain(out io.Writer, mod *ir.Module, svc *ir.Service, op *ir.Operation) {
	moduleName := mod.Name
	if moduleName == "" {
		moduleName = "(root)"
	}

	qualified := moduleName + "." + svc.Name + "." + op.Name

	if colorEnabled {
		_, _ = fmt.Fprintf(out, "%s  %s\n", bold(qualified), dim(op.Pos.String()))
	} else {
		_, _ = fmt.Fprintf(out, "%s  %s\n", qualified, op.Pos.String())
	}

	_, _ = fmt.Fprintf(out, "  transports: %s\n", explainTransports(op.Transports))
	_, _ = fmt.Fprintf(out, "  auth:       %s\n", explainAuth(op.Auth))
	_, _ = fmt.Fprintf(out, "  permission: %s\n", explainPermission(op.Permission))
	_, _ = fmt.Fprintf(out, "  errors:     %s\n", explainErrors(op.Errors))
}

// explainTransports summarizes op.Transports: always notes gRPC, plus
// "METHOD path" when an HTTP transport is present.
func explainTransports(transports []ir.Transport) string {
	parts := make([]string, 0, len(transports))

	for _, t := range transports {
		switch v := t.(type) {
		case ir.HTTPTransport:
			parts = append(parts, fmt.Sprintf("HTTP %s %s", v.Method, v.Path))
		case ir.GRPCTransport:
			parts = append(parts, "gRPC")
		}
	}

	if len(parts) == 0 {
		return "gRPC"
	}

	return strings.Join(parts, ", ")
}

// explainAuth summarizes op.Auth, or "none" when absent.
func explainAuth(auth *ir.AuthPolicy) string {
	if auth == nil {
		return "none"
	}

	if len(auth.Roles) == 0 {
		return "required"
	}

	return "required (roles: " + strings.Join(auth.Roles, ", ") + ")"
}

// explainPermission summarizes op.Permission, or "none" when absent.
func explainPermission(perm *ir.PermissionCheck) string {
	if perm == nil {
		return "none"
	}

	resource := "?"
	if perm.Resource != nil {
		resource = perm.Resource.Name
	}

	return fmt.Sprintf("check(%q, resource: %s)", perm.Check, resource)
}

// explainErrors summarizes op.Errors, or "none" when absent.
func explainErrors(errs []*ir.ErrorCase) string {
	if len(errs) == 0 {
		return "none"
	}

	names := make([]string, 0, len(errs))
	for _, e := range errs {
		names = append(names, e.Code.GRPCName())
	}

	return strings.Join(names, ", ")
}
