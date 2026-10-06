package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// promptArg describes one prompt argument.
type promptArg struct {
	Name        string
	Description string
	Required    bool
}

// promptDef is a message template with named arguments.
type promptDef struct {
	Name        string
	Description string
	Args        []promptArg
	Render      func(args map[string]string) (string, error)
}

// promptMessage is one rendered prompt message.
type promptMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// prompts lists every prompt template in stable order.
func prompts() []promptDef {
	return []promptDef{
		{
			Name:        "scaffold-feature",
			Description: "Scaffold a zever feature from a natural-language description.",
			Args: []promptArg{
				{Name: "description", Description: "What to build.", Required: true},
			},
			Render: func(args map[string]string) (string, error) {
				desc, ok := args["description"]
				if !ok || strings.TrimSpace(desc) == "" {
					return "", fmt.Errorf("zever-mcp: prompt argument %q is required", "description")
				}

				return "Scaffold this zever feature:\n\n" + desc +
					"\n\nSteps: 1. Write the .zen schema. 2. Run `zever check` on it. " +
					"3. Run `zever compile`. 4. Preview mutations with `--dry-run` before writing.", nil
			},
		},
		{
			Name:        "explain-schema",
			Description: "Explain a schema element by dotted path.",
			Args: []promptArg{
				{Name: "path", Description: "Dotted path like blog.Post.", Required: true},
			},
			Render: func(args map[string]string) (string, error) {
				path, ok := args["path"]
				if !ok || strings.TrimSpace(path) == "" {
					return "", fmt.Errorf("zever-mcp: prompt argument %q is required", "path")
				}

				return "Explain the schema element at path " + path +
					" using the zever_schema and zever_explain tools.", nil
			},
		},
		{
			Name:        "debug-doctor",
			Description: "Triage doctor FAIL rows into config and env fixes.",
			Args: []promptArg{
				{Name: "doctor_json", Description: "Pasted `zever doctor --json` rows.", Required: true},
			},
			Render: func(args map[string]string) (string, error) {
				rows, ok := args["doctor_json"]
				if !ok || strings.TrimSpace(rows) == "" {
					return "", fmt.Errorf("zever-mcp: prompt argument %q is required", "doctor_json")
				}

				return "Triage these `zever doctor --json` rows into config-file and " +
					"environment fixes, marking expected secret failures as such:\n\n" + rows, nil
			},
		},
		{
			Name:        "fix-diags",
			Description: "Turn check diagnostics into a minimal .zen edit plan.",
			Args: []promptArg{
				{Name: "diags", Description: "Pasted `zever check` diagnostics.", Required: true},
			},
			Render: func(args map[string]string) (string, error) {
				diags, ok := args["diags"]
				if !ok || strings.TrimSpace(diags) == "" {
					return "", fmt.Errorf("zever-mcp: prompt argument %q is required", "diags")
				}

				return "Propose the smallest `.zen` edits resolving these diagnostics, " +
					"quoting exact lines:\n\n" + diags, nil
			},
		},
	}
}

// promptList returns the prompts/list payload.
func promptList() []map[string]any {
	defs := prompts()
	out := make([]map[string]any, 0, len(defs))

	for _, p := range defs {
		args := make([]map[string]any, 0, len(p.Args))
		for _, a := range p.Args {
			args = append(args, map[string]any{
				"name":        a.Name,
				"description": a.Description,
				"required":    a.Required,
			})
		}

		out = append(out, map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"arguments":   args,
		})
	}

	return out
}

// promptGetParams is the prompts/get request payload.
type promptGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

// getPrompt renders one prompt template.
func getPrompt(req rpcRequest) rpcResponse {
	var p promptGetParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, codeInvalidParams, "zever-mcp: invalid prompts/get params")
	}

	for _, def := range prompts() {
		if def.Name != p.Name {
			continue
		}

		text, err := def.Render(p.Arguments)
		if err != nil {
			return errorResponse(req.ID, codeInvalidParams, err.Error())
		}

		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"description": def.Description,
			"messages": []promptMessage{{
				Role:    "user",
				Content: map[string]any{"type": "text", "text": text},
			}},
		}}
	}

	return errorResponse(req.ID, codeInvalidParams, fmt.Sprintf("zever-mcp: unknown prompt %q", p.Name))
}
