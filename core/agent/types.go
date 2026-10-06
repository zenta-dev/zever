package agent

import "github.com/zenta-dev/zever/core/ai"

// Result is the outcome of a Run.
type Result struct {
	// Content is the final assistant text after the loop stops.
	Content string
	// Messages is the full conversation, including tool turns.
	Messages []ai.Message
	// ToolCalls records every tool call the model made, in order.
	ToolCalls []ai.ToolCall
	// Usage aggregates token consumption across every generation.
	Usage ai.Usage
	// Steps is the number of generation iterations executed.
	Steps int
}
