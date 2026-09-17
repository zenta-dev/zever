package ollama

import "errors"

// ErrNoModel is returned when no model is set on the call or the adapter.
var ErrNoModel = errors.New("ollama: model is required")
