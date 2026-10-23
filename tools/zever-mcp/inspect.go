package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/dsl/ir"

	"github.com/zenta-dev/zever/shared/codec"
)

// schemaSourceProps is the shared dir/files input every schema tool accepts.
func schemaSourceProps() map[string]any {
	return map[string]any{
		"dir": map[string]any{
			"type":        "string",
			"description": "Directory containing .zen files.",
		},
		"files": map[string]any{
			"type":                 "object",
			"description":          "Inline schema files keyed by filename.",
			"additionalProperties": map[string]any{"type": "string"},
		},
	}
}

// checkTool validates schema sources without generating anything, the MCP
// counterpart of `zever check`.
func checkTool() toolDef {
	return toolDef{
		Name:        "zever_check",
		Description: "Validate .zen schema sources (parse and resolve, no code generation). Returns an ok summary, or the file:line:col diagnostics as an error.",
		InputSchema: objectSchema(schemaSourceProps()),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			var in compileInput
			if err := decodeArgs(args, &in); err != nil {
				return "", err
			}

			res, diags, err := resolveSchema(ctx, in)
			if err != nil {
				return "", err
			}

			if diags.HasErrors() {
				return "", errors.New(formatDiags(diags))
			}

			out := fmt.Sprintf("ok: %d module(s)", len(res.Schema.Modules))
			if len(diags) > 0 {
				out += "\nwarnings:\n" + formatDiags(diags)
			}

			return out, nil
		},
	}
}

// routeEntry is one HTTP binding returned by zever_routes. The JSON shape
// matches the `routes` array of `zever routes --json`.
type routeEntry struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Target is the "Module.Service.RPC" operation name.
	Target string `json:"target"`
}

// routesResult is the zever_routes payload.
type routesResult struct {
	Routes []routeEntry `json:"routes"`
	Count  int          `json:"count"`
}

// collectRoutes returns one entry per RPC that declares an HTTP binding, in
// schema order. A module with an empty name reports as "(root)".
func collectRoutes(s *ir.Schema) []routeEntry {
	entries := []routeEntry{}

	for _, m := range s.Modules {
		name := m.Name
		if name == "" {
			name = "(root)"
		}

		for _, svc := range m.Services {
			for _, op := range svc.Operations {
				for _, t := range op.Transports {
					h, ok := t.(ir.HTTPTransport)
					if !ok {
						continue
					}

					entries = append(entries, routeEntry{Method: h.Method, Path: h.Path, Target: name + "." + svc.Name + "." + op.Name})

					break
				}
			}
		}
	}

	return entries
}

// routesTool lists the HTTP routes the schema declares, the MCP counterpart
// of `zever routes`.
func routesTool() toolDef {
	return toolDef{
		Name:        "zever_routes",
		Description: "List the HTTP routes declared by RPCs in .zen schema sources as JSON: method, path, and the Module.Service.RPC target.",
		InputSchema: objectSchema(schemaSourceProps()),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			var in compileInput
			if err := decodeArgs(args, &in); err != nil {
				return "", err
			}

			res, diags, err := resolveSchema(ctx, in)
			if err != nil {
				return "", err
			}

			if diags.HasErrors() {
				return "", errors.New(formatDiags(diags))
			}

			routes := collectRoutes(res.Schema)

			b, err := codec.MarshalIndent(routesResult{Routes: routes, Count: len(routes)}, "  ")
			if err != nil {
				return "", err
			}

			return string(b), nil
		},
	}
}
