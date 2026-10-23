package main

import (
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/parser"
)

// inlineInsertText returns the insert text of an inline item, failing the
// test when it is not the plain-string shape this server always emits.
func inlineInsertText(t *testing.T, item *protocol.InlineCompletionItem) string {
	t.Helper()

	text, ok := item.InsertText.(protocol.String)
	if !ok {
		t.Fatalf("InlineCompletionItem.InsertText = %#v, want protocol.String", item.InsertText)
	}

	return string(text)
}

// TestInlineCompletionOffers pins the one skeleton offered per declaration
// slot: the entity block at top level, the index member in an entity body,
// and the rpc member in a service body. Every offer covers the whitespace
// run of its line and carries an empty filter text.
func TestInlineCompletionOffers(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		line       uint32
		character  uint32
		wantInsert string
		wantStart  uint32
		wantEnd    uint32
	}{
		{
			name:       "empty file offers entity",
			src:        "",
			line:       0,
			character:  0,
			wantInsert: "entity ${1:Name} {\n\t$0\n}",
			wantStart:  0,
			wantEnd:    0,
		},
		{
			name:       "blank line between declarations offers entity",
			src:        "entity User {\n}\n\n",
			line:       3,
			character:  0,
			wantInsert: "entity ${1:Name} {\n\t$0\n}",
			wantStart:  0,
			wantEnd:    0,
		},
		{
			name:       "indented empty entity line offers index",
			src:        "entity User {\n\tid: uuid @primary\n\t\n}\n",
			line:       2,
			character:  1,
			wantInsert: "index ${1:cols}",
			wantStart:  0,
			wantEnd:    1,
		},
		{
			name:       "empty service line offers rpc",
			src:        "service S {\n\t\n}\n",
			line:       1,
			character:  1,
			wantInsert: "rpc ${1:name}($2) -> ${3:Type} {\n\t$0\n}",
			wantStart:  0,
			wantEnd:    1,
		},
		{
			name:       "final empty line after trailing newline offers entity",
			src:        "entity User {\n}\n",
			line:       2,
			character:  0,
			wantInsert: "entity ${1:Name} {\n\t$0\n}",
			wantStart:  0,
			wantEnd:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := parser.New("inline.zen", []byte(tc.src)).ParseFile()
			cursor := protocol.Position{Line: tc.line, Character: tc.character}

			item := inlineCompletionAt(nil, file, tc.src, cursor, clientCaps{})
			if item == nil {
				t.Fatalf("inlineCompletionAt() = nil, want %q", tc.wantInsert)
			}

			if got := inlineInsertText(t, item); got != tc.wantInsert {
				t.Errorf("inlineCompletionAt() insert = %q, want %q", got, tc.wantInsert)
			}

			if item.FilterText == nil || *item.FilterText != "" {
				t.Errorf("inlineCompletionAt() filter = %#v, want non-nil empty", item.FilterText)
			}

			if item.Range == nil {
				t.Fatalf("inlineCompletionAt() range is nil, want same-line whitespace run")
			}

			wantRange := protocol.Range{
				Start: protocol.Position{Line: tc.line, Character: tc.wantStart},
				End:   protocol.Position{Line: tc.line, Character: tc.wantEnd},
			}
			if *item.Range != wantRange {
				t.Errorf("inlineCompletionAt() range = %+v, want %+v", *item.Range, wantRange)
			}

			snippetItem := inlineCompletionAt(nil, file, tc.src, cursor, snippetCaps())
			if snippetItem == nil {
				t.Fatalf("inlineCompletionAt(snippet caps) = nil, want %q", tc.wantInsert)
			}

			if got := inlineInsertText(t, snippetItem); got != tc.wantInsert {
				t.Errorf("inlineCompletionAt(snippet caps) insert = %q, want %q (caps must not change ghost text)", got, tc.wantInsert)
			}
		})
	}
}

