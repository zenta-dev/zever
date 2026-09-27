// Package stdout provides an observability.Provider implementation that
// writes spans and verbose metrics as JSON lines to an io.Writer.
//
// The adapter uses only the standard library. Span End emits a single JSON
// line of the form {"traceID","spanID","name","attrs","error"}. Metric
// instruments record only when Options.Verbose is set and are otherwise
// dropped silently.
package stdout
