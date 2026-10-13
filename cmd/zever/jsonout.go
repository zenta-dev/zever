package main

import (
	"errors"

	"github.com/zenta-dev/zever/dsl/diag"
)

// JSONDiagnostic is one machine-readable compiler diagnostic for the JSON
// envelope's diagnostics field. It mirrors diag.Diagnostic without the
// wrapped cause (see config.Redact: error display paths never echo raw
// internals), so agents can branch on severity without parsing prose.
type JSONDiagnostic struct {
	// Severity is "error" or "warning".
	Severity string `json:"severity"`
	// Phase is the compiler phase that reported it ("lex", "parse",
	// "resolve", or a backend name).
	Phase string `json:"phase"`
	// Position is the "file:line:col" source location.
	Position string `json:"position"`
	// Message is the human-readable diagnostic text.
	Message string `json:"message"`
}

// jsonDiagnostics converts diags to envelope form in deterministic
// (file, line, col) order. It returns nil when diags is empty so the
// envelope's diagnostics field stays omitted on clean runs.
func jsonDiagnostics(diags diag.List) []JSONDiagnostic {
	if len(diags) == 0 {
		return nil
	}

	sorted := diags.Sorted()
	out := make([]JSONDiagnostic, 0, len(sorted))

	for _, d := range sorted {
		severity := "warning"
		if d.Severity == diag.SeverityError {
			severity = "error"
		}

		out = append(out, JSONDiagnostic{
			Severity: severity,
			Phase:    d.Phase,
			Position: d.Pos.String(),
			Message:  d.Msg,
		})
	}

	return out
}

// errJSONEmitted marks an error whose JSON failure envelope was already
// written to jsonOut by the handler via emitFailure. main (both the Cobra
// and legacy paths) skips its own emitError for such errors and only sets
// the exit code, so a failing command emits exactly one envelope on stdout.
var errJSONEmitted = errors.New("zever: json envelope already emitted")

// emitFailure writes a failure envelope for command carrying the handler's
// structured data and diagnostics, then returns an error joining err with
// errJSONEmitted. Callers return it directly: the process exits nonzero
// (exitCodeFor maps the joined error to ExitError unless the wrapped cause
// is flag misuse) without main emitting a second envelope.
//
// The error code/message contract from the envelope spec maps onto the
// existing fields: code is ExitCode, message is Error. Diagnostics carries
// the compiler diagnostics that also print to stderr.
func emitFailure(command string, data any, diags []JSONDiagnostic, err error) error {
	emitEnvelope(Envelope{
		OK:          false,
		Command:     command,
		ExitCode:    exitCodeFor(err),
		Data:        data,
		Diagnostics: diags,
		Error:       err.Error(),
		Breadcrumbs: breadcrumbsFor(command),
	})

	return errors.Join(err, errJSONEmitted)
}
