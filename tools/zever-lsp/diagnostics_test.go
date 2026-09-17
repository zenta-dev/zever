package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/diag"
)

// errPublishBoom is the sentinel a fake client fails with, so tests can
// prove publish wraps (not swallows) delivery failures via errors.Is.
var errPublishBoom = errors.New("zever-lsp: fake publish failed")

// fakeClient records publishDiagnostics calls; it embeds the default client
// so only the one method under test needs an override.
type fakeClient struct {
	protocol.UnimplementedClient

	mu    sync.Mutex
	calls map[string][]protocol.Diagnostic
	err   error
}

func (c *fakeClient) PublishDiagnostics(_ context.Context, params *protocol.PublishDiagnosticsParams) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.calls == nil {
		c.calls = make(map[string][]protocol.Diagnostic)
	}

	c.calls[string(params.URI)] = params.Diagnostics

	return c.err
}

func (c *fakeClient) published(uri string) ([]protocol.Diagnostic, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	list, ok := c.calls[uri]

	return list, ok
}

func (c *fakeClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.calls)
}

// fakeTimer is a manually-fired stopper: no wall clock, no sleeps.
type fakeTimer struct {
	mu      sync.Mutex
	fn      func()
	stopped bool
	fired   bool
}

func (t *fakeTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.fired {
		return false
	}

	t.stopped = true

	return true
}

func (t *fakeTimer) fire() {
	t.mu.Lock()
	if t.stopped || t.fired {
		t.mu.Unlock()

		return
	}

	t.fired = true
	fn := t.fn
	t.mu.Unlock()

	fn()
}

// fakeClock hands out fakeTimers and fires them on demand, making debounce
// tests deterministic.
type fakeClock struct {
	mu     sync.Mutex
	timers []*fakeTimer
}

func (c *fakeClock) afterFunc(_ time.Duration, fn func()) stopper {
	t := &fakeTimer{fn: fn}

	c.mu.Lock()
	c.timers = append(c.timers, t)
	c.mu.Unlock()

	return t
}

func (c *fakeClock) fireAll() {
	c.mu.Lock()
	timers := append([]*fakeTimer(nil), c.timers...)
	c.mu.Unlock()

	for _, t := range timers {
		t.fire()
	}
}

func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.timers)
}

// withFakeClock swaps the publisher's timer source for a fake and returns it.
func withFakeClock(p *diagnosticPublisher) *fakeClock {
	clock := &fakeClock{}
	p.afterFunc = clock.afterFunc

	return clock
}

func TestDiagToLSPDiagnostic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		in           *diag.Diagnostic
		wantSeverity protocol.DiagnosticSeverity
		wantRange    protocol.Range
	}{
		{
			name: "error at line 3 col 5 becomes a 0-based point range",
			in: &diag.Diagnostic{
				Pos:      diag.Position{File: "a.zen", Line: 3, Col: 5},
				Severity: diag.SeverityError,
				Phase:    "parse",
				Msg:      "unexpected token",
			},
			wantSeverity: protocol.DiagnosticSeverityError,
			wantRange: protocol.Range{
				Start: protocol.Position{Line: 2, Character: 4},
				End:   protocol.Position{Line: 2, Character: 5},
			},
		},
		{
			name: "warning maps to warning severity",
			in: &diag.Diagnostic{
				Pos:      diag.Position{File: "a.zen", Line: 1, Col: 1},
				Severity: diag.SeverityWarning,
				Phase:    "resolve",
				Msg:      "unused entity",
			},
			wantSeverity: protocol.DiagnosticSeverityWarning,
			wantRange: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 1},
			},
		},
		{
			name: "a zero position clamps to the start of the document",
			in: &diag.Diagnostic{
				Pos:      diag.Position{File: "a.zen", Line: 0, Col: 0},
				Severity: diag.SeverityError,
				Msg:      "no position",
			},
			wantSeverity: protocol.DiagnosticSeverityError,
			wantRange: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 1},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := diagToLSPDiagnostic(tc.in)

			if got.Range != tc.wantRange {
				t.Errorf("Range = %+v, want %+v", got.Range, tc.wantRange)
			}

			if got.Severity != tc.wantSeverity {
				t.Errorf("Severity = %v, want %v", got.Severity, tc.wantSeverity)
			}

			msg, ok := got.Message.(protocol.String)
			if !ok || string(msg) != tc.in.Msg {
				t.Errorf("Message = %#v, want %q verbatim", got.Message, tc.in.Msg)
			}

			src, ok := got.Source.Get()
			if !ok || src != "zen" {
				t.Errorf("Source = %q, %v; want \"zen\" present", src, ok)
			}
		})
	}
}

