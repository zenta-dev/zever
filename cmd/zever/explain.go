package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
)

// ExplainConfig carries every input runExplainWith needs. Screen agents
// build it from huh forms; the flag shell (runExplain) builds it from argv.
// OpPath is "Service.Operation" or "Module.Service.Operation". A nil Out
// defaults to os.Stdout. JSON emits a machine-readable envelope instead of
// the human summary.
type ExplainConfig struct {
	OpPath string
	Files  []string
	Out    io.Writer
	JSON   bool
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
// runExplain prints the wiring graph when invoked bare (`zever explain`),
// one service's wiring for a single-segment service or service/name ref,
// and an operation summary for dotted operation paths (see runExplainWith).
func runExplain(args []string) error {
	if len(args) == 0 {
		return runExplainWiringWith(ExplainWiringConfig{JSON: jsonMode})
	}

	path := args[0]

	if !strings.Contains(path, ".") {
		return runExplainWiringWith(ExplainWiringConfig{Ref: path, JSON: jsonMode})
	}

	if err := rejectUnknownFlags("explain", args[1:], nil); err != nil {
		return err
	}

	paths, err := resolveInputFiles(args[1:])
	if err != nil {
		return fmt.Errorf("zever explain: %w", err)
	}

	if len(args) < 2 {
		reportAutoDiscovery(paths)
	}

	return runExplainWith(ExplainConfig{OpPath: path, Files: paths, JSON: jsonMode})
}

// runExplainWith resolves cfg.Files with zero backends and prints cfg.OpPath's
// declaration location plus a compact summary (transports, errors, auth,
// permission) to cfg.Out.
func runExplainWith(cfg ExplainConfig) error {
	out := outOrStdout(cfg.Out)

	files, err := loadFiles(cfg.Files)
	if err != nil {
		return fmt.Errorf("zever explain: %w", err)
	}

	result, diags := compile.Compile(files)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		compileErr := fmt.Errorf("zever explain: %d file(s) failed to compile", len(files))
		if cfg.JSON {
			return emitFailure("explain", map[string]any{"path": cfg.OpPath, "files": cfg.Files}, jsonDiagnostics(diags), compileErr)
		}

		return compileErr
	}

	moduleName, serviceName, opName, err := parseExplainPath(cfg.OpPath)
	if err != nil {
		return err
	}

	op, svc, mod, found := findOperation(result.Schema.Modules, moduleName, serviceName, opName)
	if !found {
		return explainNotFoundError(cfg.OpPath, result.Schema.Modules)
	}

	if cfg.JSON {
		emitSuccess("explain", map[string]any{
			"path":       cfg.OpPath,
			"module":     mod.Name,
			"service":    svc.Name,
			"operation":  op.Name,
			"location":   op.Pos.String(),
			"transports": explainTransports(op.Transports),
			"auth":       explainAuth(op.Auth),
			"permission": explainPermission(op.Permission),
			"errors":     explainErrors(op.Errors),
		})

		return nil
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

// ExplainWiringConfig carries every input runExplainWiringWith needs. Ref is
// "" for the full graph, a service name for one service's section, or
// "service/name" for one named instance. A nil Out defaults to os.Stdout.
// JSON emits a machine-readable envelope instead of the human graph.
type ExplainWiringConfig struct {
	Ref  string
	Out  io.Writer
	JSON bool
}

// wiringDependsOn names the documented container-level typed dependencies:
// Job builds a dispatcher over the resolved Queue, and the Scheduler shares
// the process-wide Cache and Queue (resolving Scheduler resolves Queue as a
// side effect). Every other service resolves standalone.
func wiringDependsOn(service string) []string {
	switch service {
	case "job":
		return []string{"queue"}
	case "scheduler":
		return []string{"cache", "queue"}
	default:
		return nil
	}
}

// wiringRow is one ref's wiring section, human and JSON alike.
type wiringRow struct {
	// Ref is the "service" or "service/NAME" label (see Container.Wired).
	Ref string `json:"ref"`
	// Driver is the selected adapter for the ref.
	Driver string `json:"driver"`
	// Drivers lists every registered adapter for the service.
	Drivers []string `json:"drivers"`
	// Depends lists the ref's typed container dependencies, if any.
	Depends []string `json:"depends,omitempty"`
	// Env lists the env keys wiring the ref, in sorted order: the ADAPTER
	// selector plus every set key under the ref's namespace.
	Env []string `json:"env"`
}

// runExplainWiringWith prints the wiring graph: every wired ref's driver,
// registered drivers, typed dependencies, and env keys. Service config is
// env-only (see config.FromEnv); refs cover the default instance of every
// service plus each SERVICE__NAME__* named instance in the environment.
func runExplainWiringWith(cfg ExplainWiringConfig) error {
	out := outOrStdout(cfg.Out)

	env, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("zever explain: resolve env: %w", err)
	}

	resolved, err := config.Resolve(env)
	if err != nil {
		return fmt.Errorf("zever explain: resolve config from env: %w", err)
	}

	services := resolved.RedactedServices()
	instances := redactedInstances(env, resolved, services)

	rows := buildWiringRows(env, services, instances)

	if cfg.Ref != "" {
		rows = filterWiringRows(rows, cfg.Ref)
		if len(rows) == 0 {
			return wiringNotFoundError(cfg.Ref, services)
		}
	}

	if cfg.JSON {
		emitSuccess("explain", map[string]any{"ref": cfg.Ref, "wiring": rows})

		return nil
	}

	printWiring(out, rows)

	return nil
}

// buildWiringRows assembles one row per default instance plus one per named
// instance, in deterministic (service, name) order.
func buildWiringRows(env config.Env, services map[string]config.ServiceConfig, instances map[string]config.ServiceConfig) []wiringRow {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}

	sort.Strings(names)

	var rows []wiringRow

	for _, svc := range names {
		rows = append(rows, wiringRow{
			Ref:     svc,
			Driver:  services[svc].Adapter,
			Drivers: container.RegisteredAdapters(svc),
			Depends: wiringDependsOn(svc),
			Env:     wiringEnvKeys(env, svc, ""),
		})

		for _, iname := range sortedInstanceNames(instances, svc) {
			rows = append(rows, wiringRow{
				Ref:     svc + "/" + iname,
				Driver:  instances[svc+"/"+iname].Adapter,
				Drivers: container.RegisteredAdapters(svc),
				Depends: wiringDependsOn(svc),
				Env:     wiringEnvKeys(env, svc, iname),
			})
		}
	}

	return rows
}

