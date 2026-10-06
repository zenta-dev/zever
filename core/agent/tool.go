package agent

import (
	"context"

	"github.com/zenta-dev/zever/core/ai"
)

// ToolHandler executes a tool call and returns its textual result. An error is
// reported back to the model as a tool turn so it can self-correct.
type ToolHandler func(ctx context.Context, args map[string]any) (string, error)

// ConfirmFunc gates execution of a destructive tool. Returning false skips the
// call and reports the refusal to the model as a tool turn.
type ConfirmFunc func(ctx context.Context, call ai.ToolCall) (bool, error)

// Tool pairs a model-visible ai.Tool schema with a server-side handler.
type Tool struct {
	// Name is the unique tool identifier the model calls.
	Name string
	// Description is the human-readable purpose shown to the model.
	Description string
	// Parameters is the JSON Schema object for the tool arguments.
	Parameters map[string]any
	// Handler executes the call. Required.
	Handler ToolHandler
	// ReadOnly marks the tool as side-effect free.
	ReadOnly bool
	// Destructive marks the tool as requiring confirmation.
	Destructive bool
}

// spec converts t to the ai.Tool schema sent to the model.
func (t Tool) spec() ai.Tool {
	return ai.Tool{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  t.Parameters,
	}
}
