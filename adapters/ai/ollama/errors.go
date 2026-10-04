package ollama

import "errors"

// ErrNoModel is returned when no model is set on the call or the adapter.
var ErrNoModel = errors.New("ollama: model is required")

// ErrOllamaStatus is returned when the Ollama server replies with a non-200 status.
var ErrOllamaStatus = errors.New("ollama: unexpected status")

// ErrInvalidTimeout is returned when Options.Timeout is negative.
var ErrInvalidTimeout = errors.New("ollama: timeout must be >= 0")
