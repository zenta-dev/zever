package agent

import (
	"context"

	"github.com/zenta-dev/zever/core/ai"
)

// Agent runs a conversation to completion, dispatching tool calls.
type Agent interface {
	// Run executes messages to completion and returns the final result.
	Run(ctx context.Context, messages []ai.Message) (Result, error)
}

// Loop is the built-in tool-calling Agent over an ai.AI backend.
type Loop struct {
	client ai.AI
	opts   Options
	tools  map[string]Tool
	order  []string
}

// New builds a Loop over client. It validates client and the tool set, and
// resolves option defaults.
func New(client ai.AI, opts Options) (*Loop, error) {
	if client == nil {
		return nil, ErrNilClient
	}

	tools := make(map[string]Tool, len(opts.Tools))
	order := make([]string, 0, len(opts.Tools))

	for _, t := range opts.Tools {
		if t.Name == "" {
			return nil, InvalidToolError{Reason: "name is required"}
		}
		if t.Handler == nil {
			return nil, InvalidToolError{Name: t.Name, Reason: "handler is required"}
		}
		if _, dup := tools[t.Name]; dup {
			return nil, DuplicateToolError{Name: t.Name}
		}
		tools[t.Name] = t
		order = append(order, t.Name)
	}

	return &Loop{
		client: client,
		opts:   opts.withDefaults(),
		tools:  tools,
		order:  order,
	}, nil
}

// Tools returns the registered tool names in registration order.
func (l *Loop) Tools() []string {
	out := make([]string, len(l.order))
	copy(out, l.order)
	return out
}
