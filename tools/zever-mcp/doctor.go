package main

import (
	"context"
)

// doctorInput selects the project configuration to diagnose.
type doctorInput struct {
	Dir string `json:"dir"`
}

// doctorTool resolves every battery through the installed CLI and reports
// the result. It shells out to `zever doctor --json` so the report always
// matches CLI behavior; the zever binary must be on PATH.
func doctorTool() toolDef {
	return toolDef{
		Name:        "zever_doctor",
		Description: "Resolve every battery from the project config and report OK/FAIL rows.",
		InputSchema: objectSchema(map[string]any{
			"dir": map[string]any{
				"type":        "string",
				"description": "Project directory to diagnose (runs zever there).",
			},
		}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			var in doctorInput
			if err := decodeArgs(args, &in); err != nil {
				return "", err
			}

			argv := []string{"doctor", "--json"}
			if in.Dir == "" {
				return runZeverIn(ctx, "", argv...)
			}

			return runZeverIn(ctx, in.Dir, argv...)
		},
	}
}
