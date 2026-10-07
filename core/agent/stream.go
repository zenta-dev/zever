package agent

import (
	"context"
	"strings"

	"github.com/zenta-dev/zever/core/ai"
)

// ToolResult pairs a dispatched tool call with its output.
type ToolResult struct {
	// Call is the model-requested invocation.
	Call ai.ToolCall
	// Output is the handler output reported back to the model.
	Output string
}

// StreamEvent is one event from a streaming run.
type StreamEvent struct {
	// Delta carries incremental assistant text.
	Delta string
	// ToolCall is set when the model requests a tool.
	ToolCall *ai.ToolCall
	// ToolResult is set when a dispatched tool completes.
	ToolResult *ToolResult
	// Done marks the terminal event; exactly one of Done or Err is set.
	Done bool
	// Result is the final result on Done; nil otherwise.
	Result *Result
	// Err is a terminal failure; nil otherwise.
	Err error
}

// RunStream executes the tool-calling loop like Run but streams progress:
// text deltas as they arrive, tool calls and results as they complete, and
// a Done event carrying the final result. The channel is bounded and closed
// by the producer; every send also selects ctx.Done so cancelling the context
// releases a consumer-blocked producer. Callers must therefore cancel the
// context (or drain to close) to guarantee no producer goroutine lingers
// after abandoning the channel.
func (l *Loop) RunStream(ctx context.Context, messages []ai.Message) <-chan StreamEvent {
	ch := make(chan StreamEvent, 16)

	go l.runStream(ctx, messages, ch)

	return ch
}

// send delivers ev unless ctx ends first, in which case it reports false.
func send(ctx context.Context, ch chan<- StreamEvent, ev StreamEvent) bool {
	select {
	case ch <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// runStream drives the loop, emitting events into ch and closing it on return.
func (l *Loop) runStream(ctx context.Context, messages []ai.Message, ch chan<- StreamEvent) {
	defer close(ch)

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
			// Best-effort direct send: the channel is fresh and buffered,
			// so this cannot block; select-based send could drop it.
			select {
			case ch <- StreamEvent{Err: err}:
			default:
			}

			return
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

		stream, err := l.client.Stream(ctx, l.opts.Model, msgs, genOpts)
		if err != nil {
			send(ctx, ch, StreamEvent{Err: err})

			return
		}

		gen, ok := l.consumeStream(ctx, ch, stream, &usage)
		if !ok {
			return
		}

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
			send(ctx, ch, StreamEvent{Done: true, Result: &res})

			return
		}

		calls = append(calls, gen.ToolCalls...)

		for _, call := range gen.ToolCalls {
			l.emit(Event{Type: EventToolCall, Step: step, ToolCall: call})

			if !send(ctx, ch, StreamEvent{ToolCall: &call}) {
				return
			}
		}

		results, err := l.dispatchAll(ctx, gen.ToolCalls)
		if err != nil {
			send(ctx, ch, StreamEvent{Err: err})

			return
		}

		for i, call := range gen.ToolCalls {
			l.emit(Event{Type: EventToolResult, Step: step, ToolCall: call, Output: results[i]})

			if !send(ctx, ch, StreamEvent{ToolResult: &ToolResult{Call: call, Output: results[i]}}) {
				return
			}

			msgs = append(msgs, ai.Message{
				Role:       ai.RoleTool,
				Content:    results[i],
				ToolCallID: call.ID,
			})
		}
	}

	send(ctx, ch, StreamEvent{Err: MaxStepsError{Max: l.opts.MaxSteps}})
}

// consumeStream drains one step's stream, emitting text deltas and folding
// tool-call argument deltas by call ID. It returns false when the consumer
// went away or a terminal chunk error arrived (already reported).
func (l *Loop) consumeStream(ctx context.Context, ch chan<- StreamEvent, stream <-chan ai.StreamChunk, usage *ai.Usage) (ai.Generation, bool) {
	var (
		content strings.Builder
		order   []string
		args    = map[string]*strings.Builder{}
		names   = map[string]string{}
		gen     ai.Generation
	)

	for chunk := range stream {
		if chunk.Err != nil {
			send(ctx, ch, StreamEvent{Err: chunk.Err})

			return ai.Generation{}, false
		}

		if chunk.Delta != "" {
			content.WriteString(chunk.Delta)

			if !send(ctx, ch, StreamEvent{Delta: chunk.Delta}) {
				return ai.Generation{}, false
			}
		}

		if chunk.ToolCallID != "" {
			if _, ok := args[chunk.ToolCallID]; !ok {
				args[chunk.ToolCallID] = &strings.Builder{}
				names[chunk.ToolCallID] = chunk.ToolName
				order = append(order, chunk.ToolCallID)
			}

			args[chunk.ToolCallID].WriteString(chunk.ToolArgsDelta)

			if chunk.ToolName != "" {
				names[chunk.ToolCallID] = chunk.ToolName
			}
		}

		if chunk.Usage != nil {
			usage.PromptTokens += chunk.Usage.PromptTokens
			usage.CompletionTokens += chunk.Usage.CompletionTokens
		}

		if chunk.Done {
			gen.FinishReason = chunk.FinishReason
		}
	}

	gen.Content = content.String()

	for _, id := range order {
		gen.ToolCalls = append(gen.ToolCalls, ai.ToolCall{
			ID:        id,
			Name:      names[id],
			Arguments: args[id].String(),
		})
	}

	return gen, true
}
