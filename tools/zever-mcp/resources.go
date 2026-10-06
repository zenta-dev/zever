package main

import (
	"encoding/json"
	"fmt"
)

// resourceDef describes one static MCP resource.
type resourceDef struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Text        string
}

// staticResources are reference documents served without I/O.
var staticResources = []resourceDef{
	{
		URI:         "zever://cli-reference",
		Name:        "CLI reference",
		Description: "zever command invocations, flags, and exit codes",
		MimeType:    "text/markdown",
		Text: "# zever CLI reference\n" +
			"\n" +
			"- zever new <name> [--dry-run]: scaffold an application.\n" +
			"- zever generate <sub> ... [--dry-run]: scaffold modules, entities, jobs, schedules, entrypoints.\n" +
			"- zever compile <files> [--backend X] [--json]: compile schemas.\n" +
			"- zever check <files> [--json]: validate only.\n" +
			"- zever explain <path> <files> [--json]: describe one operation.\n" +
			"- zever doctor [--json]: resolve every battery.\n" +
			"- zever --help --agent: machine-readable command catalog.\n" +
			"\n" +
			"Exit codes: 0 success, 1 runtime error, 2 flag misuse.\n" +
			"--json emits {ok, command, exitCode, data?, error?} on stdout.\n",
	},
	{
		URI:         "zever://schema-guide",
		Name:        "Schema guide",
		Description: "how to inspect and validate .zen schemas",
		MimeType:    "text/markdown",
		Text: "# .zen schema guide\n" +
			"\n" +
			"1. zever_schema with inline files or a directory for a module overview.\n" +
			"2. zever_explain with a dotted path (Module.Entity, Module.Service, Module.Service.Operation).\n" +
			"3. zever_compile for the merged OpenAPI document.\n" +
			"4. zever check --json to validate before compiling.\n",
	},
}

// resourceList returns the resources/list payload.
func resourceList() []map[string]any {
	out := make([]map[string]any, 0, len(staticResources))

	for _, r := range staticResources {
		out = append(out, map[string]any{
			"uri":         r.URI,
			"name":        r.Name,
			"description": r.Description,
			"mimeType":    r.MimeType,
		})
	}

	return out
}

// resourceReadParams is the resources/read request payload.
type resourceReadParams struct {
	URI string `json:"uri"`
}

// readResource serves one static resource by URI.
func readResource(req rpcRequest) rpcResponse {
	var p resourceReadParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, codeInvalidParams, "zever-mcp: invalid resources/read params")
	}

	for _, r := range staticResources {
		if r.URI != p.URI {
			continue
		}

		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"contents": []map[string]any{{
				"uri":      r.URI,
				"mimeType": r.MimeType,
				"text":     r.Text,
			}},
		}}
	}

	return errorResponse(req.ID, codeInvalidParams, fmt.Sprintf("zever-mcp: unknown resource %q", p.URI))
}
