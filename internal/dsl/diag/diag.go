// Package diag provides error/position types for the DSL compiler.
// It has zero dependencies on anything else in this codebase.
package diag

import (
	"fmt"
	"sort"
	"strings"
)

// Position represents a location in a source file.
type Position struct {
	File string
	Line int
	Col  int
}

// String returns a string representation of the position in the format "file:line:col".
func (p Position) String() string {
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
}

// Severity indicates the severity level of a diagnostic.
type Severity int

// Severity constants.
const (
	SeverityError Severity = iota
	SeverityWarning
)

// Diagnostic represents a compile problem (error or warning).
// Unwrap() returns Wrapped so errors.Is/errors.As reach through it transparently.
//
//nolint:errname // established compiler vocabulary: a positioned diagnostic, not an error value
type Diagnostic struct {
	Pos      Position
	Severity Severity
	Phase    string // "lex" | "parse" | "resolve" | a backend's Name()
	Msg      string
	Wrapped  error
}

// Error returns the error message in the format "pos: [phase] msg".
func (d *Diagnostic) Error() string {
	return fmt.Sprintf("%s: [%s] %s", d.Pos.String(), d.Phase, d.Msg)
}

// Unwrap returns the wrapped error, enabling errors.Is/errors.As to reach through it.
func (d *Diagnostic) Unwrap() error {
	return d.Wrapped
}

// New creates a new Diagnostic without a wrapped cause.
func New(phase string, pos Position, format string, args ...any) *Diagnostic {
	return &Diagnostic{
		Pos:     pos,
		Phase:   phase,
		Msg:     fmt.Sprintf(format, args...),
		Wrapped: nil,
	}
}

// Wrap creates a new Diagnostic with a wrapped cause error.
func Wrap(phase string, pos Position, cause error, format string, args ...any) *Diagnostic {
	return &Diagnostic{
		Pos:     pos,
		Phase:   phase,
		Msg:     fmt.Sprintf(format, args...),
		Wrapped: cause,
	}
}

// List represents a collection of diagnostics.
// It implements Unwrap() []error so errors.Is/errors.As reach every element's Wrapped cause.
//
//nolint:errname // established compiler vocabulary: a diagnostic collection, not an error value
type List []*Diagnostic

// Error returns a newline-separated string representation of all diagnostics.
func (l List) Error() string {
	if len(l) == 0 {
		return ""
	}

	var msgs []string
	for _, d := range l {
		msgs = append(msgs, d.Error())
	}

	return strings.Join(msgs, "\n")
}

// Unwrap returns a slice of all wrapped errors (Go 1.20+ multi-error).
// This allows errors.Is/errors.As to find causes in any element of the list.
func (l List) Unwrap() []error {
	errs := make([]error, len(l))
	for i, d := range l {
		errs[i] = d.Wrapped
	}

	return errs
}

// HasErrors returns true if the list contains at least one error (not just warnings).
func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity == SeverityError {
			return true
		}
	}

	return false
}

// Sorted returns a new list sorted by (File, Line, Col) in stable order,
// suitable for deterministic CLI output.
func (l List) Sorted() List {
	sorted := make(List, len(l))
	copy(sorted, l)

	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Pos.File != sorted[j].Pos.File {
			return sorted[i].Pos.File < sorted[j].Pos.File
		}

		if sorted[i].Pos.Line != sorted[j].Pos.Line {
			return sorted[i].Pos.Line < sorted[j].Pos.Line
		}

		return sorted[i].Pos.Col < sorted[j].Pos.Col
	})

	return sorted
}
