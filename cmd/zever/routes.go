package main

import (
	"fmt"
	"io"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
)

// RoutesConfig carries every input runRoutesWith needs. Screen agents build
// it from huh forms; the flag shell (runRoutes) builds it from argv. A nil
// Out defaults to os.Stdout.
type RoutesConfig struct {
	Files []string
	Out   io.Writer
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
	paths, err := resolveInputFiles(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		reportAutoDiscovery(paths)
	}

	return runRoutesWith(RoutesConfig{Files: paths})
}

// runRoutesWith resolves cfg.Files with zero backends and prints one line
// per RPC declaring an HTTP binding to cfg.Out, in
// "<METHOD> <path> -> <Module>.<Service>.<RPC>" form.
func runRoutesWith(cfg RoutesConfig) error {
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
		return fmt.Errorf("zever routes: %d file(s) failed to compile", len(files))
	}

	for _, m := range result.Schema.Modules {
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

				method := http.Method

				if colorEnabled {
					var methodStyled string

					switch method {
					case "GET":
						methodStyled = green(fmt.Sprintf("%-6s", method))
					case "POST":
						methodStyled = yellow(fmt.Sprintf("%-6s", method))
					case "DELETE":
						methodStyled = red(fmt.Sprintf("%-6s", method))
					default:
						methodStyled = cyan(fmt.Sprintf("%-6s", method))
					}

					target := dim(moduleName+".") + bold(svc.Name+"."+rpc.Name)
					_, _ = fmt.Fprintf(out, "%s  %s  %s %s\n", methodStyled, cyan(http.Path), dim("→"), target)
				} else {
					_, _ = fmt.Fprintf(out, "%s %s -> %s.%s.%s\n", method, http.Path, moduleName, svc.Name, rpc.Name)
				}
			}
		}
	}

	return nil
}
