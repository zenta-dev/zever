package diag

import "testing"

// TestNewMsgMatchesNew pins NewMsg's documented contract: it produces a
// diagnostic identical to New(phase, pos, "%s", msg), with no wrapped cause.
func TestNewMsgMatchesNew(t *testing.T) {
	t.Parallel()

	pos := Position{File: "app.zen", Line: 3, Col: 7}

	msg := NewMsg("lex", pos, "illegal character '@'")
	want := New("lex", pos, "%s", "illegal character '@'")

	if msg.Pos != want.Pos {
		t.Fatalf("NewMsg Pos = %v, want %v", msg.Pos, want.Pos)
	}

	if msg.Phase != want.Phase {
		t.Fatalf("NewMsg Phase = %q, want %q", msg.Phase, want.Phase)
	}

	if msg.Msg != want.Msg {
		t.Fatalf("NewMsg Msg = %q, want %q", msg.Msg, want.Msg)
	}

	if msg.Severity != want.Severity {
		t.Fatalf("NewMsg Severity = %v, want %v", msg.Severity, want.Severity)
	}

	if msg.Error() != want.Error() {
		t.Fatalf("NewMsg Error() = %q, want %q", msg.Error(), want.Error())
	}

	if msg.Wrapped != nil || msg.Unwrap() != nil {
		t.Fatalf("NewMsg Wrapped = %v, want nil", msg.Wrapped)
	}
}

// TestNewMsgNoFormatExpansion proves NewMsg treats its message as literal
// text: a percent sign must not be re-interpreted as a format verb.
func TestNewMsgNoFormatExpansion(t *testing.T) {
	t.Parallel()

	msg := NewMsg("parse", Position{}, "100% unexpected %v")

	if got, want := msg.Msg, "100% unexpected %v"; got != want {
		t.Fatalf("NewMsg Msg = %q, want literal %q", got, want)
	}
}

// BenchmarkNewMsg measures the zero-variadic constructor the lexer uses on
// its per-rune illegal-character path.
func BenchmarkNewMsg(b *testing.B) {
	pos := Position{File: "app.zen", Line: 3, Col: 7}

	b.ReportAllocs()

	for b.Loop() {
		_ = NewMsg("lex", pos, "illegal character")
	}
}
