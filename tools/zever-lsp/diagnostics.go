package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/diag"
)

// debounceDelay is how long an edit burst must settle before recompiling.
// It is also the default of the settings-driven delay (Settings.DebounceDelay).
const debounceDelay = 250 * time.Millisecond

// Diagnostic codes. Each code names one class of compiler diagnostic,
// classified deterministically from the message text (see diagnosticClasses).
// The codes are a stability contract: once a class ships with a code, that
// code never moves to another class, so editor filters and CodeDescription
// links stay meaningful across releases.
const (
	// diagCodeUnknownFieldType is the resolver's "unknown field type" class:
	// a field's type names no scalar, enum, entity, or message.
	diagCodeUnknownFieldType = "zen-001"
	// diagCodeUnknownRelationTarget is the resolver's "relation ... references
	// unknown entity" class: a relation points at an entity that does not
	// exist (the class code_action.go's fixUnresolvedRelationTarget serves).
	diagCodeUnknownRelationTarget = "zen-002"
	// diagCodeDuplicateDeclaration is the duplicate-declaration class:
	// duplicated entities, enum values, error codes, saga steps, and
	// many_to_many join tables.
	diagCodeDuplicateDeclaration = "zen-003"
	// diagCodeCrossModuleReference is the module-boundary class: a relation,
	// rpc param/return, or permission crosses module boundaries.
	diagCodeCrossModuleReference = "zen-004"
	// diagCodeUnknownAttribute is the unknown-attribute class: an @attribute
	// the resolver does not know on fields, entities, relations, indexes,
	// messages, params, and so on.
	diagCodeUnknownAttribute = "zen-005"
	// diagCodeUnknownName is the generic unknown-identifier class: a name
	// outside its expected vocabulary (calls, argument keys, default
	// functions, enum values, error codes, validate kinds, formats, and the
	// like). It deliberately omits the classes above, which match first.
	diagCodeUnknownName = "zen-006"
	// diagCodeSyntaxError is the parser's syntax-error class: messages of
	// the "expected X, got Y" shape.
	diagCodeSyntaxError = "zen-007"
	// diagCodeInvalidValue is the invalid-literal/invalid-value class:
	// malformed numbers, durations, cron expressions, and path placeholders.
	diagCodeInvalidValue = "zen-008"
)

// diagDocURL anchors the in-two-clicks explanation of these codes in this
// package's README, which documents the full code table alongside the
// settings this server reads.
const diagDocURL = "https://github.com/zenta-dev/zever/blob/main/tools/zever-lsp/README.md#diagnostic-codes"

// diagnosticClasses classifies diagnostic messages into codes: the first
// pattern (plain substring containment, the same matcher code_action.go
// uses) that the message contains wins. Ordered most-specific first, so
// narrower patterns ("attribute @", which the resolver renders inside
// "unknown ... attribute @x" messages) must precede the broad "unknown ".
// A message matching nothing carries no code at all -- a speculative code
// is accidental API.
var diagnosticClasses = []struct {
	pattern string
	code    string
}{
	{"unknown field type", diagCodeUnknownFieldType},
	{"references unknown entity", diagCodeUnknownRelationTarget},
	{"duplicate ", diagCodeDuplicateDeclaration},
	{"different module", diagCodeCrossModuleReference},
	{"crosses module boundary", diagCodeCrossModuleReference},
	{"attribute @", diagCodeUnknownAttribute},
	{"unknown ", diagCodeUnknownName},
	{"expected ", diagCodeSyntaxError},
	{"invalid ", diagCodeInvalidValue},
	{"out of range", diagCodeInvalidValue},
	{"nesting depth exceeded", diagCodeInvalidValue},
}

// diagnosticCode returns the stable code of the diagnostic class msg
// belongs to, or "" when no class matches.
func diagnosticCode(msg string) string {
	for _, class := range diagnosticClasses {
		if strings.Contains(msg, class.pattern) {
			return class.code
		}
	}

	return ""
}

// stopper is the cancel half of a scheduled debounce callback. *time.Timer
// satisfies it, and tests substitute a fake driven by hand.
type stopper interface {
	Stop() bool
}

// diagToLSPDiagnostic translates one compiler diagnostic into its LSP form.
// The compiler records a point, not a span; when the document source is
// available the range is widened to the whole identifier containing that
// point, so editors underline the token instead of a single caret. With no
// source it stays a one-character point range. Related locations map to
// RelatedInformation the same way: a related entry in the primary file
// reuses src, otherwise srcFor supplies that file's source, with a point
// range fallback when the content is unknown.
func diagToLSPDiagnostic(
	d *diag.Diagnostic, src string, srcFor func(path string) (string, bool),
) protocol.Diagnostic {
	severity := protocol.DiagnosticSeverityError
	if d.Severity == diag.SeverityWarning {
		severity = protocol.DiagnosticSeverityWarning
	}

	out := protocol.Diagnostic{
		Range:    diagnosticRange(d.Pos, src),
		Severity: severity,
		Source:   protocol.NewOptional("zen"),
		Message:  protocol.String(d.Msg),
	}

	for _, rel := range d.Related {
		if rel.Pos.File == "" {
			continue
		}

		relSrc := ""
		if rel.Pos.File == d.Pos.File {
			relSrc = src
		} else if srcFor != nil {
			if content, ok := srcFor(rel.Pos.File); ok {
				relSrc = content
			}
		}

		out.RelatedInformation = append(out.RelatedInformation, protocol.DiagnosticRelatedInformation{
			Location: protocol.Location{
				URI:   pathToURI(rel.Pos.File),
				Range: diagnosticRange(rel.Pos, relSrc),
			},
			Message: rel.Msg,
		})
	}

	// Only a message matching a known class carries a code (plus its
	// CodeDescription, which the spec requires to be paired with one);
	// everything else stays code-less rather than inventing a class.
	if code := diagnosticCode(d.Msg); code != "" {
		out.Code = protocol.String(code)
		out.CodeDescription = protocol.CodeDescription{Href: uri.URI(diagDocURL)}
	}

	return out
}

