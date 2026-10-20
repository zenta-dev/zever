package main

import (
	"context"
	"fmt"
	"strings"
)

// generateSubcommands are the `zever generate` subcommands this tool accepts.
// Anything else fails closed with a usage error.
var generateSubcommands = map[string]bool{
	"module": true, "entity": true, "job": true, "schedule": true,
	"server": true, "worker": true, "seed": true, "tinker": true,
	"adapter": true,
}

// generateInput drives the zever_generate tool. Without confirm it runs a
// dry-run plan; with confirm it asks the human first via elicitation and
// only applies on accept.
type generateInput struct {
	Dir        string   `json:"dir"`
	Subcommand string   `json:"subcommand"`
	Args       []string `json:"args"`
	Confirm    bool     `json:"confirm"`
}

// generateTool scaffolds through the installed CLI. Planning is always safe
// (dry-run); applying requires both model intent (confirm:true) and human
// approval (elicitation accept). Anything else returns the plan or a
// declined message the model can act on.
func (s *Server) generateTool() toolDef {
	return toolDef{
		Name:        "zever_generate",
		Description: "Scaffold with `zever generate`. Without confirm runs a dry-run plan; with confirm asks the human, then applies on accept.",
		InputSchema: objectSchema(map[string]any{
			"dir": map[string]any{
				"type":        "string",
				"description": "Project directory to run in.",
			},
			"subcommand": map[string]any{
				"type":        "string",
				"description": "Generate subcommand: module, entity, job, schedule, server, worker, seed, tinker, adapter.",
			},
			"args": map[string]any{
				"type":        "array",
				"description": "Subcommand argv tokens.",
				"items":       map[string]any{"type": "string"},
			},
			"confirm": map[string]any{
				"type":        "boolean",
				"description": "Apply after human approval. Default false returns the dry-run plan only.",
			},
		}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			var in generateInput
			if err := decodeArgs(args, &in); err != nil {
				return "", err
			}

			if !generateSubcommands[in.Subcommand] {
				return "", fmt.Errorf("zever-mcp: unknown generate subcommand %q", in.Subcommand)
			}

			argv := append([]string{"generate", in.Subcommand}, in.Args...)

			if !in.Confirm {
				plan, err := runZeverIn(ctx, in.Dir, append(argv, "--dry-run")...)
				if err != nil {
					return plan, err
				}

				return plan + "\nRe-call with confirm:true to apply after reviewing the plan.", nil
			}

			res, err := s.elicit(ctx, "Apply `zever "+strings.Join(argv, " ")+"`?", map[string]any{"type": "object"})
			if err != nil {
				return "", err
			}

			if res.Action != ElicitAccept {
				return "declined by user", nil
			}

			out, err := runZeverIn(ctx, in.Dir, argv...)
			if err != nil {
				return out, err
			}

			return out, nil
		},
	}
}
