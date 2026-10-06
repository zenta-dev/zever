package agent

// DefaultMaxSteps is the default bound on generation iterations per Run.
const DefaultMaxSteps = 8

// DefaultMaxRetries is the default decode-retry budget for GenerateStructured.
const DefaultMaxRetries = 3

// Options configures a Loop.
type Options struct {
	// Model is the model name passed to the ai.AI backend.
	Model string
	// SystemPrompt, when non-empty, seeds the conversation as a system turn.
	SystemPrompt string
	// MaxSteps bounds the tool loop; <= 0 selects DefaultMaxSteps.
	MaxSteps int
	// Tools are the tools exposed to the model for every Run.
	Tools []Tool
	// Confirm gates destructive tools; nil runs them without confirmation.
	Confirm ConfirmFunc
	// MaxParallel caps concurrent tool dispatches per step; <= 1 runs
	// sequentially. Result order always matches call order.
	MaxParallel int
	// Observe receives execution events; nil disables observation. It must
	// be goroutine-safe when MaxParallel exceeds 1.
	Observe Observer
}

// withDefaults returns a copy of o with zero-valued bounds resolved.
func (o Options) withDefaults() Options {
	if o.MaxSteps <= 0 {
		o.MaxSteps = DefaultMaxSteps
	}
	return o
}
