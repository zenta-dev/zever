package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/shared/mcpclient"
)

// ToolsFromClient lists an MCP server's tools as agent tools. Each tool calls
// through to the server; tool execution failures (isError) propagate as
// errors, which the loop reports as tool turns for self-correction.
func ToolsFromClient(ctx context.Context, c *mcpclient.Client) ([]Tool, error) {
	defs, err := c.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: list mcp tools: %w", err)
	}

	tools := make([]Tool, 0, len(defs))

	for _, def := range defs {
		tools = append(tools, ToolFromDefinition(c, def))
	}

	return tools, nil
}

// ToolFromDefinition converts one MCP tool definition into an agent tool.
func ToolFromDefinition(c *mcpclient.Client, def mcpclient.Tool) Tool {
	return Tool{
		Name:        def.Name,
		Description: def.Description,
		Parameters:  def.InputSchema,
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			res, err := c.CallTool(ctx, def.Name, args)
			if err != nil {
				return "", err
			}

			if res.IsError {
				return "", mcpToolError(def.Name, res)
			}

			parts := make([]string, 0, len(res.Content))
			for _, block := range res.Content {
				parts = append(parts, block.Text)
			}

			return strings.Join(parts, "\n"), nil
		},
	}
}

// mcpToolError formats a failed tool result as an error.
func mcpToolError(name string, res mcpclient.CallResult) error {
	parts := make([]string, 0, len(res.Content))
	for _, block := range res.Content {
		parts = append(parts, block.Text)
	}

	return fmt.Errorf("agent: mcp tool %s failed: %s", name, strings.Join(parts, "\n"))
}
