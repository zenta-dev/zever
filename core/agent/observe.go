package agent

import "github.com/zenta-dev/zever/core/ai"

// EventType identifies the kind of agent event.
type EventType string

// Agent event types emitted during a Loop run.
const (
	// EventGenerate fires before each model generation with the step index.
	EventGenerate EventType = "generate"
	// EventToolCall fires for each requested tool call before dispatch.
	EventToolCall EventType = "tool_call"
	// EventToolResult fires for each completed tool call with its output.
	EventToolResult EventType = "tool_result"
	// EventDone fires once when the run completes with the final result.
	EventDone EventType = "done"
)

// Event is one observation from a Loop run.
type Event struct {
	// Type identifies the event kind.
	Type EventType
	// Step is the zero-based generation iteration that produced the event.
	Step int
	// ToolCall is the call for tool events; zero otherwise.
	ToolCall ai.ToolCall
	// Output is the tool output for result events; empty otherwise.
	Output string
	// Result is the final result for done events; nil otherwise.
	Result *Result
	// Err is the terminal failure for failed runs; nil otherwise. It is set
	// on Done events that end the run abnormally.
	Err error
}

// Observer receives agent events. A nil Observer disables observation.
// Observers must be goroutine-safe: tool events may fire concurrently when
// Options.MaxParallel exceeds 1.
type Observer func(Event)