func TestGroupDiagnosticsByFile(t *testing.T) {
	t.Parallel()

	diags := diag.List{
		{Pos: diag.Position{File: "a.zen", Line: 1, Col: 1}, Msg: "one"},
		{Pos: diag.Position{File: "b.zen", Line: 2, Col: 1}, Msg: "two"},
		{Pos: diag.Position{File: "a.zen", Line: 3, Col: 1}, Msg: "three"},
		// A diagnostic with no file has no document to attach to.
		{Pos: diag.Position{}, Msg: "backend failed"},
		nil,
	}

	grouped := groupDiagnostics(diags)

	if len(grouped) != 2 {
		t.Fatalf("grouped into %d files, want 2 (%v)", len(grouped), grouped)
	}

	if len(grouped["a.zen"]) != 2 {
		t.Errorf("a.zen has %d diagnostics, want 2", len(grouped["a.zen"]))
	}

	if len(grouped["b.zen"]) != 1 {
		t.Errorf("b.zen has %d diagnostics, want 1", len(grouped["b.zen"]))
	}
}

// TestPublishClearsStaleDiagnostics proves a file that had errors and now has
// none receives an explicit empty array, otherwise editors keep the squiggles.
func TestPublishClearsStaleDiagnostics(t *testing.T) {
	t.Parallel()

	client := &fakeClient{}
	p := newDiagnosticPublisher()
	known := []string{"/w/a.zen"}
	wantURI := string(pathToURI("/w/a.zen"))

	if err := p.publish(client, known, diag.List{
		{Pos: diag.Position{File: "/w/a.zen", Line: 1, Col: 1}, Msg: "boom"},
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	list, ok := client.published(wantURI)
	if !ok || len(list) != 1 {
		t.Fatalf("first publish = %v, want one file with one diagnostic", client.calls)
	}

	// Now clean: the same file must be published again, with an empty list.
	if err := p.publish(client, known, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}

	list, ok = client.published(wantURI)
	if !ok {
		t.Fatalf("second publish sent nothing, want a clearing publish for %q", wantURI)
	}

	if list == nil || len(list) != 0 {
		t.Errorf("second publish sent %d diagnostics, want 0 (a clearing publish)", len(list))
	}

	n := client.count()

	// Still clean and already cleared: nothing more to say.
	if err := p.publish(client, known, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if got := client.count(); got != n {
		t.Errorf("third publish sent %d notifications, want %d (already clean)", got, n)
	}
}

// TestPublishSendsPerFileDiagnostics proves one notification goes out per
// affected file with the messages preserved verbatim.
func TestPublishSendsPerFileDiagnostics(t *testing.T) {
	t.Parallel()

	client := &fakeClient{}
	p := newDiagnosticPublisher()

	err := p.publish(client, []string{"/w/a.zen", "/w/b.zen"}, diag.List{
		{Pos: diag.Position{File: "/w/a.zen", Line: 1, Col: 1}, Severity: diag.SeverityError, Msg: "one"},
		{Pos: diag.Position{File: "/w/b.zen", Line: 2, Col: 3}, Severity: diag.SeverityWarning, Msg: "two"},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	if got := client.count(); got != 2 {
		t.Fatalf("publish sent %d notifications, want 2", got)
	}

	a, ok := client.published(string(pathToURI("/w/a.zen")))
	if !ok || len(a) != 1 {
		t.Fatalf("a.zen publish = %v, want one diagnostic", a)
	}

	if msg, msgOK := a[0].Message.(protocol.String); !msgOK || string(msg) != "one" {
		t.Errorf("a.zen message = %#v, want %q", a[0].Message, "one")
	}

	if a[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("a.zen severity = %v, want error", a[0].Severity)
	}

	b, ok := client.published(string(pathToURI("/w/b.zen")))
	if !ok || len(b) != 1 || b[0].Severity != protocol.DiagnosticSeverityWarning {
		t.Errorf("b.zen publish = %+v, want one warning", b)
	}
}

// TestPublishSkipsCleanUnknownFiles proves a workspace with no diagnostics at
// all stays silent: files that never had diagnostics get no notification.
func TestPublishSkipsCleanUnknownFiles(t *testing.T) {
	t.Parallel()

	client := &fakeClient{}
	p := newDiagnosticPublisher()

	if err := p.publish(client, []string{"/w/a.zen"}, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if got := client.count(); got != 0 {
		t.Errorf("publish sent %d notifications for a clean workspace, want 0", got)
	}
}

// TestPublishNilClientDoesNothing proves a nil client is a silent no-op.
func TestPublishNilClientDoesNothing(t *testing.T) {
	t.Parallel()

	p := newDiagnosticPublisher()

	if err := p.publish(nil, []string{"/w/a.zen"}, diag.List{
		{Pos: diag.Position{File: "/w/a.zen", Line: 1, Col: 1}, Msg: "boom"},
	}); err != nil {
		t.Errorf("publish(nil client) = %v, want nil", err)
	}
}

// TestPublishWrapsClientErrors proves delivery failures come back wrapped,
// so callers can still match the client cause with errors.Is.
func TestPublishWrapsClientErrors(t *testing.T) {
	t.Parallel()

	client := &fakeClient{err: errPublishBoom}
	p := newDiagnosticPublisher()

	err := p.publish(client, []string{"/w/a.zen"}, diag.List{
		{Pos: diag.Position{File: "/w/a.zen", Line: 1, Col: 1}, Msg: "boom"},
	})
	if err == nil {
		t.Fatal("publish with a failing client returned nil, want the wrapped failure")
	}

	if !errors.Is(err, errPublishBoom) {
		t.Errorf("publish error = %v, want it to wrap errPublishBoom", err)
	}
}

// TestScheduleFiresOnceAfterDebounce proves a scheduled recompile runs when
// the debounce settles.
func TestScheduleFiresOnceAfterDebounce(t *testing.T) {
	p := newDiagnosticPublisher()
	clock := withFakeClock(p)

	fired := 0
	p.schedule("/w/a.zen", func() { fired++ })

	if got := clock.pending(); got != 1 {
		t.Fatalf("pending timers = %d, want 1", got)
	}

	clock.fireAll()

	if fired != 1 {
		t.Errorf("scheduled fn fired %d times, want 1", fired)
	}
}

// TestScheduleCollapsesRapidEdits proves a second keystroke before the delay
// cancels the first timer, so a burst compiles once.
func TestScheduleCollapsesRapidEdits(t *testing.T) {
	p := newDiagnosticPublisher()
	clock := withFakeClock(p)

	fired := 0
	p.schedule("/w/a.zen", func() { fired++ })
	p.schedule("/w/a.zen", func() { fired++ })

	clock.fireAll()

	if fired != 1 {
		t.Errorf("burst fired %d times, want exactly 1 (the second schedule must win)", fired)
	}
}

// TestScheduleIsPerDocument proves timers for different documents do not
// cancel each other.
func TestScheduleIsPerDocument(t *testing.T) {
	p := newDiagnosticPublisher()
	clock := withFakeClock(p)

	fired := map[string]int{}
	p.schedule("/w/a.zen", func() { fired["a"]++ })
	p.schedule("/w/b.zen", func() { fired["b"]++ })

	clock.fireAll()

	if fired["a"] != 1 || fired["b"] != 1 {
		t.Errorf("per-document fires = %v, want one each", fired)
	}
}

// TestScheduleUsesRealClockByDefault proves the production publisher wires a
// working timer source: schedule creates a pending entry and cancel drops it.
// There are no timing assertions here; the fake-clock tests above own the
// behavior proof, so this cannot flake.
func TestScheduleUsesRealClockByDefault(t *testing.T) {
	p := newDiagnosticPublisher()

	p.schedule("/w/a.zen", func() {})

	p.mu.Lock()
	pending := len(p.timers)
	p.mu.Unlock()

	if pending != 1 {
		t.Fatalf("pending timers = %d, want 1", pending)
	}

	p.cancel("/w/a.zen")
	p.cancel("/w/a.zen")

	p.mu.Lock()
	pending = len(p.timers)
	p.mu.Unlock()

	if pending != 0 {
		t.Errorf("pending timers after cancel = %d, want 0", pending)
	}
}

// TestCancelStopsPendingDebounce proves closing a document drops its pending
// recompile, and cancelling an unknown key is a no-op.
func TestCancelStopsPendingDebounce(t *testing.T) {
	p := newDiagnosticPublisher()
	clock := withFakeClock(p)

	fired := 0
	p.schedule("/w/a.zen", func() { fired++ })
	p.cancel("/w/a.zen")
	p.cancel("/w/missing.zen")

	clock.fireAll()

	if fired != 0 {
		t.Errorf("cancelled fn fired %d times, want 0", fired)
	}
}

// TestPublishClearsDeletedFile proves a file that vanished from the workspace
// (no longer in known, no new diagnostics) still gets a clearing publish
// instead of leaving stale squiggles behind.
func TestPublishClearsDeletedFile(t *testing.T) {
	t.Parallel()

	client := &fakeClient{}
	p := newDiagnosticPublisher()
	wantURI := string(pathToURI("/w/gone.zen"))

	if err := p.publish(client, []string{"/w/gone.zen"}, diag.List{
		{Pos: diag.Position{File: "/w/gone.zen", Line: 1, Col: 1}, Msg: "boom"},
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// gone.zen left the workspace: known is empty and the compile is clean.
	if err := p.publish(client, nil, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}

	list, ok := client.published(wantURI)
	if !ok {
		t.Fatalf("deleted file got no clearing publish for %q", wantURI)
	}

	if len(list) != 0 {
		t.Errorf("deleted file publish sent %d diagnostics, want 0 (a clearing publish)", len(list))
	}
}

// TestPublishSkipsFileRecordedClean covers the defensive branch for a file
// whose last-published state is already empty: it stays silent instead of
// re-sending a clearing publish.
func TestPublishSkipsFileRecordedClean(t *testing.T) {
	p := newDiagnosticPublisher()
	p.lastPublished["/w/a.zen"] = []protocol.Diagnostic{}

	client := &fakeClient{}

	if err := p.publish(client, []string{"/w/a.zen"}, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if got := client.count(); got != 0 {
		t.Errorf("publish sent %d notifications, want 0 (already clean)", got)
	}
}