// diagnosticPublisher groups diagnostics per file and publishes them,
// clearing files that had diagnostics on a previous run but no longer do.
type diagnosticPublisher struct {
	mu            sync.Mutex
	lastPublished map[string][]protocol.Diagnostic
	timers        map[string]stopper
	afterFunc     func(d time.Duration, fn func()) stopper
	// srcFor supplies a file's source text so diagnostic ranges can widen to
	// the enclosing identifier. Nil (the zero value) falls back to point
	// ranges, which is what tests that do not exercise spans rely on.
	srcFor func(path string) (string, bool)
	// delayHook supplies the debounce delay for each schedule call, so the
	// server can honor Settings.DebounceDelay. Nil falls back to debounceDelay.
	// It runs while the schedule lock is held, so an implementation must not
	// re-enter the publisher; the server wires it to an atomic settings load,
	// which takes no other lock.
	delayHook func() time.Duration
}

// newDiagnosticPublisher returns a publisher with empty state and a time.AfterFunc-driven debounce clock.
func newDiagnosticPublisher() *diagnosticPublisher {
	return &diagnosticPublisher{
		lastPublished: make(map[string][]protocol.Diagnostic),
		timers:        make(map[string]stopper),
		afterFunc: func(d time.Duration, fn func()) stopper {
			return time.AfterFunc(d, fn)
		},
	}
}

// groupDiagnostics buckets a diagnostic list by the file each one points at.
// Diagnostics with no file (e.g. a backend-level failure) are dropped, since
// there is no document to attach them to. srcFor, when non-nil, supplies a
// file's source text so each range can be widened to its enclosing
// identifier; a nil srcFor or a miss falls back to a point range.
func groupDiagnostics(diags diag.List, srcFor func(path string) (string, bool)) map[string][]protocol.Diagnostic {
	grouped := make(map[string][]protocol.Diagnostic)

	for _, d := range diags {
		if d == nil || d.Pos.File == "" {
			continue
		}

		src := ""

		if srcFor != nil {
			if content, ok := srcFor(d.Pos.File); ok {
				src = content
			}
		}

		grouped[d.Pos.File] = append(grouped[d.Pos.File], diagToLSPDiagnostic(d, src, srcFor))
	}

	return grouped
}

// publish sends one publishDiagnostics notification per affected file,
// including empty arrays for files that are now clean. The debounce timer
// fires without a request context, so it publishes over a background
// context with the client captured when the server was created. Per-file
// failures are joined, so one undeliverable file does not hide the rest.
func (p *diagnosticPublisher) publish(client protocol.Client, known []string, diags diag.List) error {
	if client == nil {
		return nil
	}

	grouped := groupDiagnostics(diags, p.srcFor)

	p.mu.Lock()

	// Every file we know about should end up with an explicit state, so a
	// file that just became clean gets an empty array rather than stale
	// squiggles left behind from the previous compile.
	for _, path := range known {
		if _, ok := grouped[path]; !ok {
			grouped[path] = nil
		}
	}

	for path := range p.lastPublished {
		if _, ok := grouped[path]; !ok {
			grouped[path] = nil
		}
	}

	toSend := make(map[string][]protocol.Diagnostic, len(grouped))

	for path, list := range grouped {
		previous, had := p.lastPublished[path]
		if !had && len(list) == 0 {
			// Never published for this file and still clean: nothing to say.
			continue
		}

		if had && len(previous) == 0 && len(list) == 0 {
			continue
		}

		toSend[path] = list

		if len(list) == 0 {
			delete(p.lastPublished, path)
		} else {
			p.lastPublished[path] = list
		}
	}

	p.mu.Unlock()

	var errs []error

	for path, list := range toSend {
		if list == nil {
			list = []protocol.Diagnostic{}
		}

		params := &protocol.PublishDiagnosticsParams{
			URI:         pathToURI(path),
			Diagnostics: list,
		}

		if err := client.PublishDiagnostics(context.Background(), params); err != nil {
			errs = append(errs, fmt.Errorf("zever-lsp: publish diagnostics for %s: %w", path, errors.Join(ErrPublishDiagnostics, err)))
		}
	}

	return errors.Join(errs...)
}

// schedule debounces recompilation for a document: rapid keystrokes collapse
// into a single compile once the user pauses.
func (p *diagnosticPublisher) schedule(key string, fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if timer, ok := p.timers[key]; ok {
		timer.Stop()
	}

	delay := debounceDelay
	if p.delayHook != nil {
		delay = p.delayHook()
	}

	p.timers[key] = p.afterFunc(delay, fn)
}

// cancel stops any pending debounce for a document.
func (p *diagnosticPublisher) cancel(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if timer, ok := p.timers[key]; ok {
		timer.Stop()
		delete(p.timers, key)
	}
}