// wiringEnvKeys lists the env keys wiring one ref in sorted order: the
// ADAPTER selector when set, plus every set key under the ref's namespace
// (SERVICE_ for the default instance, SERVICE__NAME__ for a named one).
func wiringEnvKeys(env config.Env, svc, name string) []string {
	var prefix, adapterKey string

	if name == "" {
		prefix = strings.ToUpper(svc) + "_"
		adapterKey = strings.ToUpper(svc) + "_ADAPTER"
	} else {
		prefix = strings.ToUpper(svc) + "__" + strings.ToUpper(name) + "__"
		adapterKey = prefix + "ADAPTER"
	}

	var out []string

	for k := range env {
		upper := strings.ToUpper(k)
		if upper == adapterKey || (strings.HasPrefix(upper, prefix) && upper != strings.ToUpper(svc)) {
			out = append(out, k)
		}
	}

	sort.Strings(out)

	return out
}

// filterWiringRows keeps the row for ref (service or service/name,
// case-insensitive on both segments) plus, for a bare service, its named
// instances.
func filterWiringRows(rows []wiringRow, ref string) []wiringRow {
	svc, name, _ := strings.Cut(ref, "/")

	var out []wiringRow

	for _, r := range rows {
		rsvc, rname, _ := strings.Cut(r.Ref, "/")
		if !strings.EqualFold(rsvc, svc) {
			continue
		}

		if name == "" || strings.EqualFold(rname, name) {
			out = append(out, r)
		}
	}

	return out
}

// wiringNotFoundError builds the unknown-ref error with a did-you-mean hint
// against every known service label.
func wiringNotFoundError(ref string, services map[string]config.ServiceConfig) error {
	known := make([]string, 0, len(services))

	for svc := range services {
		known = append(known, svc)
	}

	sort.Strings(known)

	if s := closest(ref, known); s != "" {
		return fmt.Errorf("zever explain: no service matches %q (did you mean %q?)", ref, s)
	}

	return fmt.Errorf("zever explain: no service matches %q", ref)
}

// printWiring prints rows in ref order: driver, registered drivers, typed
// dependencies, and env keys per ref.
func printWiring(out io.Writer, rows []wiringRow) {
	for _, r := range rows {
		_, _ = fmt.Fprintf(out, "%s  driver=%s\n", cyan(r.Ref), r.Driver)

		if len(r.Drivers) > 0 {
			_, _ = fmt.Fprintf(out, "  drivers: %s\n", strings.Join(r.Drivers, ", "))
		}

		deps := "—"
		if len(r.Depends) > 0 {
			deps = strings.Join(r.Depends, ", ")
		}

		_, _ = fmt.Fprintf(out, "  depends: %s\n", deps)

		if len(r.Env) > 0 {
			_, _ = fmt.Fprintf(out, "  env: %s\n", strings.Join(r.Env, ", "))
		} else {
			_, _ = fmt.Fprintf(out, "  env: (defaults; set %s_* to override)\n", strings.ToUpper(strings.SplitN(r.Ref, "/", 2)[0]))
		}
	}
}
