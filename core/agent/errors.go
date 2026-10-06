package agent

import (
	"errors"
	"fmt"
)

var (
	// ErrNilClient is returned when New is called without an ai.AI backend.
	ErrNilClient = errors.New("agent: nil client")
	// ErrUnknownTool is returned when the model calls an unregistered tool.
	ErrUnknownTool = errors.New("agent: unknown tool")
	// ErrMaxSteps is returned when the loop exceeds its step bound.
	ErrMaxSteps = errors.New("agent: max steps exceeded")
	// ErrInvalidOptions is returned for invalid agent options.
	ErrInvalidOptions = errors.New("agent: invalid options")
	// ErrInvalidTool is returned for a tool with an empty name or nil handler.
	ErrInvalidTool = errors.New("agent: invalid tool")
	// ErrStructuredOutput is returned when structured output cannot be decoded.
	ErrStructuredOutput = errors.New("agent: structured output")
)

// UnknownToolError reports a model call to an unregistered tool.
type UnknownToolError struct {
	// Name is the requested tool name.
	Name string
}

// Error returns a human-readable unknown-tool message.
func (e UnknownToolError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownTool, e.Name)
}

// Unwrap returns ErrUnknownTool.
func (e UnknownToolError) Unwrap() error { return ErrUnknownTool }

// MaxStepsError reports the loop exceeding its step bound.
type MaxStepsError struct {
	// Max is the bound that was exceeded.
	Max int
}

// Error returns a human-readable max-steps message.
func (e MaxStepsError) Error() string {
	return fmt.Sprintf("%s: %d", ErrMaxSteps, e.Max)
}

// Unwrap returns ErrMaxSteps.
func (e MaxStepsError) Unwrap() error { return ErrMaxSteps }

// InvalidOptionsError reports an options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e InvalidOptionsError) Unwrap() error { return ErrInvalidOptions }

// InvalidToolError reports a malformed tool registration.
type InvalidToolError struct {
	// Name is the offending tool name (may be empty).
	Name string
	// Reason describes the failure.
	Reason string
}

// Error returns a human-readable invalid-tool message.
func (e InvalidToolError) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("%s: %s", ErrInvalidTool, e.Reason)
	}
	return fmt.Sprintf("%s: %s: %s", ErrInvalidTool, e.Name, e.Reason)
}

// Unwrap returns ErrInvalidTool.
func (e InvalidToolError) Unwrap() error { return ErrInvalidTool }

// DuplicateToolError reports a tool name registered twice.
type DuplicateToolError struct {
	// Name is the duplicated tool name.
	Name string
}

// Error returns a human-readable duplicate-tool message.
func (e DuplicateToolError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidTool, e.Name)
}

// Unwrap returns ErrInvalidTool.
func (e DuplicateToolError) Unwrap() error { return ErrInvalidTool }
