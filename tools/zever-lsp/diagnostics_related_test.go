package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/diag"
)

// TestDiagToLSPDiagnosticMapsRelated proves Related locations surface as
// LSP RelatedInformation: same-file entries reuse the primary source,
// cross-file entries consult srcFor, and unknown content falls back to a
// point range.
func TestDiagToLSPDiagnosticMapsRelated(t *testing.T) {
	t.Parallel()

	src := "entity User {\n}\n"

	srcFor := func(path string) (string, bool) {
		if path == "b.zen" {
			return src, true
		}

		return "", false
	}

	got := diagToLSPDiagnostic(&diag.Diagnostic{
		Pos: diag.Position{File: "a.zen", Line: 1, Col: 8},
		Msg: "boom",
		Related: []diag.RelatedInfo{
			{Pos: diag.Position{File: "a.zen", Line: 1, Col: 1}, Msg: "same file"},
			{Pos: diag.Position{File: "b.zen", Line: 1, Col: 8}, Msg: "other file"},
			{Pos: diag.Position{File: "c.zen", Line: 1, Col: 8}, Msg: "unknown file"},
		},
	}, src, srcFor)

	if len(got.RelatedInformation) != 3 {
		t.Fatalf("RelatedInformation len = %d, want 3", len(got.RelatedInformation))
	}

	same := got.RelatedInformation[0]
	if string(same.Location.URI) != string(pathToURI("a.zen")) {
		t.Errorf("same-file URI = %q, want %q", same.Location.URI, pathToURI("a.zen"))
	}

	if same.Message != "same file" {
		t.Errorf("same-file message = %q, want %q", same.Message, "same file")
	}

	if want := diagnosticRange(diag.Position{File: "a.zen", Line: 1, Col: 1}, src); same.Location.Range != want {
		t.Errorf("same-file range = %+v, want widened %+v", same.Location.Range, want)
	}

	other := got.RelatedInformation[1]
	if string(other.Location.URI) != string(pathToURI("b.zen")) {
		t.Errorf("cross-file URI = %q, want %q", other.Location.URI, pathToURI("b.zen"))
	}

	if want := diagnosticRange(diag.Position{File: "b.zen", Line: 1, Col: 8}, src); other.Location.Range != want {
		t.Errorf("cross-file range = %+v, want widened %+v", other.Location.Range, want)
	}

	unknown := got.RelatedInformation[2]
	if want := pointRange(diag.Position{File: "c.zen", Line: 1, Col: 8}); unknown.Location.Range != want {
		t.Errorf("unknown-file range = %+v, want point range %+v", unknown.Location.Range, want)
	}
}

// TestDiagToLSPDiagnosticSkipsEmptyRelatedFile proves a related entry with
// no file is dropped instead of publishing an empty URI.
func TestDiagToLSPDiagnosticSkipsEmptyRelatedFile(t *testing.T) {
	t.Parallel()

	got := diagToLSPDiagnostic(&diag.Diagnostic{
		Pos:     diag.Position{File: "a.zen", Line: 1, Col: 1},
		Msg:     "boom",
		Related: []diag.RelatedInfo{{Msg: "nowhere"}},
	}, "", nil)

	if len(got.RelatedInformation) != 0 {
		t.Fatalf("RelatedInformation = %+v, want empty", got.RelatedInformation)
	}
}

// TestRefreshDiagnosticsDuplicateAcrossFilesPointsAtFirst proves a
// duplicate declaration spanning two open documents publishes on the
// second file with RelatedInformation pointing at the first.
func TestRefreshDiagnosticsDuplicateAcrossFilesPointsAtFirst(t *testing.T) {
	s := NewServer()
	fake := &fakeClient{}
	s.setClient(fake)

	const dupBody = "entity Dup {\n  id: uuid\n}\n"

	openDoc(t, s, serverDocA, dupBody)
	openDoc(t, s, serverDocB, dupBody)

	// DidOpen already refreshed twice; force another round for determinism.
	s.refreshDiagnostics()

	list, ok := fake.published(diagURI(t, serverDocB))
	if !ok || len(list) == 0 {
		t.Fatalf("published for second file = %v, want the duplicate diagnostic", list)
	}

	var matched = -1

	for i := range list {
		text, ok := list[i].Message.(protocol.String)
		if !ok {
			continue
		}

		if strings.Contains(string(text), `duplicate top-level declaration "Dup"`) {
			matched = i

			break
		}
	}

	if matched < 0 {
		t.Fatalf("no duplicate-declaration diagnostic published for the second file: %+v", list)
	}

	rel := list[matched].RelatedInformation
	if len(rel) != 1 {
		t.Fatalf("RelatedInformation len = %d, want exactly 1", len(rel))
	}

	if string(rel[0].Location.URI) != diagURI(t, serverDocA) {
		t.Errorf("related URI = %q, want the first file %q", rel[0].Location.URI, diagURI(t, serverDocA))
	}

	if rel[0].Message != "first declared here" {
		t.Errorf("related message = %q, want %q", rel[0].Message, "first declared here")
	}
}
