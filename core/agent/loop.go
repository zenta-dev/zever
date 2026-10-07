package agent

import (
	"context"
	"encoding/json"

	"github.com/zenta-dev/zever/core/ai"
)

// Run drives the conversation through a bounded generate/tool loop. It seeds a
// system turn when Options.SystemPrompt is set, then repeatedly generates,
// dispatches any tool calls, appends the tool results, and generates again
// until the model produces a response with no tool calls or MaxSteps is hit.
func (l *Loop) Run(ctx context.Context, messages []ai.Message) (Result, error) {
	msgs := make([]ai.Message, 0, len(messages)+2)
	if l.opts.SystemPrompt != "" {
		msgs = append(msgs, ai.Message{Role: ai.RoleSystem, Content: l.opts.SystemPrompt})
	}
	msgs = append(msgs, messages...)

	specs := l.specs()

	var (
		usage ai.Usage
		calls []ai.ToolCall
	)

	for step := 0; step < l.opts.MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			l.emit(Event{Type: EventDone, Step: step, Err: err})

			return Result{}, err
		}

		l.emit(Event{Type: EventGenerate, Step: step})

		genOpts := ai.GenerateOptions{
			Tools:      specs,
			ToolChoice: ai.ToolChoiceAuto,
		}

		if l.opts.MaxParallel > 1 {
			parallel := true
			genOpts.ParallelToolCalls = &parallel
		}

		gen, err := l.client.Generate(ctx, l.opts.Model, msgs, genOpts)
		if err != nil {
			l.emit(Event{Type: EventDone, Step: step, Err: err})

			return Result{}, err
		}

		usage.PromptTokens += gen.Usage.PromptTokens
		usage.CompletionTokens += gen.Usage.CompletionTokens

		msgs = append(msgs, ai.Message{
			Role:      ai.RoleAssistant,
			Content:   gen.Content,
			ToolCalls: gen.ToolCalls,
		})

		if len(gen.ToolCalls) == 0 {
			res := Result{
				Content:   gen.Content,
				Messages:  msgs,
				ToolCalls: calls,
				Usage:     usage,
				Steps:     step + 1,
			}

			l.emit(Event{Type: EventDone, Step: step, Result: &res})

			return res, nil
		}

		calls = append(calls, gen.ToolCalls...)

		for _, call := range gen.ToolCalls {
			l.emit(Event{Type: EventToolCall, Step: step, ToolCall: call})
		}

		results, err := l.dispatchAll(ctx, gen.ToolCalls)
		if err != nil {
			l.emit(Event{Type: EventDone, Step: step, Err: err})

			return Result{}, err
		}

		for i, call := range gen.ToolCalls {
			l.emit(Event{Type: EventToolResult, Step: step, ToolCall: call, Output: results[i]})

			msgs = append(msgs, ai.Message{
				Role:       ai.RoleTool,
				Content:    results[i],
				ToolCallID: call.ID,
			})
		}
	}

	maxErr := MaxStepsError{Max: l.opts.MaxSteps}
	l.emit(Event{Type: EventDone, Step: l.opts.MaxSteps, Err: maxErr})

	return Result{}, maxErr
}

// emit delivers ev to the configured observer, if any.
func (l *Loop) emit(ev Event) {
	if l.opts.Observe != nil {
		l.opts.Observe(ev)
	}
}

// dispatch resolves and runs one tool call. Unknown tools and confirmation
// failures fail closed; handler errors and malformed arguments are returned to
// the model as tool turns so it can self-correct.
func (l *Loop) dispatch(ctx context.Context, call ai.ToolCall) (string, error) {
	tool, ok := l.tools[call.Name]
	if !ok {
		return "", UnknownToolError{Name: call.Name}
	}

	if tool.Destructive && l.opts.Confirm != nil {
		approved, err := l.opts.Confirm(ctx, call)
		if err != nil {
			return "", err
		}
		if !approved {
			return "declined by user", nil
		}
	}

	args, err := decodeArgs(call.Arguments)
	if err != nil {
		//nolint:nilerr // surfaced to the model as a tool turn so it can self-correct
		return "invalid arguments: " + err.Error(), nil
	}

	out, err := tool.Handler(ctx, args)
	if err != nil {
		//nolint:nilerr // surfaced to the model as a tool turn so it can self-correct
		return "error: " + err.Error(), nil
	}

	return out, nil
}

// specs returns the model-visible tool schemas in registration order.
func (l *Loop) specs() []ai.Tool {
	if len(l.order) == 0 {
		return nil
	}

	specs := make([]ai.Tool, 0, len(l.order))
	for _, name := range l.order {
		specs = append(specs, l.tools[name].spec())
	}

	return specs
}

// decodeArgs parses a tool call's raw JSON arguments into a map. Empty
// arguments decode to an empty map.
func decodeArgs(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}

	if args == nil {
		args = map[string]any{}
	}

	return args, nil
}
