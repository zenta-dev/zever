package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/diag"
)

// debounceDelay is how long an edit burst must settle before recompiling.
const debounceDelay = 250 * time.Millisecond

// stopper is the cancel half of a scheduled debounce callback. *time.Timer
// satisfies it, and tests substitute a fake driven by hand.
type stopper interface {
	Stop() bool
}

// diagToLSPDiagnostic translates one compiler diagnostic into its LSP form.
// The compiler records a point, not a span; when the document source is
// available the range is widened to the whole identifier containing that
// point, so editors underline the token instead of a single caret. With no
// source it stays a one-character point range.
func diagToLSPDiagnostic(d *diag.Diagnostic, src string) protocol.Diagnostic {
	severity := protocol.DiagnosticSeverityError
	if d.Severity == diag.SeverityWarning {
		severity = protocol.DiagnosticSeverityWarning
	}

	return protocol.Diagnostic{
		Range:    diagnosticRange(d.Pos, src),
		Severity: severity,
		Source:   protocol.NewOptional("zen"),
		Message:  protocol.String(d.Msg),
	}
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

		grouped[d.Pos.File] = append(grouped[d.Pos.File], diagToLSPDiagnostic(d, src))
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

	p.timers[key] = p.afterFunc(debounceDelay, fn)
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