// TestInlineCompletionSilent pins that ghost text stays silent everywhere
// else: mid-line and mid-declaration cursors, blocks with no single
// declaration skeleton, and cursors past the end of the buffer.
func TestInlineCompletionSilent(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		line      uint32
		character uint32
	}{
		{
			name:      "mid line after keyword",
			src:       "entity User {\n}\n",
			line:      0,
			character: 6,
		},
		{
			name:      "mid declaration",
			src:       "entity User {\n\tid: uuid\n}\n",
			line:      1,
			character: 5,
		},
		{
			name:      "partial keyword line",
			src:       "enti",
			line:      0,
			character: 4,
		},
		{
			name:      "rpc body empty line",
			src:       "service S {\n\trpc M() -> T {\n\t\t\n\t}\n}\n",
			line:      2,
			character: 2,
		},
		{
			name:      "job body empty line",
			src:       "job J() {\n\t\n}\n",
			line:      1,
			character: 1,
		},
		{
			name:      "schedule body empty line",
			src:       "schedule S {\n\t\n}\n",
			line:      1,
			character: 1,
		},
		{
			name:      "message body empty line",
			src:       "message M {\n\t\n}\n",
			line:      1,
			character: 1,
		},
		{
			name:      "errors set empty line",
			src:       "service S {\n\trpc M() -> T {\n\t\terrors: {\n\t\t\t\n\t\t}\n\t}\n}\n",
			line:      3,
			character: 3,
		},
		{
			name:      "cursor past end of buffer",
			src:       "entity User {\n}\n",
			line:      50,
			character: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := parser.New("inline.zen", []byte(tc.src)).ParseFile()
			cursor := protocol.Position{Line: tc.line, Character: tc.character}

			if item := inlineCompletionAt(nil, file, tc.src, cursor, clientCaps{}); item != nil {
				t.Errorf("inlineCompletionAt() = %+v, want nil", item)
			}
		})
	}
}

// TestInlineCompletionHandler covers the server handler: an offer on an
// empty line arrives as a one-item list, while unknown documents and
// mid-line cursors answer null.
func TestInlineCompletionHandler(t *testing.T) {
	t.Run("empty line offers one-item list", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, "")

		res, err := s.InlineCompletion(t.Context(), &protocol.InlineCompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("InlineCompletion: %v", err)
		}

		list, ok := res.(*protocol.InlineCompletionList)
		if !ok || len(list.Items) != 1 {
			t.Fatalf("InlineCompletion result = %#v, want one-item *protocol.InlineCompletionList", res)
		}

		if got := inlineInsertText(t, &list.Items[0]); got != "entity ${1:Name} {\n\t$0\n}" {
			t.Errorf("InlineCompletion insert = %q, want entity skeleton", got)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		res, err := s.InlineCompletion(t.Context(), &protocol.InlineCompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("InlineCompletion: %v", err)
		}

		if res != nil {
			t.Errorf("InlineCompletion unknown doc = %#v, want null", res)
		}
	})

	t.Run("mid-line cursor is null", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, "entity User {\n}\n")

		res, err := s.InlineCompletion(t.Context(), &protocol.InlineCompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: 6},
			},
		})
		if err != nil {
			t.Fatalf("InlineCompletion: %v", err)
		}

		if res != nil {
			t.Errorf("InlineCompletion mid-line = %#v, want null", res)
		}
	})
}

// TestInitialize_inlineCompletionProvider pins the capability: ghost text is
// a bare bool, since unlike completion there are no trigger characters to
// configure.
func TestInitialize_inlineCompletionProvider(t *testing.T) {
	s := NewServer()

	res, err := s.Initialize(t.Context(), &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	provider, ok := res.Capabilities.InlineCompletionProvider.(protocol.Boolean)
	if !ok || !bool(provider) {
		t.Errorf("InlineCompletionProvider = %#v, want Boolean(true)", res.Capabilities.InlineCompletionProvider)
	}
}
