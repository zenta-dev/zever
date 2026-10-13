package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
)

// RoutesConfig carries every input runRoutesWith needs. Screen agents build
// it from huh forms; the flag shell (runRoutes) builds it from argv. A nil
// Out defaults to os.Stdout. JSON emits a machine-readable envelope instead
// of the human line list.
type RoutesConfig struct {
	Files []string
	Out   io.Writer
	JSON  bool
}

// routeEntry is one JSON-serializable HTTP binding for the routes envelope.
type routeEntry struct {
	// Method is the HTTP method (GET, POST, ...).
	Method string `json:"method"`
	// Path is the HTTP path template.
	Path string `json:"path"`
	// Target is the "Module.Service.RPC" operation name.
	Target string `json:"target"`
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

// runRoutes compiles the given .zen files with zero backends (schema
// resolution only) and prints one line per RPC that declares an HTTP
// binding, in "<METHOD> <path> -> <Module>.<Service>.<RPC>" form.
func runRoutes(args []string) error {
	if err := rejectUnknownFlags("routes", args, nil); err != nil {
		return err
	}

	paths, err := resolveInputFiles(args)
	if err != nil {
		return fmt.Errorf("zever routes: %w", err)
	}

	if len(args) == 0 {
		reportAutoDiscovery(paths)
	}

	return runRoutesWith(RoutesConfig{Files: paths, JSON: jsonMode})
}

// runRoutesWith resolves cfg.Files with zero backends and prints one line
// per RPC declaring an HTTP binding to cfg.Out, in
// "<METHOD> <path> -> <Module>.<Service>.<RPC>" form. With cfg.JSON it
// emits a machine-readable envelope instead and cfg.Out stays untouched.
func runRoutesWith(cfg RoutesConfig) error {
	out := outOrStdout(cfg.Out)

	files, err := loadFiles(cfg.Files)
	if err != nil {
		return fmt.Errorf("zever routes: %w", err)
	}

	result, diags := compile.Compile(files)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	if diags.HasErrors() {
		err := fmt.Errorf("zever routes: %d file(s) failed to compile", len(files))
		if cfg.JSON {
			return emitFailure("routes", map[string]any{"files": cfg.Files}, jsonDiagnostics(diags), err)
		}

		return err
	}

	entries := collectRoutes(result.Schema.Modules)

	if cfg.JSON {
		emitSuccess("routes", map[string]any{
			"files":  cfg.Files,
			"routes": entries,
			"count":  len(entries),
		})

		return nil
	}

	for _, e := range entries {
		printRoute(out, e)
	}

	return nil
}

// collectRoutes walks modules in order and returns one entry per RPC that
// declares an HTTP binding.
func collectRoutes(modules []*ir.Module) []routeEntry {
	entries := []routeEntry{}

	for _, m := range modules {
		moduleName := m.Name
		if moduleName == "" {
			moduleName = "(root)"
		}

		for _, svc := range m.Services {
			for _, rpc := range svc.Operations {
				http, ok := findHTTPTransport(rpc.Transports)
				if !ok {
					continue
				}

				entries = append(entries, routeEntry{
					Method: http.Method,
					Path:   http.Path,
					Target: moduleName + "." + svc.Name + "." + rpc.Name,
				})
			}
		}
	}

	return entries
}

// printRoute prints one entry in the human "<METHOD> <path> ->
// <Module>.<Service>.<RPC>" form, styled when color is enabled.
func printRoute(out io.Writer, e routeEntry) {
	if colorEnabled {
		var methodStyled string

		switch e.Method {
		case http.MethodGet:
			methodStyled = green(fmt.Sprintf("%-6s", e.Method))
		case http.MethodPost:
			methodStyled = yellow(fmt.Sprintf("%-6s", e.Method))
		case http.MethodDelete:
			methodStyled = red(fmt.Sprintf("%-6s", e.Method))
		default:
			methodStyled = cyan(fmt.Sprintf("%-6s", e.Method))
		}

		// Split "Module.Service.RPC" for the dim-module/bold-service styling.
		target := e.Target
		if i := strings.LastIndex(target, "."); i >= 0 {
			if j := strings.LastIndex(target[:i], "."); j >= 0 {
				target = dim(target[:j+1]) + bold(target[j+1:])
			} else {
				target = bold(target)
			}
		}

		_, _ = fmt.Fprintf(out, "%s  %s  %s %s\n", methodStyled, cyan(e.Path), dim("→"), target)

		return
	}

	_, _ = fmt.Fprintf(out, "%s %s -> %s\n", e.Method, e.Path, e.Target)
}
